package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"jobscout.ai/internal/config"
	"jobscout.ai/internal/core"
	"jobscout.ai/internal/store/memory"
)

func TestHTTPProfileImportAndStatusFlow(t *testing.T) {
	st := memory.New()
	cfg := config.Config{
		Scoring:           core.DefaultScoringConfig(),
		MaxVacancyAgeDays: 30,
		EnableTelegram:    false,
	}
	application := New(cfg, st, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server := httptest.NewServer(application.Router())
	defer server.Close()

	profile := core.CandidateProfile{
		DesiredRoles:                []string{"go backend engineer"},
		RemoteAllowed:               true,
		YearsOfCommercialExperience: 5,
	}
	postJSON(t, server.URL+"/v1/profile", profile, nil)

	importReq := ManualImportRequest{
		URL:         "https://example.com/jobs/42",
		Title:       "Go Backend Engineer",
		CompanyName: "Acme",
		Text:        "Go Backend Engineer\nAcme\nBuild backend APIs in Go",
		Location:    "Remote",
	}
	var imported storeResponse
	postJSON(t, server.URL+"/v1/vacancies/import-url", importReq, &imported)
	if imported.Vacancy.ID == "" {
		t.Fatal("expected imported vacancy id")
	}
	if imported.Vacancy.Status != core.VacancyStatusRecommended {
		t.Fatalf("expected recommended vacancy, got %s", imported.Vacancy.Status)
	}

	var recommended []storeResponse
	getJSON(t, server.URL+"/v1/vacancies/recommended", &recommended)
	if len(recommended) != 1 {
		t.Fatalf("expected one recommended vacancy, got %d", len(recommended))
	}

	var all []storeResponse
	getJSON(t, server.URL+"/v1/vacancies", &all)
	if len(all) != 1 {
		t.Fatalf("expected one vacancy in list, got %d", len(all))
	}

	patchReq := StatusUpdateRequest{Status: string(core.VacancyStatusArchived)}
	var updated storeResponse
	patchJSON(t, server.URL+"/v1/vacancies/"+imported.Vacancy.ID+"/status", patchReq, &updated)
	if updated.Vacancy.Status != core.VacancyStatusArchived {
		t.Fatalf("expected archived status, got %s", updated.Vacancy.Status)
	}

	var archived []storeResponse
	getJSON(t, server.URL+"/v1/vacancies?status=ARCHIVED", &archived)
	if len(archived) != 1 {
		t.Fatalf("expected one archived vacancy, got %d", len(archived))
	}
}

func TestHTTPVacanciesEmptyListIsJSONArray(t *testing.T) {
	st := memory.New()
	cfg := config.Config{
		Scoring:           core.DefaultScoringConfig(),
		MaxVacancyAgeDays: 30,
		EnableTelegram:    false,
	}
	application := New(cfg, st, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server := httptest.NewServer(application.Router())
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/vacancies")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(bytes.TrimSpace(body)); got != "[]" {
		t.Fatalf("expected empty array, got %q", got)
	}
}

type storeResponse struct {
	Vacancy core.Vacancy       `json:"vacancy"`
	Company *core.Company      `json:"company"`
	Match   *core.VacancyMatch `json:"match"`
}

func postJSON(t *testing.T, url string, payload any, dest any) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(data))
	}
	if dest != nil {
		if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
			t.Fatal(err)
		}
	}
}

func getJSON(t *testing.T, url string, dest any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(data))
	}
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		t.Fatal(err)
	}
}

func patchJSON(t *testing.T, url string, payload any, dest any) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(data))
	}
	if dest != nil {
		if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
			t.Fatal(err)
		}
	}
}
