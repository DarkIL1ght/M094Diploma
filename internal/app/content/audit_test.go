package content_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DarkIL1ght/M094Diploma/internal/app/content"
)

func q(id, text string, opts ...content.Option) content.Question {
	return content.Question{ExtID: id, Subject: "algorithms", Kind: "single", TextMD: text, ExplanationMD: "Потому что.", Options: opts}
}

func opt(id, text string, ok bool) content.Option {
	return content.Option{ExtID: id, TextMD: text, IsCorrect: ok}
}

func TestFingerprintIgnoresIDsCaseAndOptionOrder(t *testing.T) {
	a := q("a", "Сложность бинарного поиска?", opt("1", "O(log n)", true), opt("2", "O(n)", false))
	b := q("b", "  сложность БИНАРНОГО поиска ", opt("x", "O(n)", false), opt("y", "O(log n)", true))
	b.ExplanationMD = "Другое пояснение."
	if a.Fingerprint() != b.Fingerprint() {
		t.Fatal("одинаковые по смыслу вопросы дали разные отпечатки")
	}
	c := q("c", "Сложность бинарного поиска?", opt("1", "O(log n)", false), opt("2", "O(n)", true))
	if a.Fingerprint() == c.Fingerprint() {
		t.Fatal("другой верный ответ — другой вопрос")
	}
	if got := content.Audit(content.Seed{Questions: []content.Question{q("e", "?", opt("1", "`O(n²)`", true), opt("2", "`O(n)`", false))}}); len(got) != 0 {
		t.Fatalf("O(n²) и O(n) — разные варианты: %v", got)
	}
}

func TestAuditFindsDuplicatesSameOptionsAndEmptyExplanations(t *testing.T) {
	dup := q("b", "сложность бинарного поиска", opt("1", "O(n)", false), opt("2", "O(log n)", true))
	same := q("c", "Что быстрее?", opt("1", "O(1)", true), opt("2", "o(1)", false))
	empty := q("d", "Пусто?", opt("1", "Да", true), opt("2", "Нет", false))
	empty.ExplanationMD = " "
	got := strings.Join(content.Audit(content.Seed{Questions: []content.Question{
		q("a", "Сложность бинарного поиска?", opt("1", "O(log n)", true), opt("2", "O(n)", false)), dup, same, empty,
	}}), "\n")
	for _, want := range []string{"b: дубль вопроса a", "c: два одинаковых варианта", "d: пустое пояснение"} {
		if !strings.Contains(got, want) {
			t.Errorf("нет %q в:\n%s", want, got)
		}
	}
}

// Банк вопросов из репозитория (content/seed): проходит проверку загрузчика и аудит, у каждого предмета КТ
// есть вопросы. До полного банка (S-04, S-07) нехватка до формата КТ только печатается.
func TestRepositorySeed(t *testing.T) {
	root := filepath.Join("..", "..", "..", "content")
	s, err := content.Read(os.DirFS(filepath.Join(root, "seed")))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range content.Audit(s) {
		t.Error(p)
	}
	raw, err := os.ReadFile(filepath.Join(root, "exam_config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Blocks map[string]struct {
			Subject string `json:"subject"`
			Tasks   int    `json:"tasks"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	have := content.QuestionsBySubject(s)
	for _, b := range cfg.Blocks {
		switch n := have[b.Subject]; {
		case n == 0:
			t.Errorf("%s: в seed нет ни одного вопроса", b.Subject)
		case n < b.Tasks:
			t.Logf("%s: %d вопросов, на вариант КТ нужно %d", b.Subject, n, b.Tasks)
		}
	}
}
