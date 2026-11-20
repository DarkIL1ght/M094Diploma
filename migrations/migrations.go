// Package migrations — схема базы (S-03): SQL-миграции goose, встроенные в бинарник.
//
// Файлы NNNNN_имя.sql применяются по порядку номеров, у каждого есть Up и Down. Применённая миграция не
// меняется: правка схемы — новый файл со следующим номером. Описание таблиц и ER-схема — docs/db.md.
// Запуск — подкоманда сервера `server migrate up|down|status` (make migrate-up).
package migrations

import (
	"database/sql"
	"embed"

	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var files embed.FS

// NewProvider готовит goose к работе с базой db. Глобальное состояние goose не используется:
// провайдеров может быть несколько (например, по схеме на тест).
func NewProvider(db *sql.DB, opts ...goose.ProviderOption) (*goose.Provider, error) {
	opts = append([]goose.ProviderOption{goose.WithDisableGlobalRegistry(true)}, opts...)
	return goose.NewProvider(goose.DialectPostgres, db, files, opts...)
}
