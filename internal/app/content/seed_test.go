package content_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/DarkIL1ght/M094Diploma/internal/app/content"
)

// validFS — маленький правильный seed: тема и раздел по базам данных, общий материал, два вопроса и вопрос аудирования.
func validFS() fstest.MapFS {
	return fstest.MapFS{
		"databases/topics.json":    {Data: []byte(`[{"ext_id":"db-sql","title":"SQL","position":1}]`)},
		"databases/sections.json":  {Data: []byte(`[{"ext_id":"db-sql-select","topic":"db-sql","title":"SELECT","position":1,"body_md":"Текст"}]`)},
		"databases/materials.json": {Data: []byte(`[{"ext_id":"db-tbl","body_md":"| a |"}]`)},
		"databases/questions.json": {Data: []byte(`[
			{"ext_id":"db-q1","topic":"db-sql","kind":"single","text_md":"Что?","explanation_md":"Так.","section":"db-sql-select",
			 "shared_material":"db-tbl","code_lang":"sql","code_source":"select 1;",
			 "options":[{"ext_id":"a","text_md":"Да","is_correct":true},{"ext_id":"b","text_md":"Нет","is_correct":false}]},
			{"ext_id":"db-q2","topic":"db-sql","kind":"multiple","text_md":"Какие?","explanation_md":"Эти.",
			 "options":[{"ext_id":"a","text_md":"1","is_correct":true},{"ext_id":"b","text_md":"2","is_correct":true}]}
		]`)},
		"english/questions.json": {Data: []byte(`[
			{"ext_id":"en-q1","kind":"single","text_md":"Pick","explanation_md":"Because.","listen_text":"Hello","listen_lang":"en-GB",
			 "options":[{"ext_id":"a","text_md":"x","is_correct":false},{"ext_id":"b","text_md":"y","is_correct":true}]}
		]`)},
	}
}

// questionFS — английский вопрос "q" с заданными полями kind, options и прочими.
func questionFS(fields string) fstest.MapFS {
	return fstest.MapFS{"english/questions.json": {Data: []byte(`[{"ext_id":"q","text_md":"?","explanation_md":".",` + fields + `}]`)}}
}

const twoOptions = `"options":[{"ext_id":"a","text_md":"a","is_correct":true},{"ext_id":"b","text_md":"b","is_correct":false}]`

func TestReadValidSeed(t *testing.T) {
	s, err := content.Read(validFS())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Topics) != 1 || len(s.Sections) != 1 || len(s.Materials) != 1 || len(s.Questions) != 3 {
		t.Fatalf("прочитано %d/%d/%d/%d", len(s.Topics), len(s.Sections), len(s.Materials), len(s.Questions))
	}
	if s.Topics[0].Subject != "databases" || s.Questions[2].Subject != "english" {
		t.Errorf("предмет берётся из каталога: %+v %+v", s.Topics[0], s.Questions[2])
	}
	if q := s.Questions[0]; q.Options[0].Position != 1 || q.Options[1].Position != 2 {
		t.Errorf("позиция варианта — его место в списке: %+v", q.Options)
	}
}

func TestReadReportsProblemWithFileAndExtID(t *testing.T) {
	cases := map[string]struct {
		fs   fstest.MapFS
		want string
	}{
		"неизвестный предмет": {fstest.MapFS{"math/topics.json": {Data: []byte(`[]`)}}, `неизвестный предмет "math"`},
		"неизвестный файл":    {fstest.MapFS{"tgo/notes.json": {Data: []byte(`[]`)}}, `tgo/notes.json: неизвестный файл`},
		"неизвестное поле": {fstest.MapFS{"tgo/topics.json": {Data: []byte(`[{"ext_id":"t","title":"T","position":1,"pos":2}]`)}},
			`tgo/topics.json: json: unknown field "pos"`},
		"повтор ext_id": {fstest.MapFS{"tgo/topics.json": {Data: []byte(`[{"ext_id":"t","title":"A","position":1},{"ext_id":"t","title":"B","position":2}]`)}},
			`tgo/topics.json: t: ext_id уже встречался`},
		"повтор позиции": {fstest.MapFS{"tgo/topics.json": {Data: []byte(`[{"ext_id":"t1","title":"A","position":1},{"ext_id":"t2","title":"B","position":1}]`)}},
			`t2: позиция 1 уже занята`},
		"позиция с нуля": {fstest.MapFS{"tgo/topics.json": {Data: []byte(`[{"ext_id":"t","title":"A","position":0}]`)}},
			`t: позиция начинается с 1`},
		"нет темы раздела": {fstest.MapFS{"tgo/sections.json": {Data: []byte(`[{"ext_id":"s","topic":"nope","title":"S","position":1,"body_md":"x"}]`)}},
			`tgo/sections.json: s: нет темы "nope"`},
		"тема другого предмета": {fstest.MapFS{
			"tgo/topics.json":        {Data: []byte(`[{"ext_id":"t","title":"T","position":1}]`)},
			"english/questions.json": {Data: []byte(`[{"ext_id":"q","topic":"t","kind":"single","text_md":"?","explanation_md":".",` + twoOptions + `}]`)}},
			`english/questions.json: q: тема "t" — предмета tgo`},
		"single с двумя верными": {questionFS(`"kind":"single","options":[{"ext_id":"a","text_md":"a","is_correct":true},{"ext_id":"b","text_md":"b","is_correct":true}]`),
			`q: у single ровно один верный вариант, а здесь 2`},
		"multiple без верного": {questionFS(`"kind":"multiple","options":[{"ext_id":"a","text_md":"a","is_correct":false},{"ext_id":"b","text_md":"b","is_correct":false}]`),
			`q: у multiple нужен хотя бы один верный вариант`},
		"один вариант":     {questionFS(`"kind":"multiple","options":[{"ext_id":"a","text_md":"a","is_correct":true}]`), `q: нужно хотя бы два варианта`},
		"неизвестный kind": {questionFS(`"kind":"open",` + twoOptions), `q: kind должен быть single или multiple`},
		"повтор варианта": {questionFS(`"kind":"single","options":[{"ext_id":"a","text_md":"a","is_correct":true},{"ext_id":"a","text_md":"b","is_correct":false}]`),
			`q: вариант "a" повторяется`},
		"код без языка":    {questionFS(`"kind":"single","code_source":"x",` + twoOptions), `q: code_lang и code_source задаются вместе`},
		"аудио без языка":  {questionFS(`"kind":"single","listen_text":"x",` + twoOptions), `q: listen_text и listen_lang задаются вместе`},
		"нет материала":    {questionFS(`"kind":"single","shared_material":"m",` + twoOptions), `q: нет общего материала "m"`},
		"нет раздела":      {questionFS(`"kind":"single","section":"s",` + twoOptions), `q: нет раздела "s"`},
		"пустое поле темы": {fstest.MapFS{"tgo/topics.json": {Data: []byte(`[{"ext_id":"t","title":" ","position":1}]`)}}, `t: пустое поле title`},
		"профильный вопрос без темы": {fstest.MapFS{"algorithms/questions.json": {Data: []byte(`[{"ext_id":"q","kind":"single","text_md":"?","explanation_md":".",` + twoOptions + `}]`)}},
			`algorithms/questions.json: q: у вопроса по algorithms нужна тема`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := content.Read(c.fs)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("ошибка %v, ждали с %q", err, c.want)
			}
		})
	}
}

func TestReadCollectsAllProblems(t *testing.T) {
	fs := fstest.MapFS{
		"tgo/topics.json":   {Data: []byte(`[{"ext_id":"t","title":"","position":1}]`)},
		"tgo/sections.json": {Data: []byte(`[{"ext_id":"s","topic":"x","title":"S","position":1,"body_md":"x"}]`)},
	}
	_, err := content.Read(fs)
	if err == nil || !strings.Contains(err.Error(), "пустое поле title") || !strings.Contains(err.Error(), `нет темы "x"`) {
		t.Fatalf("все ошибки разом: %v", err)
	}
}

func TestSumHash(t *testing.T) {
	s, err := content.Read(validFS())
	if err != nil {
		t.Fatal(err)
	}
	q := s.Questions[0]
	h := q.SumHash()
	if len(h) != 64 || strings.Trim(h, "0123456789abcdef") != "" {
		t.Fatalf("sum_hash — sha256 в hex: %q", h)
	}
	if q.SumHash() != h {
		t.Error("хэш одной строки стабилен")
	}
	q.Options = append([]content.Option(nil), q.Options...)
	q.Options[1].IsCorrect = true
	if q.SumHash() == h {
		t.Error("хэш вопроса зависит от вариантов")
	}
	top := s.Topics[0]
	th := top.SumHash()
	top.Position = 2
	if top.SumHash() == th {
		t.Error("хэш темы зависит от позиции")
	}
	if s.Sections[0].SumHash() == s.Materials[0].SumHash() {
		t.Error("у разных строк разные хэши")
	}
}

type fakeStore struct{ calls int }

func (f *fakeStore) Replace(_ context.Context, s content.Seed) (content.Report, error) {
	f.calls++
	return content.Report{Topics: content.Counts{Added: len(s.Topics)}}, nil
}

func TestLoadWritesOnlyValidSeed(t *testing.T) {
	st := &fakeStore{}
	if _, err := content.Load(context.Background(), questionFS(`"kind":"single","options":[]`), st); err == nil || st.calls != 0 {
		t.Fatalf("неверный seed: ошибка %v, записей %d", err, st.calls)
	}
	rep, err := content.Load(context.Background(), validFS(), st)
	if err != nil || st.calls != 1 || rep.Topics.Added != 1 {
		t.Fatalf("Load: %v, записей %d, отчёт %+v", err, st.calls, rep)
	}
}

func TestReportString(t *testing.T) {
	r := content.Report{Questions: content.Counts{Added: 2, Changed: 1, Unchanged: 3, Restored: 1, Retired: 4}}
	if !strings.Contains(r.String(), "вопросы: добавлено 2, изменено 1, без изменений 3, возвращено 1, выведено 4") {
		t.Errorf("отчёт: %s", r)
	}
	if !r.Changed() || (content.Report{Topics: content.Counts{Unchanged: 5}}).Changed() {
		t.Error("Changed — были ли изменения")
	}
}
