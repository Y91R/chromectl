// Команда migrator накатывает и откатывает миграции. golang-migrate подключён
// библиотекой, а миграции вшиты в бинарь: отдельный CLI и монтирование каталога
// с .sql в контейнер не нужны.
//
// ADR: docs/adr/0006-migracii-bibliotekoy.md
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/spf13/cobra"

	"github.com/your-org/chrome_skill/internal/config"
	"github.com/your-org/chrome_skill/internal/db"
)

func main() {
	if err := run(); err != nil {
		slog.Default().Error("миграции не выполнены", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	root := &cobra.Command{
		Use:           "migrator",
		Short:         "Миграции схемы chrome_skill",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(upCmd(), downCmd(), versionCmd())

	// Сигнал не убивает процесс посреди миграции: незавершённая миграция
	// оставляет схему в состоянии dirty, и следующий запуск требует ручного
	// вмешательства.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return root.ExecuteContext(ctx)
}

func upCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Накатить все миграции",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withMigrator(cmd.Context(), func(m *migrate.Migrate) error {
				err := m.Up()
				if errors.Is(err, migrate.ErrNoChange) {
					slog.Default().Info("миграции уже накачены")
					return nil
				}
				return err
			})
		},
	}
}

func downCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Откатить одну миграцию",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withMigrator(cmd.Context(), func(m *migrate.Migrate) error { return m.Steps(-1) })
		},
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Текущая версия схемы",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withMigrator(cmd.Context(), func(m *migrate.Migrate) error {
				version, dirty, err := m.Version()
				if errors.Is(err, migrate.ErrNilVersion) {
					fmt.Println("схема пуста")
					return nil
				}
				if err != nil {
					return err
				}
				fmt.Printf("версия %d (dirty=%v)\n", version, dirty)
				return nil
			})
		},
	}
}

func withMigrator(ctx context.Context, fn func(*migrate.Migrate) error) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	source, err := iofs.New(db.Migrations, "migrations")
	if err != nil {
		return fmt.Errorf("чтение миграций: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, pgxURL(cfg.Database.URL))
	if err != nil {
		return fmt.Errorf("инициализация миграций: %w", err)
	}
	defer func() {
		if srcErr, dbErr := m.Close(); srcErr != nil || dbErr != nil {
			slog.Default().Warn("закрытие миграций", slog.Any("source", srcErr), slog.Any("database", dbErr))
		}
	}()

	// Сигнал переводится в GracefulStop: migrate дорабатывает текущую миграцию
	// и останавливается на границе, не оставляя схему dirty. Горутина живёт
	// ровно столько, сколько выполняется команда.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			slog.Default().Warn("получен сигнал: миграции остановятся после текущего шага")
			m.GracefulStop <- true
		case <-done:
		}
	}()

	return fn(m)
}

// pgxURL: golang-migrate выбирает драйвер по схеме URL, и обычный postgres://
// он отдаёт драйверу lib/pq, которого в зависимостях нет.
func pgxURL(url string) string {
	for _, prefix := range []string{"postgres://", "postgresql://"} {
		if rest, ok := strings.CutPrefix(url, prefix); ok {
			return "pgx5://" + rest
		}
	}
	return url
}
