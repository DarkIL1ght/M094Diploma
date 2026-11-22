package postgres_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DarkIL1ght/M094Diploma/internal/adapters/postgres"
	"github.com/DarkIL1ght/M094Diploma/internal/adapters/postgres/pgtest"
	"github.com/DarkIL1ght/M094Diploma/internal/app/content"
)

// seedFS — тема, раздел, материал и два вопроса по базам данных; questions можно подменить.
func seedFS(questions string) fstest.MapFS {
	if questions == "" {
		questions = `[
			{"ext_id":"q1","topic":"t","kind":"single","text_md":"Что?","explanation_md":".","section":"s","shared_material":"m",
			 "options":[{"ext_id":"a","text_md":"Да","is_correct":true},{"ext_id":"b","text_md":"Нет","is_correct":false}]},
			{"ext_id":"q2","topic":"t","kind":"multiple","text_md":"Какие?","explanation_md":".",
			 "options":[{"ext_id":"a","text_md":"1","is_correct":true},{"ext_id":"b","text_md":"2","is_correct":true}]}]`
	}
	return fstest.MapFS{
		"databases/topics.json":    {Data: []byte(`[{"ext_id":"t","title":"SQL","position":1}]`)},
		"databases/sections.json":  {Data: []byte(`[{"ext_id":"s","topic":"t","title":"SELECT","position":1,"body_md":"Текст"}]`)},
		"databases/materials.json": {Data: []byte(`[{"ext_id":"m","body_md":"| a |"}]`)},
		"databases/questions.json": {Data: []byte(questions)},
	}
}

func load(t *testing.T, st *postgres.ContentStore, fs fstest.MapFS) content.Report {
	t.Helper()
	rep, err := content.Load(context.Background(), fs, st)
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func count(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestContentLoadIsIdempotent(t *testing.T) {
	pool := pgtest.NewPool(t)
	st := postgres.NewContentStore(pool)

	rep := load(t, st, seedFS(""))
	want := content.Report{Topics: content.Counts{Added: 1}, Sections: content.Counts{Added: 1},
		Materials: content.Counts{Added: 1}, Questions: content.Counts{Added: 2}}
	if rep != want {
		t.Fatalf("первая загрузка: %+v", rep)
	}
	if n := count(t, pool, `select count(*) from question_options`); n != 4 {
		t.Errorf("вариантов %d", n)
	}
	if n := count(t, pool, `select count(*) from questions q join sections s on s.id = q.section_id
		join shared_materials m on m.id = q.shared_material_id where q.ext_id = 'q1'`); n != 1 {
		t.Error("ссылки вопроса на раздел и материал")
	}

	rep = load(t, st, seedFS(""))
	if rep.Changed() || rep.Questions.Unchanged != 2 || rep.Topics.Unchanged != 1 {
		t.Fatalf("повторная загрузка ничего не меняет: %+v", rep)
	}
}

func TestContentLoadUpdatesChangedQuestionAndOptions(t *testing.T) {
	pool := pgtest.NewPool(t)
	st := postgres.NewContentStore(pool)
	load(t, st, seedFS(""))
	var oldHash string
	_ = pool.QueryRow(context.Background(), `select sum_hash from questions where ext_id = 'q1'`).Scan(&oldHash)

	rep := load(t, st, seedFS(`[
		{"ext_id":"q1","topic":"t","kind":"single","text_md":"Что?","explanation_md":".","section":"s","shared_material":"m",
		 "options":[{"ext_id":"a","text_md":"Да","is_correct":false},{"ext_id":"c","text_md":"Может","is_correct":true}]},
		{"ext_id":"q2","topic":"t","kind":"multiple","text_md":"Какие?","explanation_md":".",
		 "options":[{"ext_id":"a","text_md":"1","is_correct":true},{"ext_id":"b","text_md":"2","is_correct":true}]}]`))
	if rep.Questions != (content.Counts{Changed: 1, Unchanged: 1}) {
		t.Fatalf("правка одного вопроса: %+v", rep.Questions)
	}
	if n := count(t, pool, `select count(*) from questions where ext_id = 'q1' and sum_hash <> $1`, oldHash); n != 1 {
		t.Error("новый sum_hash")
	}
	if n := count(t, pool, `select count(*) from question_options o join questions q on q.id = o.question_id
		where q.ext_id = 'q1' and o.ext_id = 'c' and o.is_correct and o.position = 2`); n != 1 {
		t.Error("варианты заменены")
	}
	if n := count(t, pool, `select count(*) from question_options`); n != 4 {
		t.Errorf("старые варианты удалены: %d", n)
	}
}

func TestContentLoadRetiresAndRestores(t *testing.T) {
	pool := pgtest.NewPool(t)
	st := postgres.NewContentStore(pool)
	load(t, st, seedFS(""))

	only1 := `[{"ext_id":"q1","topic":"t","kind":"single","text_md":"Что?","explanation_md":".","section":"s","shared_material":"m",
		"options":[{"ext_id":"a","text_md":"Да","is_correct":true},{"ext_id":"b","text_md":"Нет","is_correct":false}]}]`
	rep := load(t, st, seedFS(only1))
	if rep.Questions != (content.Counts{Unchanged: 1, Retired: 1}) {
		t.Fatalf("пропавший вопрос: %+v", rep.Questions)
	}
	if n := count(t, pool, `select count(*) from questions where ext_id = 'q2' and retired_at is not null`); n != 1 {
		t.Error("q2 выведен из оборота, но не удалён")
	}
	if n := count(t, pool, `select count(*) from question_options o join questions q on q.id = o.question_id where q.ext_id = 'q2'`); n != 2 {
		t.Error("варианты выведенного вопроса остаются")
	}
	if rep = load(t, st, seedFS(only1)); rep.Changed() {
		t.Errorf("выведенный второй раз не считается: %+v", rep)
	}

	rep = load(t, st, seedFS(""))
	if rep.Questions != (content.Counts{Unchanged: 1, Restored: 1}) {
		t.Fatalf("вернувшийся вопрос: %+v", rep.Questions)
	}
	if n := count(t, pool, `select count(*) from questions where retired_at is not null`); n != 0 {
		t.Error("retired_at снят")
	}
}

func TestContentLoadInvalidSeedWritesNothing(t *testing.T) {
	pool := pgtest.NewPool(t)
	st := postgres.NewContentStore(pool)
	bad := seedFS(`[{"ext_id":"q1","topic":"t","kind":"single","text_md":"?","explanation_md":".","options":[]}]`)
	if _, err := content.Load(context.Background(), bad, st); err == nil {
		t.Fatal("ждали ошибку проверки")
	}
	if n := count(t, pool, `select (select count(*) from topics) + (select count(*) from questions)`); n != 0 {
		t.Errorf("записано %d строк", n)
	}
}

// Ошибка базы посреди загрузки откатывает всё: ничего не записано наполовину.
func TestContentReplaceIsAtomic(t *testing.T) {
	pool := pgtest.NewPool(t)
	st := postgres.NewContentStore(pool)
	s, err := content.Read(seedFS(""))
	if err != nil {
		t.Fatal(err)
	}
	s.Questions[1].Subject = "physics" // мимо проверки Read: упадёт на домене subject_code
	if _, err := st.Replace(context.Background(), s); err == nil {
		t.Fatal("ждали ошибку базы")
	}
	if n := count(t, pool, `select count(*) from topics`); n != 0 {
		t.Errorf("тема осталась после отката: %d", n)
	}
}
