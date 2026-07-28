//go:build integration

package postgres

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"jobscout.ai/internal/core"
	"jobscout.ai/internal/store"
	"jobscout.ai/internal/testutil"
)

func TestStoreCandidateProfileRoundTrip(t *testing.T) {
	db := testutil.OpenPostgres(t)
	st := New(db)
	ctx := context.Background()

	minSalary := 250000
	profile := &core.CandidateProfile{
		DesiredRoles:                []string{"Go Backend Engineer"},
		DesiredGrades:               []string{"Senior"},
		PrimarySkills:               []string{"Go", "Postgres"},
		SecondarySkills:             []string{"Docker"},
		ExcludedSkills:              []string{"PHP"},
		DesiredLocations:            []string{"Remote"},
		RemoteAllowed:               true,
		RelocationAllowed:           false,
		MinimumSalary:               &minSalary,
		Currencies:                  []string{"RUR"},
		EmploymentTypes:             []string{"full time"},
		ExcludedCompanies:           []string{"Acme"},
		StopWords:                   []string{"intern"},
		YearsOfCommercialExperience: 5,
		Languages:                   []string{"English"},
		WorkAuthorization:           []string{"Russia"},
	}
	if err := st.UpsertCandidateProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	if profile.ID == "" {
		t.Fatal("expected id to be assigned")
	}
	if profile.CreatedAt.IsZero() || profile.UpdatedAt.IsZero() {
		t.Fatal("expected timestamps to be populated")
	}

	got, err := st.GetCandidateProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != profile.ID {
		t.Fatalf("expected same id, got %s", got.ID)
	}
	if got.MinimumSalary == nil || *got.MinimumSalary != minSalary {
		t.Fatalf("unexpected salary: %+v", got.MinimumSalary)
	}
	if !reflect.DeepEqual(got.PrimarySkills, profile.PrimarySkills) {
		t.Fatalf("unexpected primary skills: %+v", got.PrimarySkills)
	}

	updated := *got
	updated.DesiredRoles = []string{"Backend Engineer"}
	updated.MinimumSalary = nil
	updated.StopWords = nil
	if err := st.UpsertCandidateProfile(ctx, &updated); err != nil {
		t.Fatal(err)
	}

	again, err := st.GetCandidateProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != profile.ID {
		t.Fatalf("expected same profile id after update, got %s", again.ID)
	}
	if again.MinimumSalary != nil {
		t.Fatal("expected nullable salary to round-trip as nil")
	}
	if !reflect.DeepEqual(again.DesiredRoles, updated.DesiredRoles) {
		t.Fatalf("expected updated roles, got %+v", again.DesiredRoles)
	}
}

func TestStoreMissingProfileAndCompany(t *testing.T) {
	db := testutil.OpenPostgres(t)
	st := New(db)
	ctx := context.Background()

	if _, err := st.GetCandidateProfile(ctx); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
	if _, err := st.GetCompanyByID(ctx, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestStoreCompanyDedupAndFlagMerge(t *testing.T) {
	db := testutil.OpenPostgres(t)
	st := New(db)
	ctx := context.Background()

	first, err := st.GetOrCreateCompany(ctx, &core.Company{
		NormalizedName: "acme",
		DisplayName:    "Acme",
		Website:        "https://acme.example",
		CareerPage:     "https://jobs.acme.example",
		Notes:          "first",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.GetOrCreateCompany(ctx, &core.Company{
		NormalizedName: "acme",
		DisplayName:    "Acme Ltd",
		Blacklisted:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected company id to stay stable, got %s and %s", first.ID, second.ID)
	}
	if second.DisplayName != "Acme Ltd" {
		t.Fatalf("expected display name update, got %q", second.DisplayName)
	}
	if second.Website != first.Website || second.CareerPage != first.CareerPage || second.Notes != first.Notes {
		t.Fatalf("expected nullable fields to preserve prior values, got %+v", second)
	}
	if !second.Blacklisted {
		t.Fatal("expected blacklisted flag to merge")
	}
}

func TestStoreVacancyRoundTripLookupsAndConstraints(t *testing.T) {
	db := testutil.OpenPostgres(t)
	st := New(db)
	ctx := context.Background()

	empty, err := st.ListVacancies(ctx, store.ListVacanciesFilter{Page: 0, PerPage: 20})
	if err != nil {
		t.Fatal(err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("expected empty slice, got %#v", empty)
	}

	profile := mustProfile(t, st, ctx)
	source := mustSource(t, st, ctx)
	company := mustCompany(t, st, ctx, "Acme")

	baseCollected := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	vacancy := core.Vacancy{
		SourceID:                      source.ID,
		ExternalID:                    "123",
		SourceURL:                     "https://hh.ru/vacancy/123",
		CanonicalURL:                  "https://hh.ru/vacancy/123",
		Title:                         "Go Backend Engineer",
		NormalizedTitle:               "go backend engineer",
		CompanyID:                     company.ID,
		Description:                   "Build APIs",
		Requirements:                  "Go",
		Responsibilities:              "Ship APIs",
		Location:                      "Remote",
		RemoteType:                    "remote",
		EmploymentType:                "full time",
		Grade:                         "senior",
		SalaryFrom:                    intPtr(200000),
		SalaryTo:                      intPtr(300000),
		Currency:                      "RUR",
		Skills:                        []string{"go", "postgresql"},
		LanguageRequirements:          []string{"english"},
		WorkAuthorizationRequirements: []string{"russia"},
		PublishedAt:                   baseCollected.Add(-time.Hour),
		CollectedAt:                   baseCollected,
		ContentHash:                   "hash-1",
		Status:                        core.VacancyStatusRecommended,
	}
	if err := st.UpsertVacancy(ctx, &vacancy); err != nil {
		t.Fatal(err)
	}
	if vacancy.ID == "" {
		t.Fatal("expected vacancy id")
	}
	if vacancy.CreatedAt.IsZero() || vacancy.UpdatedAt.IsZero() {
		t.Fatal("expected vacancy timestamps")
	}

	got, err := st.GetVacancy(ctx, vacancy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != vacancy.ID || got.CompanyID != company.ID {
		t.Fatalf("unexpected vacancy round-trip: %+v", got)
	}
	if got.SalaryFrom == nil || got.SalaryTo == nil {
		t.Fatal("expected nullable salary fields to persist")
	}
	if !reflect.DeepEqual(got.Skills, vacancy.Skills) {
		t.Fatalf("unexpected skills: %+v", got.Skills)
	}

	bySource, err := st.FindVacancyBySourceExternalID(ctx, source.ID, "123")
	if err != nil {
		t.Fatal(err)
	}
	if bySource.ID != vacancy.ID {
		t.Fatalf("unexpected source/external lookup id: %s", bySource.ID)
	}
	byHash, err := st.FindVacancyByContentHash(ctx, vacancy.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	if byHash.ID != vacancy.ID {
		t.Fatalf("unexpected content hash lookup id: %s", byHash.ID)
	}

	match := core.VacancyMatch{
		VacancyID:          vacancy.ID,
		CandidateProfileID: profile.ID,
		TotalScore:         77,
		SkillsScore:        80,
		ExperienceScore:    70,
		LocationScore:      90,
		SalaryScore:        60,
		GradeScore:         85,
		RoleScore:          95,
		PositiveReasons:    []string{"skills"},
		NegativeReasons:    []string{"salary"},
		MissingSkills:      []string{"kubernetes"},
		HardFilterPassed:   true,
		ScoringVersion:     "v1",
	}
	if err := st.UpsertVacancyMatch(ctx, &match); err != nil {
		t.Fatal(err)
	}
	if match.ID == "" || match.CalculatedAt.IsZero() {
		t.Fatal("expected vacancy match id and timestamp")
	}
	gotMatch, err := st.GetVacancyMatch(ctx, vacancy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotMatch.ID != match.ID || gotMatch.TotalScore != 77 || !gotMatch.HardFilterPassed {
		t.Fatalf("unexpected match round-trip: %+v", gotMatch)
	}

	match.TotalScore = 88
	match.PositiveReasons = []string{"updated"}
	if err := st.UpsertVacancyMatch(ctx, &match); err != nil {
		t.Fatal(err)
	}
	if match.ID != gotMatch.ID {
		t.Fatalf("expected match id to stay stable, got %s and %s", gotMatch.ID, match.ID)
	}

	list, err := st.ListVacancies(ctx, store.ListVacanciesFilter{Page: 0, PerPage: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected one vacancy, got %d", len(list))
	}
	if list[0].Company == nil || list[0].Match == nil {
		t.Fatalf("expected list item relations to be populated: %+v", list[0])
	}

	recommended, err := st.ListRecommendedVacancies(ctx, store.ListVacanciesFilter{Page: 0, PerPage: 20}, 55)
	if err != nil {
		t.Fatal(err)
	}
	if len(recommended) != 1 {
		t.Fatalf("expected one recommended vacancy, got %d", len(recommended))
	}

	updated := vacancy
	updated.Title = "Go Backend Engineer v2"
	if err := st.UpsertVacancy(ctx, &updated); err != nil {
		t.Fatal(err)
	}
	if updated.ID != vacancy.ID {
		t.Fatalf("expected vacancy id to stay stable, got %s and %s", updated.ID, vacancy.ID)
	}
	refreshed, err := st.GetVacancy(ctx, vacancy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Title != "Go Backend Engineer v2" {
		t.Fatalf("expected updated title, got %q", refreshed.Title)
	}

	duplicate := core.Vacancy{
		SourceID:             source.ID,
		ExternalID:           "124",
		SourceURL:            "https://hh.ru/vacancy/124",
		CanonicalURL:         "https://hh.ru/vacancy/124",
		Title:                "Go Backend Engineer Copy",
		NormalizedTitle:      "go backend engineer copy",
		CompanyID:            company.ID,
		Description:          vacancy.Description,
		Requirements:         vacancy.Requirements,
		Responsibilities:     vacancy.Responsibilities,
		Location:             vacancy.Location,
		RemoteType:           vacancy.RemoteType,
		EmploymentType:       vacancy.EmploymentType,
		Grade:                vacancy.Grade,
		CollectedAt:          baseCollected.Add(time.Minute),
		PublishedAt:          baseCollected.Add(-2 * time.Hour),
		ContentHash:          vacancy.ContentHash,
		Status:               core.VacancyStatusDuplicate,
		DuplicateOfVacancyID: &vacancy.ID,
		DedupReason:          strPtr(string(core.DedupReasonContentHash)),
	}
	if err := st.UpsertVacancy(ctx, &duplicate); err != nil {
		t.Fatal(err)
	}
	if duplicate.ID == "" {
		t.Fatal("expected duplicate id")
	}
	primary, err := st.FindVacancyByContentHash(ctx, vacancy.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	if primary.ID != vacancy.ID {
		t.Fatalf("expected primary content hash vacancy, got %s", primary.ID)
	}
	items, err := st.ListVacancies(ctx, store.ListVacanciesFilter{Page: 0, PerPage: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two vacancies, got %d", len(items))
	}
	if items[0].Vacancy.ID != duplicate.ID || items[1].Vacancy.ID != vacancy.ID {
		t.Fatalf("expected deterministic ordering by collected_at desc, got %+v", items)
	}

	archived := core.VacancyStatusArchived
	if err := st.UpdateVacancyStatus(ctx, vacancy.ID, archived, nil, nil); err != nil {
		t.Fatal(err)
	}
	archivedItem, err := st.GetVacancy(ctx, vacancy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if archivedItem.Status != core.VacancyStatusArchived {
		t.Fatalf("expected archived status, got %s", archivedItem.Status)
	}

	archivedList, err := st.ListVacancies(ctx, store.ListVacanciesFilter{Page: 0, PerPage: 20, Status: &archived})
	if err != nil {
		t.Fatal(err)
	}
	if len(archivedList) != 1 {
		t.Fatalf("expected one archived vacancy, got %d", len(archivedList))
	}

	bad := core.Vacancy{
		SourceID:        "missing-source",
		ExternalID:      "broken",
		SourceURL:       "https://example.com/broken",
		Title:           "Broken",
		NormalizedTitle: "broken",
		CompanyID:       "missing-company",
		CollectedAt:     time.Now().UTC(),
		ContentHash:     "broken-hash",
		Status:          core.VacancyStatusRecommended,
	}
	if err := st.UpsertVacancy(ctx, &bad); err == nil {
		t.Fatal("expected foreign key error for missing source/company")
	}

	badStatus := vacancy
	badStatus.ID = ""
	badStatus.ExternalID = "bad-status"
	badStatus.Status = core.VacancyStatus("BROKEN")
	if err := st.UpsertVacancy(ctx, &badStatus); err == nil {
		t.Fatal("expected status check constraint error")
	}

	badMatch := core.VacancyMatch{
		VacancyID:          "missing-vacancy",
		CandidateProfileID: "missing-profile",
		TotalScore:         1,
		SkillsScore:        1,
		ExperienceScore:    1,
		LocationScore:      1,
		SalaryScore:        1,
		GradeScore:         1,
		RoleScore:          1,
		HardFilterPassed:   true,
		ScoringVersion:     "v1",
	}
	if err := st.UpsertVacancyMatch(ctx, &badMatch); err == nil {
		t.Fatal("expected foreign key error for vacancy match")
	}
}

func mustProfile(t *testing.T, st *Store, ctx context.Context) *core.CandidateProfile {
	t.Helper()
	minSalary := 240000
	profile := &core.CandidateProfile{
		DesiredRoles:                []string{"Go Backend Engineer"},
		DesiredGrades:               []string{"Senior"},
		PrimarySkills:               []string{"Go", "Postgres"},
		DesiredLocations:            []string{"Remote"},
		RemoteAllowed:               true,
		MinimumSalary:               &minSalary,
		Currencies:                  []string{"RUR"},
		EmploymentTypes:             []string{"full time"},
		YearsOfCommercialExperience: 6,
		Languages:                   []string{"English"},
		WorkAuthorization:           []string{"Russia"},
	}
	if err := st.UpsertCandidateProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	return profile
}

func mustSource(t *testing.T, st *Store, ctx context.Context) core.JobSource {
	t.Helper()
	source := core.JobSource{
		Type:    core.SourceTypeHeadhunterAPI,
		Name:    "HeadHunter",
		Enabled: true,
		Configuration: map[string]any{
			"host": "hh.ru",
		},
	}
	if err := st.UpsertJobSource(ctx, &source); err != nil {
		t.Fatal(err)
	}
	return source
}

func mustCompany(t *testing.T, st *Store, ctx context.Context, name string) *core.Company {
	t.Helper()
	company, err := st.GetOrCreateCompany(ctx, &core.Company{
		NormalizedName: core.NormalizeText(name),
		DisplayName:    name,
	})
	if err != nil {
		t.Fatal(err)
	}
	return company
}

func intPtr(v int) *int { return &v }

func strPtr(v string) *string { return &v }
