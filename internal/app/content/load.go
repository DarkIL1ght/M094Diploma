package content

import (
	"context"
	"fmt"
	"io/fs"
	"strings"
)

// Store записывает проверенный seed: добавляет новое, переписывает изменившееся (по SumHash), выводит из оборота
// пропавшее и возвращает в оборот вернувшееся. Всё или ничего — одной транзакцией.
type Store interface {
	Replace(ctx context.Context, s Seed) (Report, error)
}

// Load читает и проверяет seed из fsys и записывает его в store. Неверный seed в хранилище не попадает.
func Load(ctx context.Context, fsys fs.FS, store Store) (Report, error) {
	s, err := Read(fsys)
	if err != nil {
		return Report{}, err
	}
	return store.Replace(ctx, s)
}

// Report — что сделала загрузка с каждым видом строк. У общих материалов нет retired_at: они не выводятся.
type Report struct {
	Topics, Sections, Materials, Questions Counts
}

// Counts — сколько строк добавлено, изменено (новый sum_hash), осталось как было, возвращено в оборот
// и выведено из оборота (нет в seed).
type Counts struct {
	Added, Changed, Unchanged, Restored, Retired int
}

// Changed сообщает, изменила ли загрузка хоть что-то.
func (r Report) Changed() bool {
	for _, c := range []Counts{r.Topics, r.Sections, r.Materials, r.Questions} {
		if c.Added+c.Changed+c.Restored+c.Retired > 0 {
			return true
		}
	}
	return false
}

func (r Report) String() string {
	var b strings.Builder
	for _, row := range []struct {
		name string
		c    Counts
	}{{"темы", r.Topics}, {"разделы", r.Sections}, {"материалы", r.Materials}, {"вопросы", r.Questions}} {
		fmt.Fprintf(&b, "%s: добавлено %d, изменено %d, без изменений %d, возвращено %d, выведено %d\n",
			row.name, row.c.Added, row.c.Changed, row.c.Unchanged, row.c.Restored, row.c.Retired)
	}
	return b.String()
}
