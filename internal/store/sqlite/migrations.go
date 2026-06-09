package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

func (s *Store) Migrate(ctx context.Context) error {
	if err := s.adoptLegacySchema(ctx); err != nil {
		return err
	}
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

func (s *Store) adoptLegacySchema(ctx context.Context) error {
	hasGoose, err := s.tableExists(ctx, "goose_db_version")
	if err != nil || hasGoose {
		return err
	}
	complete, foundAny, err := s.legacySchemaState(ctx)
	if err != nil {
		return err
	}
	if !foundAny {
		return nil
	}
	if !complete {
		return errors.New("sqlite schema is partially initialized but has no goose migration version")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if _, err := tx.ExecContext(ctx, `CREATE TABLE goose_db_version (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		version_id INTEGER NOT NULL,
		is_applied INTEGER NOT NULL,
		tstamp TIMESTAMP DEFAULT (datetime('now'))
	)`); err != nil {
		return err
	}
	for _, version := range []int64{0, 1} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO goose_db_version(version_id, is_applied) VALUES(?, ?)`, version, true); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) legacySchemaState(ctx context.Context) (complete bool, foundAny bool, err error) {
	for _, name := range []string{
		"pages",
		"aliases",
		"links",
		"unresolved_links",
		"chunks",
		"chunks_fts",
		"scheduler_state",
		"index_state",
	} {
		exists, err := s.tableExists(ctx, name)
		if err != nil {
			return false, false, err
		}
		foundAny = foundAny || exists
		if !exists {
			return false, foundAny, nil
		}
	}
	return true, foundAny, nil
}

func (s *Store) tableExists(ctx context.Context, name string) (bool, error) {
	var found string
	err := s.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
