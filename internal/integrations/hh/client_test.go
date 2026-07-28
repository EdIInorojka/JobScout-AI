//go:build integration

package hh

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"jobscout.ai/internal/core"
)

func TestClientSearchAndDetailHappyPath(t *testing.T) {
	var searchCalls, detailCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/vacancies":
			searchCalls++
			if got := r.URL.Query().Get("host"); got != "hh.ru" {
				t.Fatalf("unexpected host query: %q", got)
			}
			if got := r.URL.Query().Get("page"); got != "2" {
				t.Fatalf("unexpected page query: %q", got)
			}
			if got := r.URL.Query().Get("per_page"); got != "50" {
				t.Fatalf("unexpected per_page query: %q", got)
			}
			if got := r.URL.Query().Get("text"); got != "Go backend engineer" {
				t.Fatalf("unexpected text query: %q", got)
			}
			if got := r.Header.Get("User-Agent"); got != "JobScout AI/1.0 (owner@example.com)" {
				t.Fatalf("unexpected user agent: %q", got)
			}
			if got := r.Header.Get("HH-User-Agent"); got != "JobScout AI/1.0 (owner@example.com)" {
				t.Fatalf("unexpected HH user agent: %q", got)
			}
			writeJSON(t, w, SearchResponse{
				Found:   1,
				Pages:   1,
				PerPage: 50,
				Items: []SearchVacancySummary{{
					ID:           "123",
					Name:         "Go Backend Engineer",
					AlternateURL: "https://hh.ru/vacancy/123",
					PublishedAt:  "2026-07-28T10:00:00Z",
				}},
			})
		case "/vacancies/123":
			detailCalls++
			writeJSON(t, w, VacancyDetails{
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
				}{{Name: "Go"}, {Name: "Postgres"}},
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
				Salary: &struct {
					From     *int   `json:"from"`
					To       *int   `json:"to"`
					Currency string `json:"currency"`
					Gross    bool   `json:"gross"`
				}{From: intPtr(220000), To: intPtr(320000), Currency: "RUR", Gross: false},
				Employer: struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}{ID: "1", Name: "Acme"},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "JobScout AI/1.0 (owner@example.com)", "", "hh.ru", 2*time.Second, 0)
	if err != nil {
		t.Fatal(err)
	}
	client.SetSleeper(func(context.Context, time.Duration) error { return nil })

	search, err := client.SearchVacancies(context.Background(), "Go backend engineer", 2, 50)
	if err != nil {
		t.Fatal(err)
	}
	if search.Found != 1 || len(search.Items) != 1 {
		t.Fatalf("unexpected search response: %+v", search)
	}

	details, err := client.GetVacancy(context.Background(), "123")
	if err != nil {
		t.Fatal(err)
	}
	raw := details.ToRawVacancy("source-1", "123", "https://hh.ru/vacancy/123", time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC))
	if raw.SourceType != core.SourceTypeHeadhunterAPI || raw.Title != "Go Backend Engineer" || raw.CompanyName != "Acme" {
		t.Fatalf("unexpected raw vacancy: %+v", raw)
	}
	if len(raw.Skills) != 2 || raw.Skills[0] != "Go" {
		t.Fatalf("unexpected raw skills: %+v", raw.Skills)
	}
	if raw.RemoteType != "remote" || raw.SalaryFrom == nil || raw.SalaryTo == nil {
		t.Fatalf("unexpected raw vacancy mapping: %+v", raw)
	}
	if searchCalls != 1 || detailCalls != 1 {
		t.Fatalf("expected one search and detail call, got search=%d detail=%d", searchCalls, detailCalls)
	}
}

func TestClientDoesNotRetryPermanentErrors(t *testing.T) {
	t.Run("400", func(t *testing.T) {
		client, calls := clientWithScriptedResponses(t, func(w http.ResponseWriter, r *http.Request, call int) {
			http.Error(w, "bad request", http.StatusBadRequest)
		})
		_, err := client.SearchVacancies(context.Background(), "go", 0, 20)
		if err == nil || calls() != 1 {
			t.Fatalf("expected one attempt and error, got err=%v calls=%d", err, calls())
		}
	})

	t.Run("403", func(t *testing.T) {
		client, calls := clientWithScriptedResponses(t, func(w http.ResponseWriter, r *http.Request, call int) {
			http.Error(w, "captcha", http.StatusForbidden)
		})
		_, err := client.SearchVacancies(context.Background(), "go", 0, 20)
		if !errors.Is(err, ErrCaptchaRequired) || calls() != 1 {
			t.Fatalf("expected captcha error and one attempt, got err=%v calls=%d", err, calls())
		}
	})

	t.Run("404", func(t *testing.T) {
		client, calls := clientWithScriptedResponses(t, func(w http.ResponseWriter, r *http.Request, call int) {
			http.NotFound(w, r)
		})
		_, err := client.SearchVacancies(context.Background(), "go", 0, 20)
		if err == nil || calls() != 1 {
			t.Fatalf("expected one attempt and error, got err=%v calls=%d", err, calls())
		}
	})

	t.Run("invalid-json", func(t *testing.T) {
		client, calls := clientWithScriptedResponses(t, func(w http.ResponseWriter, r *http.Request, call int) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{bad json"))
		})
		_, err := client.SearchVacancies(context.Background(), "go", 0, 20)
		if err == nil || calls() != 1 {
			t.Fatalf("expected one attempt and decode error, got err=%v calls=%d", err, calls())
		}
	})
}

func TestClientRetriesTransientErrors(t *testing.T) {
	t.Run("429", func(t *testing.T) {
		client, calls := clientWithScriptedResponses(t, func(w http.ResponseWriter, r *http.Request, call int) {
			if call == 0 {
				w.Header().Set("Retry-After", "0")
				http.Error(w, "rate limited", http.StatusTooManyRequests)
				return
			}
			writeJSON(t, w, SearchResponse{Found: 1, Pages: 1, PerPage: 20})
		})
		_, err := client.SearchVacancies(context.Background(), "go", 0, 20)
		if err != nil || calls() != 2 {
			t.Fatalf("expected retry after 429, got err=%v calls=%d", err, calls())
		}
	})

	t.Run("503", func(t *testing.T) {
		client, calls := clientWithScriptedResponses(t, func(w http.ResponseWriter, r *http.Request, call int) {
			if call < 2 {
				http.Error(w, "upstream down", http.StatusServiceUnavailable)
				return
			}
			writeJSON(t, w, SearchResponse{Found: 1, Pages: 1, PerPage: 20})
		})
		_, err := client.SearchVacancies(context.Background(), "go", 0, 20)
		if err != nil || calls() != 3 {
			t.Fatalf("expected retries for 503, got err=%v calls=%d", err, calls())
		}
	})
}

func TestClientTimeoutAndEmptyResult(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			writeJSON(t, w, SearchResponse{Found: 0, Pages: 0, PerPage: 20})
		}))
		defer server.Close()

		client, err := NewClient(server.URL, "JobScout AI/1.0 (owner@example.com)", "", "hh.ru", 20*time.Millisecond, 0)
		if err != nil {
			t.Fatal(err)
		}
		client.SetSleeper(func(context.Context, time.Duration) error { return nil })
		_, err = client.SearchVacancies(context.Background(), "go", 0, 20)
		if err == nil {
			t.Fatal("expected timeout error")
		}
	})

	t.Run("empty-result", func(t *testing.T) {
		client, calls := clientWithScriptedResponses(t, func(w http.ResponseWriter, r *http.Request, call int) {
			writeJSON(t, w, SearchResponse{Found: 0, Pages: 0, PerPage: 20, Items: []SearchVacancySummary{}})
		})
		resp, err := client.SearchVacancies(context.Background(), "", 0, 20)
		if err != nil || resp.Found != 0 || len(resp.Items) != 0 || calls() != 1 {
			t.Fatalf("unexpected empty result response: err=%v resp=%+v calls=%d", err, resp, calls())
		}
	})
}

func TestParsePublishedAt(t *testing.T) {
	if got := ParsePublishedAt("2026-07-28T10:00:00Z"); got.IsZero() {
		t.Fatal("expected RFC3339 timestamp to parse")
	}
	if got := ParsePublishedAt("bad"); !got.IsZero() {
		t.Fatal("expected invalid timestamp to stay zero")
	}
}

func clientWithScriptedResponses(t *testing.T, handler func(http.ResponseWriter, *http.Request, int)) (*Client, func() int) {
	t.Helper()
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls
		calls++
		handler(w, r, call)
	}))
	t.Cleanup(server.Close)
	client, err := NewClient(server.URL, "JobScout AI/1.0 (owner@example.com)", "", "hh.ru", 2*time.Second, 0)
	if err != nil {
		t.Fatal(err)
	}
	client.SetSleeper(func(context.Context, time.Duration) error { return nil })
	return client, func() int { return calls }
}

func writeJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatal(err)
	}
}

func intPtr(v int) *int { return &v }
