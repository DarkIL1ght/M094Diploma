package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DarkIL1ght/M094Diploma/internal/app/content"
)

// ContentStore — content.Store на таблицах контента (migrations/00002_content.sql). Загрузка идёт одной транзакцией:
// upsert по ext_id, строка переписывается, только если изменился sum_hash (или она возвращается в оборот);
// пропавшие из seed темы, разделы и вопросы помечаются retired_at, ничего не удаляется.
type ContentStore struct {
	pool *pgxpool.Pool
}

// NewContentStore создаёт загрузчик контента поверх пула.
func NewContentStore(pool *pgxpool.Pool) *ContentStore { return &ContentStore{pool: pool} }

var _ content.Store = (*ContentStore)(nil)

// existing — строка, уже лежащая в базе.
type existing struct {
	id      int64
	hash    string
	retired bool
}

// action — что сделать со строкой seed.
type action int

const (
	skip    action = iota // хэш прежний и строка в обороте
	insert                // строки нет
	update                // хэш изменился
	restore               // хэш прежний, но строка была выведена
)

func classify(rows map[string]existing, extID, hash string) action {
	e, ok := rows[extID]
	switch {
	case !ok:
		return insert
	case e.hash != hash:
		return update
	case e.retired:
		return restore
	}
	return skip
}

func tally(c *content.Counts, a action) {
	switch a {
	case skip:
		c.Unchanged++
	case insert:
		c.Added++
	case update:
		c.Changed++
	case restore:
		c.Restored++
	}
}

func (s *ContentStore) Replace(ctx context.Context, seed content.Seed) (content.Report, error) {
	var rep content.Report
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Один загрузчик за раз: второй ждёт конца транзакции первого, а не пишет вперемешку.
		if _, err := tx.Exec(ctx, `lock table topics, sections, shared_materials, questions in share row exclusive mode`); err != nil {
			return err
		}
		topics, err := s.topics(ctx, tx, seed.Topics, &rep.Topics)
		if err != nil {
			return fmt.Errorf("темы: %w", err)
		}
		sections, err := s.sections(ctx, tx, seed.Sections, topics, &rep.Sections)
		if err != nil {
			return fmt.Errorf("разделы: %w", err)
		}
		materials, err := s.materials(ctx, tx, seed.Materials, &rep.Materials)
		if err != nil {
			return fmt.Errorf("материалы: %w", err)
		}
		if err := s.questions(ctx, tx, seed.Questions, topics, sections, materials, &rep.Questions); err != nil {
			return fmt.Errorf("вопросы: %w", err)
		}
		return nil
	})
	if err != nil {
		return content.Report{}, err
	}
	return rep, nil
}

// readExisting читает ext_id, id, sum_hash и признак «выведена» всех строк таблицы.
func readExisting(ctx context.Context, tx pgx.Tx, table string, hasRetired bool) (map[string]existing, error) {
	retired := "false"
	if hasRetired {
		retired = "retired_at is not null"
	}
	rows, err := tx.Query(ctx, `select ext_id, id, sum_hash, `+retired+` from `+table)
	if err != nil {
		return nil, err
	}
	m := map[string]existing{}
	var extID string
	var e existing
	_, err = pgx.ForEachRow(rows, []any{&extID, &e.id, &e.hash, &e.retired}, func() error {
		m[extID] = e
		return nil
	})
	return m, err
}

// retire помечает retired_at действующие строки таблицы, которых нет в keep.
func retire(ctx context.Context, tx pgx.Tx, table string, keep []string) (int, error) {
	tag, err := tx.Exec(ctx, `update `+table+` set retired_at = now() where retired_at is null and ext_id <> all($1)`, keep)
	return int(tag.RowsAffected()), err
}

// ids — id строк по ext_id после загрузки: прежние и только что записанные.
func ids(rows map[string]existing) map[string]int64 {
	m := make(map[string]int64, len(rows))
	for k, e := range rows {
		m[k] = e.id
	}
	return m
}

func (s *ContentStore) topics(ctx context.Context, tx pgx.Tx, topics []content.Topic, c *content.Counts) (map[string]int64, error) {
	rows, err := readExisting(ctx, tx, "topics", true)
	if err != nil {
		return nil, err
	}
	keep := make([]string, 0, len(topics))
	for _, t := range topics {
		keep = append(keep, t.ExtID)
		hash := t.SumHash()
		a := classify(rows, t.ExtID, hash)
		tally(c, a)
		if a == skip {
			continue
		}
		var id int64
		if err := tx.QueryRow(ctx, `
			insert into topics (ext_id, subject, title, position, sum_hash) values ($1, $2, $3, $4, $5)
			on conflict (ext_id) do update set subject = excluded.subject, title = excluded.title,
				position = excluded.position, sum_hash = excluded.sum_hash, retired_at = null
			where topics.sum_hash is distinct from excluded.sum_hash or topics.retired_at is not null
			returning id`,
			t.ExtID, t.Subject, t.Title, t.Position, hash).Scan(&id); err != nil {
			return nil, fmt.Errorf("%s: %w", t.ExtID, err)
		}
		rows[t.ExtID] = existing{id: id, hash: hash}
	}
	if c.Retired, err = retire(ctx, tx, "topics", keep); err != nil {
		return nil, err
	}
	return ids(rows), nil
}

func (s *ContentStore) sections(ctx context.Context, tx pgx.Tx, sections []content.Section, topics map[string]int64, c *content.Counts) (map[string]int64, error) {
	rows, err := readExisting(ctx, tx, "sections", true)
	if err != nil {
		return nil, err
	}
	keep := make([]string, 0, len(sections))
	for _, sec := range sections {
		keep = append(keep, sec.ExtID)
		hash := sec.SumHash()
		a := classify(rows, sec.ExtID, hash)
		tally(c, a)
		if a == skip {
			continue
		}
		var id int64
		if err := tx.QueryRow(ctx, `
			insert into sections (ext_id, topic_id, title, position, body_md, sum_hash) values ($1, $2, $3, $4, $5, $6)
			on conflict (ext_id) do update set topic_id = excluded.topic_id, title = excluded.title,
				position = excluded.position, body_md = excluded.body_md, sum_hash = excluded.sum_hash, retired_at = null
			where sections.sum_hash is distinct from excluded.sum_hash or sections.retired_at is not null
			returning id`,
			sec.ExtID, topics[sec.Topic], sec.Title, sec.Position, sec.BodyMD, hash).Scan(&id); err != nil {
			return nil, fmt.Errorf("%s: %w", sec.ExtID, err)
		}
		rows[sec.ExtID] = existing{id: id, hash: hash}
	}
	if c.Retired, err = retire(ctx, tx, "sections", keep); err != nil {
		return nil, err
	}
	return ids(rows), nil
}

// materials: у общих материалов нет retired_at — пропавший из seed материал просто остаётся в базе.
func (s *ContentStore) materials(ctx context.Context, tx pgx.Tx, materials []content.Material, c *content.Counts) (map[string]int64, error) {
	rows, err := readExisting(ctx, tx, "shared_materials", false)
	if err != nil {
		return nil, err
	}
	for _, m := range materials {
		hash := m.SumHash()
		a := classify(rows, m.ExtID, hash)
		tally(c, a)
		if a == skip {
			continue
		}
		var id int64
		if err := tx.QueryRow(ctx, `
			insert into shared_materials (ext_id, body_md, sum_hash) values ($1, $2, $3)
			on conflict (ext_id) do update set body_md = excluded.body_md, sum_hash = excluded.sum_hash
			where shared_materials.sum_hash is distinct from excluded.sum_hash
			returning id`,
			m.ExtID, m.BodyMD, hash).Scan(&id); err != nil {
			return nil, fmt.Errorf("%s: %w", m.ExtID, err)
		}
		rows[m.ExtID] = existing{id: id, hash: hash}
	}
	return ids(rows), nil
}

func (s *ContentStore) questions(ctx context.Context, tx pgx.Tx, questions []content.Question,
	topics, sections, materials map[string]int64, c *content.Counts) error {
	rows, err := readExisting(ctx, tx, "questions", true)
	if err != nil {
		return err
	}
	ref := func(m map[string]int64, extID string) *int64 {
		if extID == "" {
			return nil
		}
		id := m[extID]
		return &id
	}
	keep := make([]string, 0, len(questions))
	for _, q := range questions {
		keep = append(keep, q.ExtID)
		hash := q.SumHash()
		a := classify(rows, q.ExtID, hash)
		tally(c, a)
		if a == skip {
			continue
		}
		var id int64
		if err := tx.QueryRow(ctx, `
			insert into questions (ext_id, subject, topic_id, kind, text_md, code_lang, code_source, shared_material_id,
				explanation_md, section_id, listen_text, listen_lang, sum_hash)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			on conflict (ext_id) do update set subject = excluded.subject, topic_id = excluded.topic_id,
				kind = excluded.kind, text_md = excluded.text_md, code_lang = excluded.code_lang,
				code_source = excluded.code_source, shared_material_id = excluded.shared_material_id,
				explanation_md = excluded.explanation_md, section_id = excluded.section_id,
				listen_text = excluded.listen_text, listen_lang = excluded.listen_lang,
				sum_hash = excluded.sum_hash, retired_at = null
			where questions.sum_hash is distinct from excluded.sum_hash or questions.retired_at is not null
			returning id`,
			q.ExtID, q.Subject, ref(topics, q.Topic), q.Kind, q.TextMD, nullable(q.CodeLang), nullable(q.CodeSource),
			ref(materials, q.SharedMaterial), q.ExplanationMD, ref(sections, q.Section),
			nullable(q.ListenText), nullable(q.ListenLang), hash).Scan(&id); err != nil {
			return fmt.Errorf("%s: %w", q.ExtID, err)
		}
		// Варианты — часть вопроса и входят в его хэш: при том же хэше (возврат в оборот) они уже верны.
		if a == restore {
			continue
		}
		if _, err := tx.Exec(ctx, `delete from question_options where question_id = $1`, id); err != nil {
			return fmt.Errorf("%s: %w", q.ExtID, err)
		}
		for _, o := range q.Options {
			if _, err := tx.Exec(ctx,
				`insert into question_options (question_id, ext_id, position, text_md, is_correct) values ($1, $2, $3, $4, $5)`,
				id, o.ExtID, o.Position, o.TextMD, o.IsCorrect); err != nil {
				return fmt.Errorf("%s, вариант %s: %w", q.ExtID, o.ExtID, err)
			}
		}
	}
	c.Retired, err = retire(ctx, tx, "questions", keep)
	return err
}

// nullable переводит пустую строку seed («поля нет») в null.
func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
