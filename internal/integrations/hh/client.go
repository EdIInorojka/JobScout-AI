package hh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"jobscout.ai/internal/core"
)

type Client struct {
	httpClient  *http.Client
	baseURL     *url.URL
	userAgent   string
	accessToken string
	host        string
	minDelay    time.Duration

	mu        sync.Mutex
	lastStart time.Time
}

type SearchResponse struct {
	Found   int                    `json:"found"`
	Pages   int                    `json:"pages"`
	PerPage int                    `json:"per_page"`
	Items   []SearchVacancySummary `json:"items"`
}

type SearchVacancySummary struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	AlternateURL string `json:"alternate_url"`
	PublishedAt  string `json:"published_at"`
}

type VacancyDetails struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	AlternateURL string `json:"alternate_url"`
	Description  string `json:"description"`
	PublishedAt  string `json:"published_at"`
	Snippet      struct {
		Requirement    string `json:"requirement"`
		Responsibility string `json:"responsibility"`
	} `json:"snippet"`
	KeySkills []struct {
		Name string `json:"name"`
	} `json:"key_skills"`
	Experience struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"experience"`
	Schedule struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"schedule"`
	Employment struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"employment"`
	Area struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"area"`
	Salary *struct {
		From     *int   `json:"from"`
		To       *int   `json:"to"`
		Currency string `json:"currency"`
		Gross    bool   `json:"gross"`
	} `json:"salary"`
	Employer struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"employer"`
}

func NewClient(baseURL, userAgent, accessToken, host string, timeout, minDelay time.Duration) (*Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	if host == "" {
		host = "hh.ru"
	}
	return &Client{
		httpClient:  &http.Client{Timeout: timeout},
		baseURL:     parsed,
		userAgent:   userAgent,
		accessToken: accessToken,
		host:        host,
		minDelay:    minDelay,
	}, nil
}

func (c *Client) SearchVacancies(ctx context.Context, query string, page, perPage int) (SearchResponse, error) {
	values := url.Values{}
	values.Set("host", c.host)
	values.Set("page", strconv.Itoa(page))
	values.Set("per_page", strconv.Itoa(perPage))
	if strings.TrimSpace(query) != "" {
		values.Set("text", query)
	}
	var resp SearchResponse
	if err := c.getJSON(ctx, "/vacancies", values, &resp); err != nil {
		return SearchResponse{}, err
	}
	return resp, nil
}

func (c *Client) GetVacancy(ctx context.Context, vacancyID string) (VacancyDetails, error) {
	values := url.Values{}
	values.Set("host", c.host)
	var resp VacancyDetails
	if err := c.getJSON(ctx, "/vacancies/"+url.PathEscape(vacancyID), values, &resp); err != nil {
		return VacancyDetails{}, err
	}
	return resp, nil
}

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, dest any) error {
	attempts := 3
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if err := c.throttle(ctx); err != nil {
			return err
		}
		reqURL := *c.baseURL
		reqURL.Path = strings.TrimRight(c.baseURL.Path, "/") + path
		reqURL.RawQuery = query.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", c.userAgent)
		req.Header.Set("HH-User-Agent", c.userAgent)
		req.Header.Set("Accept", "application/json")
		if c.accessToken != "" {
			req.Header.Set("Authorization", "Bearer "+c.accessToken)
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if !isTemporary(err) || attempt == attempts-1 {
				return err
			}
			if err := sleepBackoff(ctx, attempt); err != nil {
				return err
			}
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			if attempt == attempts-1 {
				return readErr
			}
			if err := sleepBackoff(ctx, attempt); err != nil {
				return err
			}
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			wait := retryAfter(resp.Header.Get("Retry-After"))
			if wait <= 0 {
				wait = time.Second
			}
			lastErr = fmt.Errorf("headhunter rate limited: %w", ErrRateLimited)
			if attempt == attempts-1 {
				return lastErr
			}
			if err := waitFor(ctx, wait); err != nil {
				return err
			}
			continue
		}
		if resp.StatusCode == http.StatusForbidden {
			return fmt.Errorf("headhunter request forbidden: %w", ErrCaptchaRequired)
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("headhunter server error: %s", resp.Status)
			if attempt == attempts-1 {
				return lastErr
			}
			if err := sleepBackoff(ctx, attempt); err != nil {
				return err
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("headhunter unexpected status %s: %s", resp.Status, strings.TrimSpace(string(body)))
		}
		if err := json.Unmarshal(body, dest); err != nil {
			return err
		}
		return nil
	}
	return lastErr
}

func (c *Client) throttle(ctx context.Context) error {
	if c.minDelay <= 0 {
		return nil
	}
	c.mu.Lock()
	now := time.Now()
	next := c.lastStart.Add(c.minDelay)
	if now.Before(next) {
		wait := next.Sub(now)
		c.mu.Unlock()
		if err := waitFor(ctx, wait); err != nil {
			return err
		}
		c.mu.Lock()
	}
	c.lastStart = time.Now()
	c.mu.Unlock()
	return nil
}

func sleepBackoff(ctx context.Context, attempt int) error {
	delay := time.Duration(attempt+1) * 500 * time.Millisecond
	return waitFor(ctx, delay)
}

func waitFor(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isTemporary(err error) bool {
	var netErr interface{ Temporary() bool }
	if errors.As(err, &netErr) {
		return netErr.Temporary()
	}
	return false
}

func retryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if t, err := http.ParseTime(value); err == nil {
		return time.Until(t)
	}
	return 0
}

var (
	ErrRateLimited     = errors.New("rate limited")
	ErrCaptchaRequired = errors.New("captcha required")
)

func ParsePublishedAt(value string) time.Time {
	formats := []string{time.RFC3339, "2006-01-02T15:04:05-0700", "2006-01-02T15:04:05Z0700"}
	for _, format := range formats {
		if t, err := time.Parse(format, value); err == nil {
			return t
		}
	}
	return time.Time{}
}

func (v VacancyDetails) ToRawVacancy(sourceID, externalID, sourceURL string, collectedAt time.Time) core.RawVacancy {
	skills := make([]string, 0, len(v.KeySkills))
	for _, item := range v.KeySkills {
		if strings.TrimSpace(item.Name) != "" {
			skills = append(skills, item.Name)
		}
	}
	var salaryFrom, salaryTo *int
	var currency string
	if v.Salary != nil {
		salaryFrom = v.Salary.From
		salaryTo = v.Salary.To
		currency = v.Salary.Currency
	}
	remoteType := v.Schedule.Name
	if strings.Contains(strings.ToLower(remoteType), "remote") || strings.Contains(strings.ToLower(v.Description), "remote") {
		remoteType = "remote"
	}
	return core.RawVacancy{
		SourceType:                    core.SourceTypeHeadhunterAPI,
		SourceID:                      sourceID,
		ExternalID:                    externalID,
		SourceURL:                     sourceURL,
		CanonicalURL:                  v.AlternateURL,
		Title:                         v.Name,
		CompanyName:                   v.Employer.Name,
		Description:                   v.Description,
		Requirements:                  v.Snippet.Requirement,
		Responsibilities:              v.Snippet.Responsibility,
		Location:                      v.Area.Name,
		RemoteType:                    remoteType,
		EmploymentType:                v.Employment.Name,
		Grade:                         v.Experience.Name,
		SalaryFrom:                    salaryFrom,
		SalaryTo:                      salaryTo,
		Currency:                      currency,
		Skills:                        skills,
		LanguageRequirements:          nil,
		WorkAuthorizationRequirements: nil,
		PublishedAt:                   ParsePublishedAt(v.PublishedAt),
		CollectedAt:                   collectedAt,
	}
}
