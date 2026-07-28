package store

import (
	"context"
	"errors"

	"jobscout.ai/internal/core"
)

var ErrNotFound = errors.New("not found")

type ListVacanciesFilter struct {
	Page            int
	PerPage         int
	Status          *core.VacancyStatus
	RecommendedOnly bool
}

type VacancyWithMatch struct {
	Vacancy core.Vacancy       `json:"vacancy"`
	Company *core.Company      `json:"company,omitempty"`
	Match   *core.VacancyMatch `json:"match,omitempty"`
}

type Store interface {
	UpsertCandidateProfile(ctx context.Context, profile *core.CandidateProfile) error
	GetCandidateProfile(ctx context.Context) (*core.CandidateProfile, error)

	UpsertJobSource(ctx context.Context, source *core.JobSource) error
	ListJobSources(ctx context.Context, enabledOnly bool) ([]core.JobSource, error)
	EnsureCoreSource(ctx context.Context, source *core.JobSource) error

	GetOrCreateCompany(ctx context.Context, company *core.Company) (*core.Company, error)
	GetCompanyByID(ctx context.Context, id string) (*core.Company, error)

	FindVacancyBySourceExternalID(ctx context.Context, sourceID, externalID string) (*core.Vacancy, error)
	FindVacancyByContentHash(ctx context.Context, contentHash string) (*core.Vacancy, error)
	UpsertVacancy(ctx context.Context, vacancy *core.Vacancy) error
	UpdateVacancyStatus(ctx context.Context, id string, status core.VacancyStatus, duplicateOf *string, dedupReason *string) error
	GetVacancy(ctx context.Context, id string) (*core.Vacancy, error)
	ListVacancies(ctx context.Context, filter ListVacanciesFilter) ([]VacancyWithMatch, error)
	ListRecommendedVacancies(ctx context.Context, filter ListVacanciesFilter, minScore int) ([]VacancyWithMatch, error)

	UpsertVacancyMatch(ctx context.Context, match *core.VacancyMatch) error
	GetVacancyMatch(ctx context.Context, vacancyID string) (*core.VacancyMatch, error)
}
