package testutil

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"jobscout.ai/internal/migrate"
)

func OpenPostgres(t testing.TB) *sql.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for integration tests")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}

	root := ProjectRoot(t)
	if err := migrate.Up(ctx, db, filepath.Join(root, "migrations")); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	ResetPostgres(t, db)
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func ResetPostgres(t testing.TB, db *sql.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := db.ExecContext(ctx, `
TRUNCATE TABLE audit_events, applications, resumes, vacancy_matches, vacancies, companies, job_sources, candidate_profiles RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatal(err)
	}
}

func ProjectRoot(t testing.TB) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve helper path")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("could not locate project root")
	return ""
}
