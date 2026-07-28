//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"jobscout.ai/internal/core"
	"jobscout.ai/internal/store"
	"jobscout.ai/internal/testutil"
)

func TestStoreResumeRoundTripAndEmptyLists(t *testing.T) {
	db := testutil.OpenPostgres(t)
	st := New(db)
	ctx := context.Background()

	emptyResumes, err := st.ListResumes(ctx, "missing")
	if err != nil {
		t.Fatal(err)
	}
	if emptyResumes == nil || len(emptyResumes) != 0 {
		t.Fatalf("expected empty resume list, got %#v", emptyResumes)
	}
	emptyApplications, err := st.ListApplications(ctx, "missing")
	if err != nil {
		t.Fatal(err)
	}
	if emptyApplications == nil || len(emptyApplications) != 0 {
		t.Fatalf("expected empty application list, got %#v", emptyApplications)
	}

	profile := mustProfile(t, st, ctx)
	resume := &core.Resume{
		CandidateProfileID: profile.ID,
		Name:               "Go Resume",
		TargetRole:         core.ResumeTargetRoleGoBackend,
		Language:           core.ResumeLanguageRU,
		TextContent:        "Go backend engineer",
		Skills:             []string{"Go", "PostgreSQL", "Go"},
		IsActive:           true,
	}
	if err := st.UpsertResume(ctx, resume); err != nil {
		t.Fatal(err)
	}
	if resume.ID == "" || resume.CreatedAt.IsZero() || resume.UpdatedAt.IsZero() {
		t.Fatalf("expected resume timestamps, got %+v", resume)
	}
	got, err := st.GetResume(ctx, resume.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != resume.ID || got.CandidateProfileID != profile.ID {
		t.Fatalf("unexpected resume round-trip: %+v", got)
	}
	if len(got.Skills) != 2 {
		t.Fatalf("expected normalized skills, got %+v", got.Skills)
	}

	updated := *got
	updated.IsActive = false
	updated.Name = "Go Resume v2"
	if err := st.UpsertResume(ctx, &updated); err != nil {
		t.Fatal(err)
	}
	refreshed, err := st.GetResume(ctx, resume.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.IsActive {
		t.Fatal("expected resume deactivation to persist")
	}

	list, err := st.ListResumes(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != resume.ID {
		t.Fatalf("unexpected resume list: %+v", list)
	}
}

func TestStoreApplicationRoundTripConstraintsAndAudit(t *testing.T) {
	db := testutil.OpenPostgres(t)
	st := New(db)
	ctx := context.Background()

	profile := mustProfile(t, st, ctx)
	source := mustSource(t, st, ctx)
	company := mustCompany(t, st, ctx, "Acme")
	resume := &core.Resume{
		CandidateProfileID: profile.ID,
		Name:               "Go Resume",
		TargetRole:         core.ResumeTargetRoleGoBackend,
		Language:           core.ResumeLanguageRU,
		TextContent:        "Project experience",
		Skills:             []string{"Go", "PostgreSQL"},
		IsActive:           true,
	}
	if err := st.UpsertResume(ctx, resume); err != nil {
		t.Fatal(err)
	}
	vacancy := core.Vacancy{
		SourceID:         source.ID,
		ExternalID:       "123",
		SourceURL:        "https://hh.ru/vacancy/123",
		CanonicalURL:     "https://hh.ru/vacancy/123",
		Title:            "Go Backend Engineer",
		NormalizedTitle:  "go backend engineer",
		CompanyID:        company.ID,
		Description:      "Build APIs",
		Requirements:     "Go",
		Responsibilities: "Ship APIs",
		Location:         "Remote",
		RemoteType:       "remote",
		EmploymentType:   "full time",
		Grade:            "senior",
		PublishedAt:      time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC),
		CollectedAt:      time.Date(2026, 7, 28, 11, 0, 0, 0, time.UTC),
		ContentHash:      "hash-1",
		Status:           core.VacancyStatusRecommended,
	}
	if err := st.UpsertVacancy(ctx, &vacancy); err != nil {
		t.Fatal(err)
	}

	application := &core.Application{
		VacancyID:          vacancy.ID,
		CandidateProfileID: profile.ID,
		ResumeID:           resume.ID,
		Status:             core.ApplicationStatusWaitingApproval,
		ApplicationMethod:  core.ApplicationMethodManualLink,
		CoverLetter:        "Hello",
		PreparedAt:         time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC),
	}
	if err := st.UpsertApplication(ctx, application); err != nil {
		t.Fatal(err)
	}
	if application.ID == "" || application.CreatedAt.IsZero() || application.UpdatedAt.IsZero() {
		t.Fatalf("expected application timestamps, got %+v", application)
	}
	got, err := st.GetApplication(ctx, application.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != application.ID || got.Status != core.ApplicationStatusWaitingApproval {
		t.Fatalf("unexpected application round-trip: %+v", got)
	}
	if !got.PreparedAt.Equal(application.PreparedAt) {
		t.Fatalf("expected prepared_at round-trip, got %+v", got.PreparedAt)
	}

	list, err := st.ListApplications(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != application.ID {
		t.Fatalf("unexpected application list: %+v", list)
	}

	audit := &core.AuditEvent{
		Actor:      "tester",
		Action:     core.AuditActionApplicationPrepared,
		EntityType: core.AuditEntityTypeApplication,
		EntityID:   application.ID,
		Metadata:   map[string]any{"vacancyId": vacancy.ID},
	}
	if err := st.CreateAuditEvent(ctx, audit); err != nil {
		t.Fatal(err)
	}
	events, err := st.ListAuditEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EntityID != application.ID {
		t.Fatalf("unexpected audit events: %+v", events)
	}

	updated := *got
	updated.Status = core.ApplicationStatusManualActionRequired
	if err := st.UpsertApplication(ctx, &updated); err != nil {
		t.Fatal(err)
	}
	updated.Status = core.ApplicationStatusSubmitted
	submittedAt := time.Date(2026, 7, 28, 13, 0, 0, 0, time.UTC)
	updated.SubmittedAt = &submittedAt
	if err := st.UpsertApplication(ctx, &updated); err != nil {
		t.Fatal(err)
	}
	submitted, err := st.GetApplication(ctx, application.ID)
	if err != nil {
		t.Fatal(err)
	}
	if submitted.Status != core.ApplicationStatusSubmitted || submitted.SubmittedAt == nil {
		t.Fatalf("expected submitted application, got %+v", submitted)
	}

	duplicate := &core.Application{
		VacancyID:          vacancy.ID,
		CandidateProfileID: profile.ID,
		ResumeID:           resume.ID,
		Status:             core.ApplicationStatusWaitingApproval,
		ApplicationMethod:  core.ApplicationMethodManualLink,
		CoverLetter:        "Duplicate",
		PreparedAt:         time.Date(2026, 7, 28, 14, 0, 0, 0, time.UTC),
	}
	if err := st.UpsertApplication(ctx, duplicate); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("expected active duplicate conflict, got %v", err)
	}

	duplicate.Status = core.ApplicationStatusCancelled
	if err := st.UpsertApplication(ctx, duplicate); err != nil {
		t.Fatal(err)
	}

	badResume := &core.Resume{
		CandidateProfileID: "missing",
		Name:               "Broken",
		TargetRole:         core.ResumeTargetRoleGoBackend,
		Language:           core.ResumeLanguageRU,
		TextContent:        "broken",
	}
	if err := st.UpsertResume(ctx, badResume); err == nil {
		t.Fatal("expected foreign key error for missing profile")
	}

	badApplication := &core.Application{
		VacancyID:          "missing",
		CandidateProfileID: profile.ID,
		ResumeID:           resume.ID,
		Status:             core.ApplicationStatusWaitingApproval,
		ApplicationMethod:  core.ApplicationMethodManualLink,
		CoverLetter:        "Broken",
		PreparedAt:         time.Date(2026, 7, 28, 15, 0, 0, 0, time.UTC),
	}
	if err := st.UpsertApplication(ctx, badApplication); err == nil {
		t.Fatal("expected foreign key error for missing vacancy")
	}

	badStatus := &core.Application{
		VacancyID:          vacancy.ID,
		CandidateProfileID: profile.ID,
		ResumeID:           resume.ID,
		Status:             core.ApplicationStatus("BROKEN"),
		ApplicationMethod:  core.ApplicationMethodManualLink,
		CoverLetter:        "Broken",
		PreparedAt:         time.Date(2026, 7, 28, 16, 0, 0, 0, time.UTC),
	}
	if err := st.UpsertApplication(ctx, badStatus); err == nil {
		t.Fatal("expected status constraint error")
	}
}
