// Package db отдаёт миграции как встроенную файловую систему: бинарь migrator
// самодостаточен, и в контейнер не нужно монтировать каталог с .sql.
//
// ADR: docs/adr/0006-migracii-bibliotekoy.md — почему без внешнего migrate CLI.
package db

import "embed"

//go:embed migrations/*.sql
var Migrations embed.FS
