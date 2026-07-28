package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDeterministicResumeSelector(t *testing.T) {
	selector := NewDeterministicResumeSelector()
	base := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)

	t.Run("go", func(t *testing.T) {
		resume := mustSelectResume(t, selector, ResumeSelectionInput{
			Vacancy: Vacancy{
				Title:           "Senior Go Backend Engineer",
				NormalizedTitle: "senior go backend engineer",
				Skills:          []string{"Go", "PostgreSQL"},
			},
			Resumes: []Resume{
				{ID: "general", CandidateProfileID: "p1", TargetRole: ResumeTargetRoleGeneralBackend, Language: ResumeLanguageRU, Name: "General", TextContent: "general", Skills: []string{"sql"}, IsActive: true, CreatedAt: base.Add(time.Hour)},
				{ID: "go-1", CandidateProfileID: "p1", TargetRole: ResumeTargetRoleGoBackend, Language: ResumeLanguageRU, Name: "Go", TextContent: "go", Skills: []string{"Go", "PostgreSQL"}, IsActive: true, CreatedAt: base},
			},
		})
		if resume.ID != "go-1" {
			t.Fatalf("expected go resume, got %s", resume.ID)
		}
	})

	t.Run("python", func(t *testing.T) {
		resume := mustSelectResume(t, selector, ResumeSelectionInput{
			Vacancy: Vacancy{
				Title:           "Python Backend Developer",
				NormalizedTitle: "python backend developer",
				Skills:          []string{"Python", "Django"},
			},
			Resumes: []Resume{
				{ID: "python-1", CandidateProfileID: "p1", TargetRole: ResumeTargetRolePythonBackend, Language: ResumeLanguageRU, Name: "Python", TextContent: "python", Skills: []string{"Python", "Django"}, IsActive: true, CreatedAt: base},
				{ID: "go-1", CandidateProfileID: "p1", TargetRole: ResumeTargetRoleGoBackend, Language: ResumeLanguageRU, Name: "Go", TextContent: "go", Skills: []string{"Go"}, IsActive: true, CreatedAt: base.Add(time.Hour)},
			},
		})
		if resume.ID != "python-1" {
			t.Fatalf("expected python resume, got %s", resume.ID)
		}
	})

	t.Run("system analyst", func(t *testing.T) {
		resume := mustSelectResume(t, selector, ResumeSelectionInput{
			Vacancy: Vacancy{
				Title:           "Системный аналитик",
				NormalizedTitle: "системный аналитик",
				Skills:          []string{"BPMN", "SQL"},
			},
			Resumes: []Resume{
				{ID: "sa-1", CandidateProfileID: "p1", TargetRole: ResumeTargetRoleSystemAnalyst, Language: ResumeLanguageRU, Name: "SA", TextContent: "sa", Skills: []string{"BPMN", "SQL"}, IsActive: true, CreatedAt: base},
				{ID: "general", CandidateProfileID: "p1", TargetRole: ResumeTargetRoleGeneralBackend, Language: ResumeLanguageRU, Name: "General", TextContent: "general", Skills: []string{"SQL"}, IsActive: true, CreatedAt: base.Add(time.Hour)},
			},
		})
		if resume.ID != "sa-1" {
			t.Fatalf("expected system analyst resume, got %s", resume.ID)
		}
	})

	t.Run("fallback", func(t *testing.T) {
		resume := mustSelectResume(t, selector, ResumeSelectionInput{
			Vacancy: Vacancy{
				Title:           "Backend Engineer",
				NormalizedTitle: "backend engineer",
				Skills:          []string{"PostgreSQL"},
			},
			Resumes: []Resume{
				{ID: "general-1", CandidateProfileID: "p1", TargetRole: ResumeTargetRoleGeneralBackend, Language: ResumeLanguageRU, Name: "General 1", TextContent: "general 1", Skills: []string{"PostgreSQL"}, IsActive: true, CreatedAt: base},
				{ID: "go-1", CandidateProfileID: "p1", TargetRole: ResumeTargetRoleGoBackend, Language: ResumeLanguageRU, Name: "Go", TextContent: "go", Skills: []string{"Go"}, IsActive: true, CreatedAt: base.Add(time.Hour)},
			},
		})
		if resume.ID != "general-1" {
			t.Fatalf("expected general fallback resume, got %s", resume.ID)
		}
	})

	t.Run("manual override", func(t *testing.T) {
		resume := mustSelectResume(t, selector, ResumeSelectionInput{
			ManualResumeID: "manual",
			Vacancy: Vacancy{
				Title:           "Python Backend Developer",
				NormalizedTitle: "python backend developer",
			},
			Resumes: []Resume{
				{ID: "manual", CandidateProfileID: "p1", TargetRole: ResumeTargetRoleGeneralBackend, Language: ResumeLanguageEN, Name: "Manual", TextContent: "manual", Skills: []string{"Kubernetes"}, IsActive: false, CreatedAt: base},
			},
		})
		if resume.ID != "manual" {
			t.Fatalf("expected manual resume, got %s", resume.ID)
		}
	})

	t.Run("no resume", func(t *testing.T) {
		_, err := selector.Select(context.Background(), ResumeSelectionInput{
			Vacancy: Vacancy{Title: "Go Backend Engineer", NormalizedTitle: "go backend engineer", Skills: []string{"Go"}},
			Resumes: nil,
		})
		if err == nil || !strings.Contains(err.Error(), ErrNoSuitableResume.Error()) {
			t.Fatalf("expected no suitable resume error, got %v", err)
		}
	})

	t.Run("inactive ignored", func(t *testing.T) {
		_, err := selector.Select(context.Background(), ResumeSelectionInput{
			Vacancy: Vacancy{Title: "Go Backend Engineer", NormalizedTitle: "go backend engineer", Skills: []string{"Go"}},
			Resumes: []Resume{
				{ID: "go-1", CandidateProfileID: "p1", TargetRole: ResumeTargetRoleGoBackend, Language: ResumeLanguageRU, Name: "Go", TextContent: "go", Skills: []string{"Go"}, IsActive: false, CreatedAt: base},
			},
		})
		if err == nil {
			t.Fatal("expected error when only inactive resume exists")
		}
	})

	t.Run("multiple matching deterministic", func(t *testing.T) {
		resume := mustSelectResume(t, selector, ResumeSelectionInput{
			Vacancy: Vacancy{Title: "Go Backend Engineer", NormalizedTitle: "go backend engineer", Skills: []string{"Go", "PostgreSQL"}},
			Resumes: []Resume{
				{ID: "b", CandidateProfileID: "p1", TargetRole: ResumeTargetRoleGoBackend, Language: ResumeLanguageRU, Name: "B", TextContent: "b", Skills: []string{"Go"}, IsActive: true, CreatedAt: base.Add(time.Hour)},
				{ID: "a", CandidateProfileID: "p1", TargetRole: ResumeTargetRoleGoBackend, Language: ResumeLanguageRU, Name: "A", TextContent: "a", Skills: []string{"Go", "PostgreSQL"}, IsActive: true, CreatedAt: base},
			},
		})
		if resume.ID != "a" {
			t.Fatalf("expected earliest/best matching resume, got %s", resume.ID)
		}
	})
}

func TestApplicationStateMachine(t *testing.T) {
	type step struct {
		from ApplicationStatus
		to   ApplicationStatus
		ok   bool
	}
	steps := []step{
		{ApplicationStatusDraft, ApplicationStatusWaitingApproval, true},
		{ApplicationStatusWaitingApproval, ApplicationStatusApproved, true},
		{ApplicationStatusApproved, ApplicationStatusManualActionRequired, true},
		{ApplicationStatusManualActionRequired, ApplicationStatusSubmitted, true},
		{ApplicationStatusSubmitted, ApplicationStatusHRContact, true},
		{ApplicationStatusSubmitted, ApplicationStatusInterview, true},
		{ApplicationStatusHRContact, ApplicationStatusInterview, true},
		{ApplicationStatusInterview, ApplicationStatusOffer, true},
		{ApplicationStatusInterview, ApplicationStatusRejected, true},
		{ApplicationStatusDraft, ApplicationStatusSubmitted, false},
		{ApplicationStatusWaitingApproval, ApplicationStatusSubmitted, false},
		{ApplicationStatusSubmitted, ApplicationStatusOffer, false},
		{ApplicationStatusCancelled, ApplicationStatusWaitingApproval, false},
	}
	for _, tc := range steps {
		if got := CanTransitionApplicationStatus(tc.from, tc.to); got != tc.ok {
			t.Fatalf("%s -> %s: expected %v, got %v", tc.from, tc.to, tc.ok, got)
		}
	}
	if !CanTransitionApplicationStatus(ApplicationStatusSubmitted, ApplicationStatusSubmitted) {
		t.Fatal("same application status must be idempotent")
	}
}

func TestCoverLetterGeneratorAndWarnings(t *testing.T) {
	gen := NewDeterministicCoverLetterGenerator()
	vacancy := Vacancy{
		Title:            "Senior Go Backend Engineer",
		NormalizedTitle:  "senior go backend engineer",
		Description:      "Build APIs",
		Requirements:     "Go, PostgreSQL",
		Responsibilities: "Ship services",
		Skills:           []string{"Go", "PostgreSQL"},
		Grade:            "Senior",
		Currency:         "RUB",
		PublishedAt:      time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC),
	}
	match := &VacancyMatch{
		PositiveReasons: []string{"role matches go backend", "skill match: go"},
		MissingSkills:   []string{"Kafka"},
	}
	text, err := gen.Generate(context.Background(), CoverLetterInput{
		CandidateName:             "Alex",
		TargetRole:                ResumeTargetRoleGoBackend,
		CommercialExperienceYears: 5,
		ProjectExperience:         "Pet project: job tracker",
		ResumeSkills:              []string{"Go", "PostgreSQL", "Docker"},
		Vacancy:                   vacancy,
		Match:                     match,
		PositiveReasons:           match.PositiveReasons,
		MissingSkills:             match.MissingSkills,
		CompanyName:               "Acme",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := len([]rune(text)); got < 500 || got > 1200 {
		t.Fatalf("unexpected letter length: %d", got)
	}
	if !strings.Contains(text, "Acme") || !strings.Contains(text, "Senior Go Backend Engineer") {
		t.Fatalf("expected company and vacancy in letter: %s", text)
	}
	if strings.Contains(strings.ToLower(text), "kafka") {
		t.Fatalf("letter must not invent missing skills as claims: %s", text)
	}

	profile := CandidateProfile{YearsOfCommercialExperience: 2}
	resume := Resume{TextContent: "Pet project: job tracker", Skills: []string{"Go"}}
	warnings := BuildApplicationWarnings(profile, resume, vacancy, match, ExtractProjectExperience(resume.TextContent))
	if len(warnings) == 0 {
		t.Fatal("expected warnings to be produced")
	}
	if !containsWarning(warnings, "Не все навыки") || !containsWarning(warnings, "проектный опыт") || !containsWarning(warnings, "зарплату") || !containsWarning(warnings, "Разрешение на работу") {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
}

func mustSelectResume(t *testing.T, selector ResumeSelector, input ResumeSelectionInput) Resume {
	t.Helper()
	got, err := selector.Select(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func containsWarning(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
