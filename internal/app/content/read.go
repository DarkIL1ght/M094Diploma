package content

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
)

// Файлы каталога предмета; любой может отсутствовать.
const (
	topicsFile    = "topics.json"
	sectionsFile  = "sections.json"
	materialsFile = "materials.json"
	questionsFile = "questions.json"
)

// Read читает seed из fsys (каталоги <subject>/ с файлами topics, sections, materials, questions .json) и проверяет
// его целиком. Ошибка перечисляет все найденные проблемы, каждую — с файлом и ext_id.
func Read(fsys fs.FS) (Seed, error) {
	r := reader{fsys: fsys}
	r.readAll()
	if len(r.errs) == 0 {
		r.validate()
	}
	if len(r.errs) > 0 {
		return Seed{}, fmt.Errorf("seed не загружен:\n%w", errors.Join(r.errs...))
	}
	return r.seed, nil
}

// reader копит разобранные строки, файл каждой строки (для сообщений) и ошибки.
type reader struct {
	fsys fs.FS
	seed Seed
	errs []error

	topicFile, sectionFile, materialFile, questionFile []string
}

func (r *reader) fail(file, extID, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if extID != "" {
		msg = extID + ": " + msg
	}
	r.errs = append(r.errs, fmt.Errorf("%s: %s", file, msg))
}

func (r *reader) readAll() {
	dirs, err := fs.ReadDir(r.fsys, ".")
	if err != nil {
		r.errs = append(r.errs, err)
		return
	}
	for _, d := range dirs {
		subject := d.Name()
		if !d.IsDir() || !slices.Contains(Subjects, subject) {
			r.fail(subject, "", "неизвестный предмет %q (есть %s)", subject, strings.Join(Subjects, ", "))
			continue
		}
		files, err := fs.ReadDir(r.fsys, subject)
		if err != nil {
			r.errs = append(r.errs, err)
			continue
		}
		for _, f := range files {
			name := path.Join(subject, f.Name())
			switch f.Name() {
			case topicsFile:
				var rows []Topic
				if r.decode(name, &rows) {
					for i := range rows {
						rows[i].Subject = subject
						r.topicFile = append(r.topicFile, name)
					}
					r.seed.Topics = append(r.seed.Topics, rows...)
				}
			case sectionsFile:
				var rows []Section
				if r.decode(name, &rows) {
					r.sectionFile = appendN(r.sectionFile, name, len(rows))
					r.seed.Sections = append(r.seed.Sections, rows...)
				}
			case materialsFile:
				var rows []Material
				if r.decode(name, &rows) {
					r.materialFile = appendN(r.materialFile, name, len(rows))
					r.seed.Materials = append(r.seed.Materials, rows...)
				}
			case questionsFile:
				var rows []Question
				if r.decode(name, &rows) {
					for i := range rows {
						rows[i].Subject = subject
						for j := range rows[i].Options {
							rows[i].Options[j].Position = j + 1
						}
					}
					r.questionFile = appendN(r.questionFile, name, len(rows))
					r.seed.Questions = append(r.seed.Questions, rows...)
				}
			default:
				r.fail(name, "", "неизвестный файл (есть %s, %s, %s, %s)", topicsFile, sectionsFile, materialsFile, questionsFile)
			}
		}
	}
}

func appendN(s []string, v string, n int) []string {
	for range n {
		s = append(s, v)
	}
	return s
}

// decode разбирает файл строго: опечатка в имени поля — ошибка, а не молча пропавшее значение.
func (r *reader) decode(name string, v any) bool {
	b, err := fs.ReadFile(r.fsys, name)
	if err != nil {
		r.errs = append(r.errs, err)
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		r.fail(name, "", "%v", err)
		return false
	}
	return true
}

func (r *reader) validate() {
	topics := map[string]Topic{}
	topicPos := map[string]bool{} // subject/position
	for i, t := range r.seed.Topics {
		f := r.topicFile[i]
		unique(r, f, t.ExtID, topics)
		topics[t.ExtID] = t
		r.required(f, t.ExtID, "title", t.Title)
		r.position(f, t.ExtID, t.Position, topicPos, t.Subject)
	}

	sectionSubject := map[string]string{}
	sectionPos := map[string]bool{}
	for i, s := range r.seed.Sections {
		f := r.sectionFile[i]
		unique(r, f, s.ExtID, sectionSubject)
		r.required(f, s.ExtID, "title", s.Title)
		r.required(f, s.ExtID, "body_md", s.BodyMD)
		r.position(f, s.ExtID, s.Position, sectionPos, s.Topic)
		subject := path.Dir(f)
		sectionSubject[s.ExtID] = subject
		if t, ok := topics[s.Topic]; !ok {
			r.fail(f, s.ExtID, "нет темы %q", s.Topic)
		} else if t.Subject != subject {
			r.fail(f, s.ExtID, "тема %q — предмета %s", s.Topic, t.Subject)
		}
	}

	materials := map[string]bool{}
	for i, m := range r.seed.Materials {
		f := r.materialFile[i]
		unique(r, f, m.ExtID, materials)
		materials[m.ExtID] = true
		r.required(f, m.ExtID, "body_md", m.BodyMD)
	}

	questions := map[string]bool{}
	for i, q := range r.seed.Questions {
		f := r.questionFile[i]
		unique(r, f, q.ExtID, questions)
		questions[q.ExtID] = true
		r.question(f, q, topics, sectionSubject, materials)
	}
}

func (r *reader) question(f string, q Question, topics map[string]Topic, sectionSubject map[string]string, materials map[string]bool) {
	id := q.ExtID
	r.required(f, id, "text_md", q.TextMD)
	r.required(f, id, "explanation_md", q.ExplanationMD)
	switch {
	case q.Topic != "":
		if t, ok := topics[q.Topic]; !ok {
			r.fail(f, id, "нет темы %q", q.Topic)
		} else if t.Subject != q.Subject {
			r.fail(f, id, "тема %q — предмета %s", q.Topic, t.Subject)
		}
	case q.Subject == "algorithms" || q.Subject == "databases":
		// без темы бывают только вопросы английского и ТГО (комментарий к questions.topic_id в схеме)
		r.fail(f, id, "у вопроса по %s нужна тема", q.Subject)
	}
	if q.Section != "" {
		if s, ok := sectionSubject[q.Section]; !ok {
			r.fail(f, id, "нет раздела %q", q.Section)
		} else if s != q.Subject {
			r.fail(f, id, "раздел %q — предмета %s", q.Section, s)
		}
	}
	if q.SharedMaterial != "" && !materials[q.SharedMaterial] {
		r.fail(f, id, "нет общего материала %q", q.SharedMaterial)
	}
	if (q.CodeLang == "") != (q.CodeSource == "") {
		r.fail(f, id, "code_lang и code_source задаются вместе")
	}
	if (q.ListenText == "") != (q.ListenLang == "") {
		r.fail(f, id, "listen_text и listen_lang задаются вместе")
	}

	if len(q.Options) < 2 {
		r.fail(f, id, "нужно хотя бы два варианта")
	}
	correct := 0
	seen := map[string]bool{}
	for _, o := range q.Options {
		if seen[o.ExtID] {
			r.fail(f, id, "вариант %q повторяется", o.ExtID)
		}
		seen[o.ExtID] = true
		r.required(f, id, "ext_id варианта", o.ExtID)
		r.required(f, id, "text_md варианта", o.TextMD)
		if o.IsCorrect {
			correct++
		}
	}
	switch q.Kind {
	case KindSingle:
		if correct != 1 {
			r.fail(f, id, "у single ровно один верный вариант, а здесь %d", correct)
		}
	case KindMultiple:
		if correct == 0 {
			r.fail(f, id, "у multiple нужен хотя бы один верный вариант")
		}
	default:
		r.fail(f, id, "kind должен быть single или multiple, а не %q", q.Kind)
	}
}

// unique проверяет, что ext_id задан и не встречался среди строк того же вида (во всём seed, как в базе).
func unique[V any](r *reader, f, extID string, seen map[string]V) {
	if strings.TrimSpace(extID) == "" {
		r.fail(f, "", "строка без ext_id")
		return
	}
	if _, ok := seen[extID]; ok {
		r.fail(f, extID, "ext_id уже встречался")
	}
}

func (r *reader) required(f, extID, field, v string) {
	if strings.TrimSpace(v) == "" {
		r.fail(f, extID, "пустое поле %s", field)
	}
}

// position проверяет, что позиция ≥ 1 и не занята внутри группы (тем предмета, разделов темы).
func (r *reader) position(f, extID string, pos int, taken map[string]bool, group string) {
	if pos < 1 {
		r.fail(f, extID, "позиция начинается с 1, а здесь %d", pos)
		return
	}
	key := fmt.Sprintf("%s/%d", group, pos)
	if taken[key] {
		r.fail(f, extID, "позиция %d уже занята", pos)
	}
	taken[key] = true
}
