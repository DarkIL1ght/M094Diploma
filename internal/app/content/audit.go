package content

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// Fingerprint — отпечаток вопроса по смыслу: предмет, условие, код и множество вариантов с отметкой верных, без
// регистра, пунктуации, лишних пробелов и порядка вариантов. В отличие от SumHash не зависит от ext_id и пояснения:
// два вопроса с одним отпечатком — дубль, даже если их завели под разными ext_id или переставили варианты.
func (q Question) Fingerprint() string {
	opts := make([]string, len(q.Options))
	for i, o := range q.Options {
		mark := "-"
		if o.IsCorrect {
			mark = "+"
		}
		opts[i] = mark + normalize(o.TextMD)
	}
	sort.Strings(opts)
	parts := append([]string{q.Subject, normalize(q.TextMD), normalize(q.CodeSource), normalize(q.SharedMaterial)}, opts...)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

// normalize приводит текст к нижнему регистру, схлопывает пробелы и убирает обратные кавычки Markdown и знаки
// препинания по краям. Символы внутри (скобки, степени, операторы) остаются: O(n²) и O(n) — разные ответы.
func normalize(s string) string {
	s = strings.Join(strings.Fields(strings.ToLower(strings.ReplaceAll(s, "`", ""))), " ")
	return strings.TrimFunc(s, func(r rune) bool { return unicode.IsPunct(r) && !strings.ContainsRune("()[]{}", r) })
}

// Audit ищет в проверенном seed то, что Read пропускает как допустимое, но в банке вопросов быть не должно:
// дубли вопросов (по отпечатку), одинаковые варианты внутри вопроса, пустые пояснения.
func Audit(s Seed) []string {
	var problems []string
	seen := map[string]string{}
	for _, q := range s.Questions {
		if fp := q.Fingerprint(); seen[fp] != "" {
			problems = append(problems, fmt.Sprintf("%s: дубль вопроса %s", q.ExtID, seen[fp]))
		} else {
			seen[fp] = q.ExtID
		}
		texts := map[string]bool{}
		for _, o := range q.Options {
			t := normalize(o.TextMD)
			if texts[t] {
				problems = append(problems, fmt.Sprintf("%s: два одинаковых варианта %q", q.ExtID, o.TextMD))
			}
			texts[t] = true
		}
		if strings.TrimSpace(q.ExplanationMD) == "" {
			problems = append(problems, fmt.Sprintf("%s: пустое пояснение", q.ExtID))
		}
	}
	return problems
}

// QuestionsBySubject — сколько вопросов каждого предмета в seed (для сверки с форматом КТ).
func QuestionsBySubject(s Seed) map[string]int {
	n := map[string]int{}
	for _, q := range s.Questions {
		n[q.Subject]++
	}
	return n
}
