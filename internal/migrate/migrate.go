package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Migration struct {
	Version string
	Name    string
	Path    string
}

func Up(ctx context.Context, db *sql.DB, migrationsDir string) error {
	if err := ensureTable(ctx, db); err != nil {
		return err
	}
	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return err
	}
	migrations, err := discover(migrationsDir)
	if err != nil {
		return err
	}
	for _, m := range migrations {
		if _, ok := applied[m.Version]; ok {
			continue
		}
		if err := applyOne(ctx, db, m); err != nil {
			return err
		}
	}
	return nil
}

func ensureTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`)
	return err
}

func appliedVersions(ctx context.Context, db *sql.DB) (map[string]struct{}, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]struct{})
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		out[version] = struct{}{}
	}
	return out, rows.Err()
}

func discover(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errorsIsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	migrations := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") || strings.HasSuffix(name, ".down.sql") {
			continue
		}
		base := strings.TrimSuffix(name, ".sql")
		parts := strings.SplitN(base, "_", 2)
		if len(parts) != 2 {
			continue
		}
		migrations = append(migrations, Migration{
			Version: parts[0],
			Name:    parts[1],
			Path:    filepath.Join(dir, name),
		})
	}
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})
	return migrations, nil
}

func applyOne(ctx context.Context, db *sql.DB, migration Migration) error {
	body, err := os.ReadFile(migration.Path)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, string(body)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("apply migration %s: %w", migration.Path, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, name) VALUES ($1, $2)`, migration.Version, migration.Name); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func errorsIsNotExist(err error) bool {
	return os.IsNotExist(err) || strings.Contains(strings.ToLower(err.Error()), "file does not exist")
}
