package sqlite

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

func (s *Store) Migrate(ctx context.Context) error {
	migrationsFS, err := fs.Sub(embeddedMigrations, "migrations")
	if err != nil {
		return fmt.Errorf("sqlite migrations subdirectory: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, s.db, migrationsFS)
	if err != nil {
		return fmt.Errorf("sqlite migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("sqlite migrations up: %w", err)
	}
	return nil
}
