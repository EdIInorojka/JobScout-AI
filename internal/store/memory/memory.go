package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"jobscout.ai/internal/core"
	"jobscout.ai/internal/store"
)

type Store struct {
	mu           sync.RWMutex
	profile      *core.CandidateProfile
	sources      map[string]core.JobSource
	companies    map[string]core.Company
	vacancies    map[string]core.Vacancy
	matches      map[string]core.VacancyMatch
	resumes      map[string]core.Resume
	applications map[string]core.Application
	auditEvents  []core.AuditEvent
}

func New() *Store {
	return &Store{
		sources:      make(map[string]core.JobSource),
		companies:    make(map[string]core.Company),
		vacancies:    make(map[string]core.Vacancy),
		matches:      make(map[string]core.VacancyMatch),
		resumes:      make(map[string]core.Resume),
		applications: make(map[string]core.Application),
	}
}

func (s *Store) UpsertCandidateProfile(ctx context.Context, profile *core.CandidateProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *profile
	s.profile = &cp
	return nil
}

func (s *Store) GetCandidateProfile(ctx context.Context) (*core.CandidateProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.profile == nil {
		return nil, store.ErrNotFound
	}
	cp := *s.profile
	return &cp, nil
}

func (s *Store) UpsertJobSource(ctx context.Context, source *core.JobSource) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *source
	s.sources[source.ID] = cp
	return nil
}

func (s *Store) EnsureCoreSource(ctx context.Context, source *core.JobSource) error {
	return s.UpsertJobSource(ctx, source)
}

func (s *Store) WithinImportTransaction(ctx context.Context, fn func(store.ImportStore) error) error {
	return fn(s)
}

func (s *Store) ListJobSources(ctx context.Context, enabledOnly bool) ([]core.JobSource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]core.JobSource, 0, len(s.sources))
	for _, source := range s.sources {
		if enabledOnly && !source.Enabled {
			continue
		}
		out = append(out, source)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) GetOrCreateCompany(ctx context.Context, company *core.Company) (*core.Company, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.companies {
		if existing.NormalizedName == company.NormalizedName {
			existing.DisplayName = company.DisplayName
			existing.Website = company.Website
			existing.CareerPage = company.CareerPage
			existing.Blacklisted = existing.Blacklisted || company.Blacklisted
			existing.Notes = company.Notes
			s.companies[existing.ID] = existing
			cp := existing
			return &cp, nil
		}
	}
	cp := *company
	s.companies[cp.ID] = cp
	return &cp, nil
}

func (s *Store) GetCompanyByID(ctx context.Context, id string) (*core.Company, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	company, ok := s.companies[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := company
	return &cp, nil
}

func (s *Store) FindVacancyBySourceExternalID(ctx context.Context, sourceID, externalID string) (*core.Vacancy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, vacancy := range s.vacancies {
		if vacancy.SourceID == sourceID && vacancy.ExternalID == externalID {
			v := vacancy
			return &v, nil
		}
	}
	return nil, store.ErrNotFound
}

func (s *Store) FindVacancyByContentHash(ctx context.Context, contentHash string) (*core.Vacancy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, vacancy := range s.vacancies {
		if vacancy.ContentHash == contentHash && vacancy.DuplicateOfVacancyID == nil {
			v := vacancy
			return &v, nil
		}
	}
	return nil, store.ErrNotFound
}

func (s *Store) UpsertVacancy(ctx context.Context, vacancy *core.Vacancy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if vacancy.ID == "" {
		vacancy.ID = core.NewID()
	}
	cp := *vacancy
	s.vacancies[cp.ID] = cp
	return nil
}

func (s *Store) UpdateVacancyStatus(ctx context.Context, id string, status core.VacancyStatus, duplicateOf *string, dedupReason *string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	vacancy, ok := s.vacancies[id]
	if !ok {
		return store.ErrNotFound
	}
	vacancy.Status = status
	vacancy.DuplicateOfVacancyID = duplicateOf
	vacancy.DedupReason = dedupReason
	s.vacancies[id] = vacancy
	return nil
}

func (s *Store) GetVacancy(ctx context.Context, id string) (*core.Vacancy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	vacancy, ok := s.vacancies[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	v := vacancy
	return &v, nil
}

func (s *Store) ListVacancies(ctx context.Context, filter store.ListVacanciesFilter) ([]store.VacancyWithMatch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]store.VacancyWithMatch, 0, len(s.vacancies))
	for _, vacancy := range s.vacancies {
		if filter.Status != nil && vacancy.Status != *filter.Status {
			continue
		}
		match, ok := s.matches[vacancy.ID]
		if filter.RecommendedOnly {
			if vacancy.Status != core.VacancyStatusRecommended || !ok || !match.HardFilterPassed {
				continue
			}
		}
		v := vacancy
		var m *core.VacancyMatch
		if ok {
			cp := match
			m = &cp
		}
		items = append(items, store.VacancyWithMatch{Vacancy: v, Match: m})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Vacancy.CollectedAt.After(items[j].Vacancy.CollectedAt) })
	start := filter.Page * filter.PerPage
	if start > len(items) {
		return []store.VacancyWithMatch{}, nil
	}
	end := start + filter.PerPage
	if end > len(items) {
		end = len(items)
	}
	result := make([]store.VacancyWithMatch, 0, end-start)
	result = append(result, items[start:end]...)
	return result, nil
}

func (s *Store) ListRecommendedVacancies(ctx context.Context, filter store.ListVacanciesFilter, minScore int) ([]store.VacancyWithMatch, error) {
	filter.RecommendedOnly = true
	items, err := s.ListVacancies(ctx, filter)
	if err != nil {
		return nil, err
	}
	out := make([]store.VacancyWithMatch, 0, len(items))
	for _, item := range items {
		if item.Match != nil && item.Match.TotalScore >= minScore {
			out = append(out, item)
		}
	}
	return out, nil
}

func (s *Store) UpsertVacancyMatch(ctx context.Context, match *core.VacancyMatch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.matches[match.VacancyID]; ok && match.ID == "" {
		match.ID = existing.ID
	}
	if match.ID == "" {
		match.ID = core.NewID()
	}
	if match.CalculatedAt.IsZero() {
		match.CalculatedAt = time.Now().UTC()
	}
	cp := *match
	s.matches[match.VacancyID] = cp
	return nil
}

func (s *Store) GetVacancyMatch(ctx context.Context, vacancyID string) (*core.VacancyMatch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	match, ok := s.matches[vacancyID]
	if !ok {
		return nil, store.ErrNotFound
	}
	m := match
	return &m, nil
}

func (s *Store) UpsertResume(ctx context.Context, resume *core.Resume) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if resume.ID == "" {
		resume.ID = core.NewID()
	}
	now := time.Now().UTC()
	if resume.CreatedAt.IsZero() {
		resume.CreatedAt = now
	}
	resume.UpdatedAt = now
	resume.Name = strings.TrimSpace(resume.Name)
	resume.TextContent = strings.TrimSpace(resume.TextContent)
	resume.Skills = core.NormalizeStringList(resume.Skills)
	cp := *resume
	s.resumes[cp.ID] = cp
	return nil
}

func (s *Store) GetResume(ctx context.Context, id string) (*core.Resume, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	resume, ok := s.resumes[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := resume
	return &cp, nil
}

func (s *Store) ListResumes(ctx context.Context, candidateProfileID string) ([]core.Resume, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]core.Resume, 0, len(s.resumes))
	for _, resume := range s.resumes {
		if candidateProfileID != "" && resume.CandidateProfileID != candidateProfileID {
			continue
		}
		cp := resume
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *Store) FindActiveApplicationByVacancyProfile(ctx context.Context, vacancyID, candidateProfileID string) (*core.Application, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, application := range s.applications {
		if application.VacancyID == vacancyID && application.CandidateProfileID == candidateProfileID && application.IsActive() {
			cp := application
			return &cp, nil
		}
	}
	return nil, store.ErrNotFound
}

func (s *Store) GetApplication(ctx context.Context, id string) (*core.Application, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	application, ok := s.applications[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := application
	return &cp, nil
}

func (s *Store) ListApplications(ctx context.Context, candidateProfileID string) ([]core.Application, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]core.Application, 0, len(s.applications))
	for _, application := range s.applications {
		if candidateProfileID != "" && application.CandidateProfileID != candidateProfileID {
			continue
		}
		cp := application
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

func (s *Store) UpsertApplication(ctx context.Context, application *core.Application) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if application.ID == "" {
		application.ID = core.NewID()
	}
	now := time.Now().UTC()
	if application.CreatedAt.IsZero() {
		application.CreatedAt = now
	}
	application.UpdatedAt = now
	if application.PreparedAt.IsZero() {
		application.PreparedAt = now
	}
	application.CoverLetter = strings.TrimSpace(application.CoverLetter)
	application.Notes = strings.TrimSpace(application.Notes)
	application.RejectionReason = strings.TrimSpace(application.RejectionReason)
	if application.IsActive() {
		for _, existing := range s.applications {
			if existing.ID != application.ID && existing.VacancyID == application.VacancyID && existing.CandidateProfileID == application.CandidateProfileID && existing.IsActive() {
				return store.ErrConflict
			}
		}
	}
	cp := *application
	s.applications[cp.ID] = cp
	return nil
}

func (s *Store) CreateAuditEvent(ctx context.Context, event *core.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.ID == "" {
		event.ID = core.NewID()
	}
	now := time.Now().UTC()
	if event.CreatedAt.IsZero() {
		event.CreatedAt = now
	}
	if event.Metadata == nil {
		event.Metadata = map[string]any{}
	}
	cp := *event
	s.auditEvents = append(s.auditEvents, cp)
	return nil
}

func (s *Store) ListAuditEvents(ctx context.Context) ([]core.AuditEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]core.AuditEvent, len(s.auditEvents))
	copy(out, s.auditEvents)
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}
