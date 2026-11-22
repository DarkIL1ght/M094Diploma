// Package content — загрузка учебного контента из seed (S-05): темы, разделы, общие материалы и вопросы.
// Read разбирает каталог seed и проверяет его целиком, Load передаёт проверенный seed хранилищу (порт Store).
// Формат seed — docs/seed.md, таблицы — docs/db.md.
package content

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Предметы КТ — те же коды, что в домене subject_code и content/exam_config.json.
var Subjects = []string{"english", "tgo", "algorithms", "databases"}

// Вид вопроса: один верный вариант или несколько.
const (
	KindSingle   = "single"
	KindMultiple = "multiple"
)

// Seed — весь контент из каталога seed. Пустые строки в необязательных полях означают «нет» (в базе — null).
type Seed struct {
	Topics    []Topic
	Sections  []Section
	Materials []Material
	Questions []Question
}

// Topic — тема предмета. Subject берётся из каталога, в котором лежит файл.
type Topic struct {
	ExtID    string `json:"ext_id"`
	Subject  string `json:"-"`
	Title    string `json:"title"`
	Position int    `json:"position"`
}

// Section — раздел (урок) темы.
type Section struct {
	ExtID    string `json:"ext_id"`
	Topic    string `json:"topic"`
	Title    string `json:"title"`
	Position int    `json:"position"`
	BodyMD   string `json:"body_md"`
}

// Material — общий текст или таблица к группе вопросов.
type Material struct {
	ExtID  string `json:"ext_id"`
	BodyMD string `json:"body_md"`
}

// Question — вопрос с вариантами. Topic, Section, SharedMaterial — ext_id, пустые — ссылки нет.
type Question struct {
	ExtID          string   `json:"ext_id"`
	Subject        string   `json:"-"`
	Topic          string   `json:"topic,omitempty"`
	Kind           string   `json:"kind"`
	TextMD         string   `json:"text_md"`
	CodeLang       string   `json:"code_lang,omitempty"`
	CodeSource     string   `json:"code_source,omitempty"`
	SharedMaterial string   `json:"shared_material,omitempty"`
	ExplanationMD  string   `json:"explanation_md"`
	Section        string   `json:"section,omitempty"`
	ListenText     string   `json:"listen_text,omitempty"`
	ListenLang     string   `json:"listen_lang,omitempty"`
	Options        []Option `json:"options"`
}

// Option — вариант ответа. Position — место в списке options, начиная с 1.
type Option struct {
	ExtID     string `json:"ext_id"`
	Position  int    `json:"-"`
	TextMD    string `json:"text_md"`
	IsCorrect bool   `json:"is_correct"`
}

// SumHash строк — sha256 в hex от канонического JSON всех полей строки (поля в порядке объявления, ссылки — ext_id).
// Загрузчик переписывает строку, только когда хэш изменился; к хэшу вопроса привязан кэш объяснений ИИ.
func (t Topic) SumHash() string {
	return sumHash(struct {
		Topic
		Subject string `json:"subject"`
	}{t, t.Subject})
}

func (s Section) SumHash() string  { return sumHash(s) }
func (m Material) SumHash() string { return sumHash(m) }

// SumHash вопроса считается вместе с вариантами и их порядком.
func (q Question) SumHash() string {
	return sumHash(struct {
		Question
		Subject string `json:"subject"`
	}{q, q.Subject})
}

func sumHash(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // в строках только строки, числа и логические значения
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
