//go:build integration

package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"jobscout.ai/internal/config"
	"jobscout.ai/internal/core"
	"jobscout.ai/internal/integrations/hh"
	"jobscout.ai/internal/store"
	postgresstore "jobscout.ai/internal/store/postgres"
	"jobscout.ai/internal/testutil"
)

func TestImportManualVacancyLookupErrorDoesNotWrite(t *testing.T) {
	application, db := newIntegrationApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/vacancies/123":
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
		default:
			http.NotFound(w, r)
		}
	})

	if _, err := application.SaveProfile(context.Background(), &core.CandidateProfile{
		DesiredRoles:                []string{"Go Backend Engineer"},
		DesiredGrades:               []string{"Senior"},
		PrimarySkills:               []string{"Go"},
		DesiredLocations:            []string{"Remote"},
		RemoteAllowed:               true,
		YearsOfCommercialExperience: 6,
	}); err != nil {
		t.Fatal(err)
	}

	_, err := application.ImportManualVacancy(context.Background(), ManualImportRequest{
		URL: "https://hh.ru/vacancy/123",
	})
	if err == nil {
		t.Fatal("expected lookup error")
	}

	if got := mustCount(t, db, "companies"); got != 0 {
		t.Fatalf("expected no companies, got %d", got)
	}
	if got := mustCount(t, db, "vacancies"); got != 0 {
		t.Fatalf("expected no vacancies, got %d", got)
	}
	if got := mustCount(t, db, "vacancy_matches"); got != 0 {
		t.Fatalf("expected no vacancy matches, got %d", got)
	}
}

func TestImportManualVacancyRollsBackOnMatchFailure(t *testing.T) {
	db, realStore := openIntegrationStore(t)
	client, server := newHHClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/vacancies/123":
			writeHHJSON(t, w, hh.VacancyDetails{
				ID:           "123",
				Name:         "Go Backend Engineer",
				AlternateURL: "https://hh.ru/vacancy/123",
				Description:  "<p>Build APIs</p>",
				PublishedAt:  "2026-07-28T10:00:00Z",
				Snippet: struct {
					Requirement    string `json:"requirement"`
					Responsibility string `json:"responsibility"`
				}{Requirement: "Go", Responsibility: "Ship"},
				KeySkills: []struct {
					Name string `json:"name"`
				}{{Name: "Go"}},
				Experience: struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}{ID: "senior", Name: "Senior"},
				Schedule: struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}{ID: "remote", Name: "Remote"},
				Employment: struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}{ID: "full", Name: "Full time"},
				Area: struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}{ID: "remote", Name: "Remote"},
				Employer: struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}{ID: "1", Name: "Acme"},
			})
		default:
			http.NotFound(w, r)
		}
	})
	defer server.Close()

	failing := &failingStore{Store: realStore}
	application := New(integrationConfig(), failing, client, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := application.SeedCoreSources(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := application.SaveProfile(context.Background(), &core.CandidateProfile{
		DesiredRoles:                []string{"Go Backend Engineer"},
		DesiredGrades:               []string{"Senior"},
		PrimarySkills:               []string{"Go"},
		DesiredLocations:            []string{"Remote"},
		RemoteAllowed:               true,
		YearsOfCommercialExperience: 6,
	}); err != nil {
		t.Fatal(err)
	}

	_, err := application.ImportManualVacancy(context.Background(), ManualImportRequest{
		URL: "https://hh.ru/vacancy/123",
	})
	if err == nil || !strings.Contains(err.Error(), "match insert failed") {
		t.Fatalf("expected injected failure, got %v", err)
	}

	if got := mustCount(t, db, "companies"); got != 0 {
		t.Fatalf("expected company rollback, got %d rows", got)
	}
	if got := mustCount(t, db, "vacancies"); got != 0 {
		t.Fatalf("expected vacancy rollback, got %d rows", got)
	}
	if got := mustCount(t, db, "vacancy_matches"); got != 0 {
		t.Fatalf("expected match rollback, got %d rows", got)
	}
}

func TestRunSearchImportsAndIsIdempotent(t *testing.T) {
	_, st := openIntegrationStore(t)
	searchCalls := 0
	detailCalls := 0
	client, server := newHHClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/vacancies":
			searchCalls++
			if got := r.URL.Query().Get("host"); got != "hh.ru" {
				t.Fatalf("unexpected host query: %q", got)
			}
			if got := r.URL.Query().Get("page"); got != "0" {
				t.Fatalf("unexpected page: %q", got)
			}
			if got := r.URL.Query().Get("per_page"); got != "20" {
				t.Fatalf("unexpected per_page: %q", got)
			}
			if got := r.URL.Query().Get("text"); got == "" {
				t.Fatal("expected non-empty query text")
			}
			writeHHJSON(t, w, hh.SearchResponse{
				Found:   1,
				Pages:   1,
				PerPage: 20,
				Items: []hh.SearchVacancySummary{{
					ID:           "123",
					Name:         "Go Backend Engineer",
					AlternateURL: "https://hh.ru/vacancy/123",
					PublishedAt:  "2026-07-28T10:00:00Z",
				}},
			})
		case "/vacancies/123":
			detailCalls++
			writeHHJSON(t, w, hh.VacancyDetails{
				ID:           "123",
				Name:         "Go Backend Engineer",
				AlternateURL: "https://hh.ru/vacancy/123",
				Description:  "<p>Build APIs</p>",
				PublishedAt:  "2026-07-28T10:00:00Z",
				Snippet: struct {
					Requirement    string `json:"requirement"`
					Responsibility string `json:"responsibility"`
				}{Requirement: "Go", Responsibility: "Ship"},
				KeySkills: []struct {
					Name string `json:"name"`
				}{{Name: "Go"}},
				Experience: struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}{ID: "senior", Name: "Senior"},
				Schedule: struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}{ID: "remote", Name: "Remote"},
				Employment: struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}{ID: "full", Name: "Full time"},
				Area: struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}{ID: "remote", Name: "Remote"},
				Employer: struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}{ID: "1", Name: "Acme"},
			})
		default:
			http.NotFound(w, r)
		}
	})
	defer server.Close()

	application := New(integrationConfig(), st, client, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := application.SeedCoreSources(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := application.SaveProfile(context.Background(), &core.CandidateProfile{
		DesiredRoles:                []string{"Go Backend Engineer"},
		DesiredGrades:               []string{"Senior"},
		PrimarySkills:               []string{"Go"},
		DesiredLocations:            []string{"Remote"},
		RemoteAllowed:               true,
		YearsOfCommercialExperience: 6,
	}); err != nil {
		t.Fatal(err)
	}

	summary, err := application.RunSearch(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Found != 1 || summary.Imported != 1 || summary.Recommended != 1 || summary.Errors != 0 {
		t.Fatalf("unexpected search summary: %+v", summary)
	}

	items, err := st.ListVacancies(context.Background(), store.ListVacanciesFilter{Page: 0, PerPage: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one vacancy, got %d", len(items))
	}
	if items[0].Vacancy.Status != core.VacancyStatusRecommended {
		t.Fatalf("expected recommended status, got %s", items[0].Vacancy.Status)
	}
	if items[0].Company == nil || items[0].Company.DisplayName != "Acme" {
		t.Fatalf("expected company relation, got %+v", items[0].Company)
	}
	if items[0].Match == nil || items[0].Match.TotalScore != 100 {
		t.Fatalf("expected deterministic match score, got %+v", items[0].Match)
	}

	recommended, err := st.ListRecommendedVacancies(context.Background(), store.ListVacanciesFilter{Page: 0, PerPage: 20}, integrationConfig().Scoring.ReviewThreshold)
	if err != nil {
		t.Fatal(err)
	}
	if len(recommended) != 1 {
		t.Fatalf("expected one recommended vacancy, got %d", len(recommended))
	}

	firstID := items[0].Vacancy.ID
	firstScore := items[0].Match.TotalScore
	secondSummary, err := application.RunSearch(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if secondSummary.Imported != 1 || secondSummary.Duplicates != 0 || secondSummary.Recommended != 1 {
		t.Fatalf("unexpected second summary: %+v", secondSummary)
	}

	again, err := st.ListVacancies(context.Background(), store.ListVacanciesFilter{Page: 0, PerPage: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 1 {
		t.Fatalf("expected one vacancy after second run, got %d", len(again))
	}
	if again[0].Vacancy.ID != firstID {
		t.Fatalf("expected stable vacancy id, got %s and %s", firstID, again[0].Vacancy.ID)
	}
	if again[0].Match == nil || again[0].Match.TotalScore != firstScore {
		t.Fatalf("expected deterministic score across runs, got %+v", again[0].Match)
	}
	if searchCalls != 2 || detailCalls != 2 {
		t.Fatalf("expected two search and detail calls, got search=%d detail=%d", searchCalls, detailCalls)
	}
}

func openIntegrationStore(t *testing.T) (*sql.DB, *postgresstore.Store) {
	t.Helper()
	db := testutil.OpenPostgres(t)
	return db, postgresstore.New(db)
}

func newHHClient(t *testing.T, handler http.HandlerFunc) (*hh.Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	client, err := hh.NewClient(server.URL, "JobScout AI/1.0 (owner@example.com)", "", "hh.ru", 2*time.Second, 0)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	client.SetSleeper(func(context.Context, time.Duration) error { return nil })
	t.Cleanup(server.Close)
	return client, server
}

func newIntegrationApp(t *testing.T, handler http.HandlerFunc) (*App, *sql.DB) {
	t.Helper()
	db, st := openIntegrationStore(t)
	client, server := newHHClient(t, handler)
	t.Cleanup(server.Close)
	application := New(integrationConfig(), st, client, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := application.SeedCoreSources(context.Background()); err != nil {
		t.Fatal(err)
	}
	return application, db
}

func integrationConfig() config.Config {
	return config.Config{
		AppName:           "JobScout AI",
		OwnerEmail:        "owner@example.com",
		Scoring:           core.DefaultScoringConfig(),
		MaxVacancyAgeDays: 45,
		HHPageSize:        20,
		HHMaxPages:        2,
		SearchTimeoutSec:  2,
		EnableTelegram:    false,
	}
}

func writeHHJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatal(err)
	}
}

func mustCount(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(context.Background(), "SELECT count(*) FROM "+table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

type failingStore struct {
	*postgresstore.Store
}

func (s failingStore) WithinImportTransaction(ctx context.Context, fn func(store.ImportStore) error) error {
	return s.Store.WithinImportTransaction(ctx, func(txStore store.ImportStore) error {
		return fn(failingImportStore{ImportStore: txStore})
	})
}

type failingImportStore struct {
	store.ImportStore
}

func (s failingImportStore) UpsertVacancyMatch(ctx context.Context, match *core.VacancyMatch) error {
	return errors.New("match insert failed")
}
