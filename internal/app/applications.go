package app

import (
	"context"
	"errors"
	"strings"

	"jobscout.ai/internal/core"
	"jobscout.ai/internal/store"
)

var ErrVacancyArchived = errors.New("vacancy is archived")
var ErrInvalidApplicationTransition = errors.New("invalid application status transition")

type PrepareApplicationRequest struct {
	ResumeID *string `json:"resumeId,omitempty"`
}

type ApplicationOutcomeRequest struct {
	Status          string `json:"status"`
	RejectionReason string `json:"rejectionReason,omitempty"`
	Notes           string `json:"notes,omitempty"`
}

type ApplicationView struct {
	Application core.Application   `json:"application"`
	Resume      *core.Resume       `json:"resume,omitempty"`
	Vacancy     *core.Vacancy      `json:"vacancy,omitempty"`
	Company     *core.Company      `json:"company,omitempty"`
	Match       *core.VacancyMatch `json:"match,omitempty"`
	Warnings    []string           `json:"warnings,omitempty"`
	VacancyURL  string             `json:"vacancyUrl,omitempty"`
	Created     bool               `json:"created,omitempty"`
}

func (a *App) ListApplications(ctx context.Context) ([]ApplicationView, error) {
	profile, err := a.GetProfile(ctx)
	if err != nil {
		return nil, err
	}
	applications, err := a.storage.ListApplications(ctx, profile.ID)
	if err != nil {
		return nil, err
	}
	if len(applications) == 0 {
		return []ApplicationView{}, nil
	}
	out := make([]ApplicationView, 0, len(applications))
	for _, application := range applications {
		view, err := a.buildApplicationView(ctx, a.storage, profile, &application)
		if err != nil {
			return nil, err
		}
		out = append(out, *view)
	}
	return out, nil
}

func (a *App) GetApplication(ctx context.Context, id string) (*ApplicationView, error) {
	profile, err := a.GetProfile(ctx)
	if err != nil {
		return nil, err
	}
	application, err := a.storage.GetApplication(ctx, id)
	if err != nil {
		return nil, err
	}
	return a.buildApplicationView(ctx, a.storage, profile, application)
}

func (a *App) PrepareApplication(ctx context.Context, vacancyID string, resumeID *string, actor string) (*ApplicationView, error) {
	profile, err := a.GetProfile(ctx)
	if err != nil {
		return nil, err
	}
	var view *ApplicationView
	err = a.withStoreTransaction(ctx, func(st store.ImportStore) error {
		var err error
		view, err = a.prepareApplicationWithStore(ctx, st, profile, vacancyID, resumeID, actor)
		return err
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

func (a *App) ApproveApplication(ctx context.Context, id string, actor string) (*ApplicationView, error) {
	profile, err := a.GetProfile(ctx)
	if err != nil {
		return nil, err
	}
	var view *ApplicationView
	err = a.withStoreTransaction(ctx, func(st store.ImportStore) error {
		application, err := st.GetApplication(ctx, id)
		if err != nil {
			return err
		}
		switch application.Status {
		case core.ApplicationStatusWaitingApproval:
			now := a.now()
			approvedAt := now
			application.Status = core.ApplicationStatusApproved
			application.ApprovedAt = &approvedAt
			application.UpdatedAt = now
			if err := st.UpsertApplication(ctx, application); err != nil {
				return err
			}
			application.Status = core.ApplicationStatusManualActionRequired
			application.UpdatedAt = now
			if err := st.UpsertApplication(ctx, application); err != nil {
				return err
			}
		case core.ApplicationStatusApproved:
			now := a.now()
			if application.ApprovedAt == nil {
				approvedAt := now
				application.ApprovedAt = &approvedAt
			}
			application.Status = core.ApplicationStatusManualActionRequired
			application.UpdatedAt = now
			if err := st.UpsertApplication(ctx, application); err != nil {
				return err
			}
		case core.ApplicationStatusManualActionRequired:
			view, err = a.buildApplicationView(ctx, st, profile, application)
			return err
		default:
			return ErrInvalidApplicationTransition
		}
		now := a.now()
		if err := st.CreateAuditEvent(ctx, &core.AuditEvent{
			Actor:      actor,
			Action:     core.AuditActionApplicationApproved,
			EntityType: core.AuditEntityTypeApplication,
			EntityID:   application.ID,
			Metadata: map[string]any{
				"vacancyId":         application.VacancyID,
				"resumeId":          application.ResumeID,
				"status":            string(application.Status),
				"approvedAt":        now,
				"applicationMethod": application.ApplicationMethod,
			},
			CreatedAt: now,
		}); err != nil {
			return err
		}
		view, err = a.buildApplicationView(ctx, st, profile, application)
		return err
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

func (a *App) CancelApplication(ctx context.Context, id string, actor string) (*ApplicationView, error) {
	profile, err := a.GetProfile(ctx)
	if err != nil {
		return nil, err
	}
	var view *ApplicationView
	err = a.withStoreTransaction(ctx, func(st store.ImportStore) error {
		application, err := st.GetApplication(ctx, id)
		if err != nil {
			return err
		}
		switch application.Status {
		case core.ApplicationStatusCancelled:
			view, err = a.buildApplicationView(ctx, st, profile, application)
			return err
		case core.ApplicationStatusRejected, core.ApplicationStatusSubmitted, core.ApplicationStatusHRContact, core.ApplicationStatusInterview, core.ApplicationStatusOffer:
			return ErrInvalidApplicationTransition
		default:
			now := a.now()
			application.Status = core.ApplicationStatusCancelled
			application.UpdatedAt = now
			if err := st.UpsertApplication(ctx, application); err != nil {
				return err
			}
			if err := st.CreateAuditEvent(ctx, &core.AuditEvent{
				Actor:      actor,
				Action:     core.AuditActionApplicationCancelled,
				EntityType: core.AuditEntityTypeApplication,
				EntityID:   application.ID,
				Metadata: map[string]any{
					"vacancyId": application.VacancyID,
					"resumeId":  application.ResumeID,
				},
				CreatedAt: now,
			}); err != nil {
				return err
			}
			view, err = a.buildApplicationView(ctx, st, profile, application)
			return err
		}
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

func (a *App) MarkApplicationSubmitted(ctx context.Context, id string, actor string) (*ApplicationView, error) {
	profile, err := a.GetProfile(ctx)
	if err != nil {
		return nil, err
	}
	var view *ApplicationView
	err = a.withStoreTransaction(ctx, func(st store.ImportStore) error {
		application, err := st.GetApplication(ctx, id)
		if err != nil {
			return err
		}
		switch application.Status {
		case core.ApplicationStatusSubmitted:
			view, err = a.buildApplicationView(ctx, st, profile, application)
			return err
		case core.ApplicationStatusManualActionRequired:
			now := a.now()
			submittedAt := now
			application.Status = core.ApplicationStatusSubmitted
			application.SubmittedAt = &submittedAt
			application.UpdatedAt = now
			if err := st.UpsertApplication(ctx, application); err != nil {
				return err
			}
			vacancy, err := st.GetVacancy(ctx, application.VacancyID)
			if err != nil {
				return err
			}
			if !core.CanTransitionVacancyStatus(vacancy.Status, core.VacancyStatusSubmitted) {
				return ErrInvalidApplicationTransition
			}
			if err := st.UpdateVacancyStatus(ctx, vacancy.ID, core.VacancyStatusSubmitted, vacancy.DuplicateOfVacancyID, vacancy.DedupReason); err != nil {
				return err
			}
			if err := st.CreateAuditEvent(ctx, &core.AuditEvent{
				Actor:      actor,
				Action:     core.AuditActionApplicationMarkedSubmitted,
				EntityType: core.AuditEntityTypeApplication,
				EntityID:   application.ID,
				Metadata: map[string]any{
					"vacancyId": application.VacancyID,
					"resumeId":  application.ResumeID,
				},
				CreatedAt: now,
			}); err != nil {
				return err
			}
			view, err = a.buildApplicationView(ctx, st, profile, application)
			return err
		default:
			return ErrInvalidApplicationTransition
		}
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

func (a *App) UpdateApplicationOutcome(ctx context.Context, id string, req ApplicationOutcomeRequest, actor string) (*ApplicationView, error) {
	profile, err := a.GetProfile(ctx)
	if err != nil {
		return nil, err
	}
	status, err := core.ParseApplicationStatus(strings.TrimSpace(req.Status))
	if err != nil {
		return nil, err
	}
	if !isOutcomeStatus(status) {
		return nil, ErrInvalidApplicationTransition
	}
	var view *ApplicationView
	err = a.withStoreTransaction(ctx, func(st store.ImportStore) error {
		application, err := st.GetApplication(ctx, id)
		if err != nil {
			return err
		}
		if application.Status == status {
			view, err = a.buildApplicationView(ctx, st, profile, application)
			return err
		}
		if !core.CanTransitionApplicationStatus(application.Status, status) {
			return ErrInvalidApplicationTransition
		}
		now := a.now()
		responseReceivedAt := now
		application.Status = status
		application.ResponseReceivedAt = &responseReceivedAt
		application.UpdatedAt = now
		if status == core.ApplicationStatusRejected && strings.TrimSpace(req.RejectionReason) != "" {
			application.RejectionReason = strings.TrimSpace(req.RejectionReason)
		}
		if strings.TrimSpace(req.Notes) != "" {
			application.Notes = strings.TrimSpace(req.Notes)
		}
		if err := st.UpsertApplication(ctx, application); err != nil {
			return err
		}
		vacancy, err := st.GetVacancy(ctx, application.VacancyID)
		if err != nil {
			return err
		}
		if !core.CanTransitionVacancyStatus(vacancy.Status, vacancyStatusForOutcome(status)) {
			return ErrInvalidApplicationTransition
		}
		if err := st.UpdateVacancyStatus(ctx, vacancy.ID, vacancyStatusForOutcome(status), vacancy.DuplicateOfVacancyID, vacancy.DedupReason); err != nil {
			return err
		}
		if err := st.CreateAuditEvent(ctx, &core.AuditEvent{
			Actor:      actor,
			Action:     auditActionForOutcome(status),
			EntityType: core.AuditEntityTypeApplication,
			EntityID:   application.ID,
			Metadata: map[string]any{
				"vacancyId": application.VacancyID,
				"resumeId":  application.ResumeID,
				"status":    string(status),
			},
			CreatedAt: now,
		}); err != nil {
			return err
		}
		view, err = a.buildApplicationView(ctx, st, profile, application)
		return err
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

func (a *App) prepareApplicationWithStore(ctx context.Context, st store.ImportStore, profile *core.CandidateProfile, vacancyID string, resumeID *string, actor string) (*ApplicationView, error) {
	vacancy, err := st.GetVacancy(ctx, vacancyID)
	if err != nil {
		return nil, err
	}
	if vacancy.Status == core.VacancyStatusArchived {
		return nil, ErrVacancyArchived
	}
	match, err := st.GetVacancyMatch(ctx, vacancy.ID)
	if err != nil {
		return nil, err
	}
	if existing, err := st.FindActiveApplicationByVacancyProfile(ctx, vacancy.ID, profile.ID); err == nil {
		return a.buildApplicationView(ctx, st, profile, existing)
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	resumes, err := st.ListResumes(ctx, profile.ID)
	if err != nil {
		return nil, err
	}
	selector := core.NewDeterministicResumeSelector()
	selected, err := selector.Select(ctx, core.ResumeSelectionInput{
		ManualResumeID: strings.TrimSpace(derefString(resumeID)),
		Vacancy:        *vacancy,
		Match:          match,
		Resumes:        resumes,
	})
	if err != nil {
		return nil, err
	}
	company, err := st.GetCompanyByID(ctx, vacancy.CompanyID)
	if err != nil {
		return nil, err
	}
	projectExperience := core.ExtractProjectExperience(selected.TextContent)
	generator := core.NewDeterministicCoverLetterGenerator()
	coverLetter, err := generator.Generate(ctx, core.CoverLetterInput{
		CandidateName:             candidateNameFromActor(actor),
		TargetRole:                selected.TargetRole,
		CommercialExperienceYears: profile.YearsOfCommercialExperience,
		ProjectExperience:         projectExperience,
		ResumeSkills:              selected.Skills,
		Vacancy:                   *vacancy,
		Match:                     match,
		PositiveReasons:           match.PositiveReasons,
		MissingSkills:             match.MissingSkills,
		CompanyName:               company.DisplayName,
	})
	if err != nil {
		return nil, err
	}
	now := a.now()
	application := &core.Application{
		ID:                 core.NewID(),
		VacancyID:          vacancy.ID,
		CandidateProfileID: profile.ID,
		ResumeID:           selected.ID,
		Status:             core.ApplicationStatusWaitingApproval,
		ApplicationMethod:  core.ApplicationMethodManualLink,
		CoverLetter:        coverLetter,
		PreparedAt:         now,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := st.UpsertApplication(ctx, application); err != nil {
		return nil, err
	}
	if err := st.CreateAuditEvent(ctx, &core.AuditEvent{
		Actor:      actor,
		Action:     core.AuditActionApplicationPrepared,
		EntityType: core.AuditEntityTypeApplication,
		EntityID:   application.ID,
		Metadata: map[string]any{
			"vacancyId":            vacancy.ID,
			"resumeId":             selected.ID,
			"targetRole":           string(selected.TargetRole),
			"applicationMethod":    string(application.ApplicationMethod),
			"manualResumeId":       strings.TrimSpace(derefString(resumeID)),
			"positiveReasonsCount": len(match.PositiveReasons),
			"missingSkillsCount":   len(match.MissingSkills),
		},
		CreatedAt: now,
	}); err != nil {
		return nil, err
	}
	view, err := a.buildApplicationView(ctx, st, profile, application)
	if err != nil {
		return nil, err
	}
	view.Company = company
	view.Resume = &selected
	view.Created = true
	if strings.TrimSpace(view.VacancyURL) == "" {
		if strings.TrimSpace(vacancy.CanonicalURL) != "" {
			view.VacancyURL = vacancy.CanonicalURL
		} else {
			view.VacancyURL = vacancy.SourceURL
		}
	}
	view.Warnings = core.BuildApplicationWarnings(*profile, selected, *vacancy, match, projectExperience)
	return view, nil
}

func (a *App) buildApplicationView(ctx context.Context, st store.ImportStore, profile *core.CandidateProfile, application *core.Application) (*ApplicationView, error) {
	resume, err := st.GetResume(ctx, application.ResumeID)
	if err != nil {
		return nil, err
	}
	vacancy, err := st.GetVacancy(ctx, application.VacancyID)
	if err != nil {
		return nil, err
	}
	company, err := st.GetCompanyByID(ctx, vacancy.CompanyID)
	if err != nil {
		return nil, err
	}
	match, err := st.GetVacancyMatch(ctx, vacancy.ID)
	if err != nil {
		return nil, err
	}
	projectExperience := core.ExtractProjectExperience(resume.TextContent)
	warnings := core.BuildApplicationWarnings(*profile, *resume, *vacancy, match, projectExperience)
	vacancyURL := vacancy.CanonicalURL
	if strings.TrimSpace(vacancyURL) == "" {
		vacancyURL = vacancy.SourceURL
	}
	return &ApplicationView{
		Application: *application,
		Resume:      resume,
		Vacancy:     vacancy,
		Company:     company,
		Match:       match,
		Warnings:    warnings,
		VacancyURL:  vacancyURL,
	}, nil
}

func (a *App) withStoreTransaction(ctx context.Context, fn func(store.ImportStore) error) error {
	if txRunner, ok := a.storage.(store.ImportTxRunner); ok {
		return txRunner.WithinImportTransaction(ctx, func(txStore store.ImportStore) error {
			return fn(txStore)
		})
	}
	return fn(a.storage)
}

func candidateNameFromActor(actor string) string {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return ""
	}
	lower := strings.ToLower(actor)
	if lower == "http" || strings.HasPrefix(lower, "telegram:") {
		return ""
	}
	return actor
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func isOutcomeStatus(status core.ApplicationStatus) bool {
	switch status {
	case core.ApplicationStatusHRContact, core.ApplicationStatusInterview, core.ApplicationStatusOffer, core.ApplicationStatusRejected:
		return true
	default:
		return false
	}
}

func vacancyStatusForOutcome(status core.ApplicationStatus) core.VacancyStatus {
	switch status {
	case core.ApplicationStatusHRContact:
		return core.VacancyStatusHRContact
	case core.ApplicationStatusInterview:
		return core.VacancyStatusInterview
	case core.ApplicationStatusOffer:
		return core.VacancyStatusOffer
	case core.ApplicationStatusRejected:
		return core.VacancyStatusRejected
	default:
		return core.VacancyStatusRecommended
	}
}

func auditActionForOutcome(status core.ApplicationStatus) core.AuditAction {
	switch status {
	case core.ApplicationStatusHRContact:
		return core.AuditActionApplicationHRContact
	case core.ApplicationStatusInterview:
		return core.AuditActionApplicationInterview
	case core.ApplicationStatusOffer:
		return core.AuditActionApplicationOffer
	case core.ApplicationStatusRejected:
		return core.AuditActionApplicationRejected
	default:
		return core.AuditActionApplicationPrepared
	}
}
