package core

import (
	"testing"
	"time"
)

func TestNormalizeVacancy(t *testing.T) {
	raw := RawVacancy{
		Title:            "Senior Go Developer",
		CompanyName:      "Acme",
		Description:      "<p>Build APIs</p>",
		Requirements:     "<ul><li>Go</li></ul>",
		Responsibilities: "<div>Ship things</div>",
		Skills:           []string{"Go", "Postgres", "Go"},
	}
	normalized := NormalizeVacancy(raw)
	if normalized.NormalizedTitle != "senior go developer" {
		t.Fatalf("unexpected normalized title: %q", normalized.NormalizedTitle)
	}
	if normalized.StrippedDescription != "Build APIs" {
		t.Fatalf("unexpected description: %q", normalized.StrippedDescription)
	}
	if normalized.RawVacancy.Title != raw.Title {
		t.Fatalf("raw vacancy should be preserved")
	}
	if normalized.ContentHash == "" {
		t.Fatal("content hash must not be empty")
	}
}

func TestApplyHardFilters(t *testing.T) {
	profile := CandidateProfile{
		DesiredRoles:     []string{"go backend"},
		DesiredLocations: []string{"remote"},
		RemoteAllowed:    true,
		StopWords:        []string{"intern"},
	}
	vacancy := Vacancy{
		Title:           "Go Backend Developer",
		NormalizedTitle: "go backend developer",
		Location:        "remote",
		RemoteType:      "remote",
		PublishedAt:     nowTest(),
		Description:     "Great role",
	}
	company := Company{DisplayName: "Acme"}
	result := ApplyHardFilters(profile, vacancy, company, nowTest(), FilterConfig{MaxVacancyAgeDays: 10})
	if !result.Passed {
		t.Fatalf("expected hard filters to pass, got %+v", result)
	}
	vacancy.Title = "Intern Go Developer"
	vacancy.NormalizedTitle = "intern go developer"
	result = ApplyHardFilters(profile, vacancy, company, nowTest(), FilterConfig{MaxVacancyAgeDays: 10})
	if result.Passed {
		t.Fatal("expected hard filters to fail")
	}
}

func TestScoreVacancy(t *testing.T) {
	minSalary := 200000
	profile := CandidateProfile{
		DesiredRoles:     []string{"go backend"},
		PrimarySkills:    []string{"go"},
		DesiredLocations: []string{"remote"},
		RemoteAllowed:    true,
		MinimumSalary:    &minSalary,
		Currencies:       []string{"RUR"},
	}
	vacancy := Vacancy{
		Title:           "Go Backend Engineer",
		NormalizedTitle: "go backend engineer",
		Location:        "remote",
		RemoteType:      "remote",
		SalaryFrom:      intPtr(250000),
		Currency:        "RUR",
		Skills:          []string{"go"},
		EmploymentType:  "full time",
		Grade:           "senior",
	}
	company := Company{DisplayName: "Acme"}
	result := ScoreVacancy(profile, vacancy, company, DefaultScoringConfig(), true)
	if result.TotalScore < 80 {
		t.Fatalf("expected high score, got %+v", result)
	}
	if result.Recommendation != RecommendationApply {
		t.Fatalf("expected apply recommendation, got %s", result.Recommendation)
	}
}

func TestVacancyTransitions(t *testing.T) {
	if CanTransitionVacancyStatus(VacancyStatusArchived, VacancyStatusRecommended) {
		t.Fatal("archived vacancy must not transition back to recommended")
	}
	if !CanTransitionVacancyStatus(VacancyStatusRecommended, VacancyStatusArchived) {
		t.Fatal("recommended vacancy should be archivable")
	}
}

func nowTest() time.Time {
	return time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
}

func intPtr(v int) *int { return &v }
