package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"jobscout.ai/internal/core"
	"jobscout.ai/internal/store"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) UpsertCandidateProfile(ctx context.Context, profile *core.CandidateProfile) error {
	if profile.ID == "" {
		profile.ID = core.NewID()
	}
	now := time.Now().UTC()
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = now
	}
	profile.UpdatedAt = now
	minSalary := sql.NullInt64{}
	if profile.MinimumSalary != nil {
		minSalary.Int64 = int64(*profile.MinimumSalary)
		minSalary.Valid = true
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO candidate_profiles (
    id, desired_roles, desired_grades, primary_skills, secondary_skills, excluded_skills,
    desired_locations, remote_allowed, relocation_allowed, minimum_salary, currencies,
    employment_types, excluded_companies, stop_words, years_of_commercial_experience,
    languages, work_authorization, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11,
    $12, $13, $14, $15,
    $16, $17, $18, $19
) ON CONFLICT (id) DO UPDATE SET
    desired_roles = EXCLUDED.desired_roles,
    desired_grades = EXCLUDED.desired_grades,
    primary_skills = EXCLUDED.primary_skills,
    secondary_skills = EXCLUDED.secondary_skills,
    excluded_skills = EXCLUDED.excluded_skills,
    desired_locations = EXCLUDED.desired_locations,
    remote_allowed = EXCLUDED.remote_allowed,
    relocation_allowed = EXCLUDED.relocation_allowed,
    minimum_salary = EXCLUDED.minimum_salary,
    currencies = EXCLUDED.currencies,
    employment_types = EXCLUDED.employment_types,
    excluded_companies = EXCLUDED.excluded_companies,
    stop_words = EXCLUDED.stop_words,
    years_of_commercial_experience = EXCLUDED.years_of_commercial_experience,
    languages = EXCLUDED.languages,
    work_authorization = EXCLUDED.work_authorization,
    updated_at = EXCLUDED.updated_at`,
		profile.ID,
		mustJSON(profile.DesiredRoles),
		mustJSON(profile.DesiredGrades),
		mustJSON(profile.PrimarySkills),
		mustJSON(profile.SecondarySkills),
		mustJSON(profile.ExcludedSkills),
		mustJSON(profile.DesiredLocations),
		profile.RemoteAllowed,
		profile.RelocationAllowed,
		minSalary,
		mustJSON(profile.Currencies),
		mustJSON(profile.EmploymentTypes),
		mustJSON(profile.ExcludedCompanies),
		mustJSON(profile.StopWords),
		profile.YearsOfCommercialExperience,
		mustJSON(profile.Languages),
		mustJSON(profile.WorkAuthorization),
		profile.CreatedAt,
		profile.UpdatedAt,
	)
	return err
}

func (s *Store) GetCandidateProfile(ctx context.Context) (*core.CandidateProfile, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, desired_roles, desired_grades, primary_skills, secondary_skills, excluded_skills,
       desired_locations, remote_allowed, relocation_allowed, minimum_salary, currencies,
       employment_types, excluded_companies, stop_words, years_of_commercial_experience,
       languages, work_authorization, created_at, updated_at
FROM candidate_profiles
ORDER BY updated_at DESC
LIMIT 1`)
	profile, err := scanCandidateProfile(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return &profile, nil
}

func (s *Store) UpsertJobSource(ctx context.Context, source *core.JobSource) error {
	if source.ID == "" {
		source.ID = core.NewID()
	}
	now := time.Now().UTC()
	if source.CreatedAt.IsZero() {
		source.CreatedAt = now
	}
	source.UpdatedAt = now
	configuration, err := mustJSONMap(source.Configuration)
	if err != nil {
		return err
	}
	var lastSync any = nil
	if source.LastSuccessfulSyncAt != nil {
		lastSync = *source.LastSuccessfulSyncAt
	}
	var lastErr any = nil
	if source.LastErrorAt != nil {
		lastErr = *source.LastErrorAt
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO job_sources (
    id, type, name, enabled, configuration, last_successful_sync_at, last_error_at, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) ON CONFLICT (id) DO UPDATE SET
    type = EXCLUDED.type,
    name = EXCLUDED.name,
    enabled = EXCLUDED.enabled,
    configuration = EXCLUDED.configuration,
    last_successful_sync_at = EXCLUDED.last_successful_sync_at,
    last_error_at = EXCLUDED.last_error_at,
    updated_at = EXCLUDED.updated_at`,
		source.ID,
		string(source.Type),
		source.Name,
		source.Enabled,
		configuration,
		lastSync,
		lastErr,
		source.CreatedAt,
		source.UpdatedAt,
	)
	return err
}

func (s *Store) EnsureCoreSource(ctx context.Context, source *core.JobSource) error {
	return s.UpsertJobSource(ctx, source)
}

func (s *Store) ListJobSources(ctx context.Context, enabledOnly bool) ([]core.JobSource, error) {
	query := `
SELECT id, type, name, enabled, configuration, last_successful_sync_at, last_error_at, created_at, updated_at
FROM job_sources`
	if enabledOnly {
		query += " WHERE enabled = TRUE"
	}
	query += " ORDER BY created_at ASC"
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]core.JobSource, 0)
	for rows.Next() {
		source, err := scanJobSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, source)
	}
	return out, rows.Err()
}

func (s *Store) GetOrCreateCompany(ctx context.Context, company *core.Company) (*core.Company, error) {
	if company.ID == "" {
		company.ID = core.NewID()
	}
	now := time.Now().UTC()
	if company.CreatedAt.IsZero() {
		company.CreatedAt = now
	}
	company.UpdatedAt = now
	row := s.db.QueryRowContext(ctx, `
INSERT INTO companies (
    id, normalized_name, display_name, website, career_page, blacklisted, notes, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) ON CONFLICT (normalized_name) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    website = COALESCE(EXCLUDED.website, companies.website),
    career_page = COALESCE(EXCLUDED.career_page, companies.career_page),
    blacklisted = companies.blacklisted OR EXCLUDED.blacklisted,
    notes = COALESCE(EXCLUDED.notes, companies.notes),
    updated_at = EXCLUDED.updated_at
RETURNING id, normalized_name, display_name, website, career_page, blacklisted, notes, created_at, updated_at`,
		company.ID,
		company.NormalizedName,
		company.DisplayName,
		nullString(company.Website),
		nullString(company.CareerPage),
		company.Blacklisted,
		nullString(company.Notes),
		company.CreatedAt,
		company.UpdatedAt,
	)
	return scanCompany(row)
}

func (s *Store) GetCompanyByID(ctx context.Context, id string) (*core.Company, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, normalized_name, display_name, website, career_page, blacklisted, notes, created_at, updated_at
FROM companies
WHERE id = $1`, id)
	company, err := scanCompany(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return company, nil
}

func (s *Store) FindVacancyBySourceExternalID(ctx context.Context, sourceID, externalID string) (*core.Vacancy, error) {
	row := s.db.QueryRowContext(ctx, vacancySelectBy("source_id = $1 AND external_id = $2"), sourceID, externalID)
	vacancy, err := scanVacancy(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return &vacancy, nil
}

func (s *Store) FindVacancyByContentHash(ctx context.Context, contentHash string) (*core.Vacancy, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, source_id, external_id, source_url, canonical_url, title, normalized_title, company_id,
       description, requirements, responsibilities, location, remote_type, employment_type, grade,
       salary_from, salary_to, currency, skills, language_requirements, work_authorization_requirements,
       published_at, collected_at, content_hash, status, duplicate_of_vacancy_id, dedup_reason,
       created_at, updated_at
FROM vacancies
WHERE content_hash = $1 AND duplicate_of_vacancy_id IS NULL
ORDER BY created_at ASC
LIMIT 1`, contentHash)
	vacancy, err := scanVacancy(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return &vacancy, nil
}

func (s *Store) UpsertVacancy(ctx context.Context, vacancy *core.Vacancy) error {
	if vacancy.ID == "" {
		vacancy.ID = core.NewID()
	}
	now := time.Now().UTC()
	if vacancy.CreatedAt.IsZero() {
		vacancy.CreatedAt = now
	}
	vacancy.UpdatedAt = now
	var duplicateOf any = nil
	if vacancy.DuplicateOfVacancyID != nil {
		duplicateOf = *vacancy.DuplicateOfVacancyID
	}
	var dedupReason any = nil
	if vacancy.DedupReason != nil {
		dedupReason = *vacancy.DedupReason
	}
	row := s.db.QueryRowContext(ctx, `
INSERT INTO vacancies (
    id, source_id, external_id, source_url, canonical_url, title, normalized_title, company_id,
    description, requirements, responsibilities, location, remote_type, employment_type, grade,
    salary_from, salary_to, currency, skills, language_requirements, work_authorization_requirements,
    published_at, collected_at, content_hash, status, duplicate_of_vacancy_id, dedup_reason,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8,
    $9, $10, $11, $12, $13, $14, $15,
    $16, $17, $18, $19, $20, $21,
    $22, $23, $24, $25, $26, $27,
    $28, $29
) ON CONFLICT (source_id, external_id) DO UPDATE SET
    source_url = EXCLUDED.source_url,
    canonical_url = EXCLUDED.canonical_url,
    title = EXCLUDED.title,
    normalized_title = EXCLUDED.normalized_title,
    company_id = EXCLUDED.company_id,
    description = EXCLUDED.description,
    requirements = EXCLUDED.requirements,
    responsibilities = EXCLUDED.responsibilities,
    location = EXCLUDED.location,
    remote_type = EXCLUDED.remote_type,
    employment_type = EXCLUDED.employment_type,
    grade = EXCLUDED.grade,
    salary_from = EXCLUDED.salary_from,
    salary_to = EXCLUDED.salary_to,
    currency = EXCLUDED.currency,
    skills = EXCLUDED.skills,
    language_requirements = EXCLUDED.language_requirements,
    work_authorization_requirements = EXCLUDED.work_authorization_requirements,
    published_at = EXCLUDED.published_at,
    collected_at = EXCLUDED.collected_at,
    content_hash = EXCLUDED.content_hash,
    status = EXCLUDED.status,
    duplicate_of_vacancy_id = EXCLUDED.duplicate_of_vacancy_id,
    dedup_reason = EXCLUDED.dedup_reason,
    updated_at = EXCLUDED.updated_at
RETURNING id`,
		vacancy.ID,
		vacancy.SourceID,
		vacancy.ExternalID,
		vacancy.SourceURL,
		vacancy.CanonicalURL,
		vacancy.Title,
		vacancy.NormalizedTitle,
		vacancy.CompanyID,
		vacancy.Description,
		vacancy.Requirements,
		vacancy.Responsibilities,
		vacancy.Location,
		vacancy.RemoteType,
		vacancy.EmploymentType,
		vacancy.Grade,
		nullInt(vacancy.SalaryFrom),
		nullInt(vacancy.SalaryTo),
		vacancy.Currency,
		mustJSON(vacancy.Skills),
		mustJSON(vacancy.LanguageRequirements),
		mustJSON(vacancy.WorkAuthorizationRequirements),
		vacancy.PublishedAt,
		vacancy.CollectedAt,
		vacancy.ContentHash,
		string(vacancy.Status),
		duplicateOf,
		dedupReason,
		vacancy.CreatedAt,
		vacancy.UpdatedAt,
	)
	return row.Scan(&vacancy.ID)
}

func (s *Store) UpdateVacancyStatus(ctx context.Context, id string, status core.VacancyStatus, duplicateOf *string, dedupReason *string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE vacancies
SET status = $2,
    duplicate_of_vacancy_id = $3,
    dedup_reason = $4,
    updated_at = NOW()
WHERE id = $1`, id, string(status), duplicateOf, dedupReason)
	return err
}

func (s *Store) GetVacancy(ctx context.Context, id string) (*core.Vacancy, error) {
	row := s.db.QueryRowContext(ctx, vacancySelectBy("id = $1"), id)
	vacancy, err := scanVacancy(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return &vacancy, nil
}

func (s *Store) ListVacancies(ctx context.Context, filter store.ListVacanciesFilter) ([]store.VacancyWithMatch, error) {
	clauses := []string{"1 = 1"}
	args := make([]any, 0, 4)
	if filter.RecommendedOnly {
		clauses = append(clauses, "status = 'RECOMMENDED'")
	} else if filter.Status != nil {
		args = append(args, string(*filter.Status))
		clauses = append(clauses, fmt.Sprintf("status = $%d", len(args)))
	}
	query := vacancySelectBy(strings.Join(clauses, " AND ")) + " ORDER BY collected_at DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, filter.PerPage, filter.Page*filter.PerPage)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]store.VacancyWithMatch, 0)
	for rows.Next() {
		vacancy, err := scanVacancy(rows)
		if err != nil {
			return nil, err
		}
		company, err := s.GetCompanyByID(ctx, vacancy.CompanyID)
		if err != nil {
			return nil, err
		}
		match, err := s.GetVacancyMatch(ctx, vacancy.ID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		out = append(out, store.VacancyWithMatch{Vacancy: vacancy, Company: company, Match: match})
	}
	return out, rows.Err()
}

func (s *Store) ListRecommendedVacancies(ctx context.Context, filter store.ListVacanciesFilter, minScore int) ([]store.VacancyWithMatch, error) {
	clauses := []string{"m.hard_filter_passed = TRUE", "m.total_score >= $1", "v.status = 'RECOMMENDED'"}
	args := []any{minScore}
	if filter.Status != nil {
		args = append(args, string(*filter.Status))
		clauses = append(clauses, fmt.Sprintf("v.status = $%d", len(args)))
	}
	query := `
SELECT v.id, v.source_id, v.external_id, v.source_url, v.canonical_url, v.title, v.normalized_title,
       v.company_id, v.description, v.requirements, v.responsibilities, v.location, v.remote_type,
       v.employment_type, v.grade, v.salary_from, v.salary_to, v.currency, v.skills,
       v.language_requirements, v.work_authorization_requirements, v.published_at, v.collected_at,
       v.content_hash, v.status, v.duplicate_of_vacancy_id, v.dedup_reason, v.created_at, v.updated_at
FROM vacancies v
JOIN vacancy_matches m ON m.vacancy_id = v.id
WHERE ` + strings.Join(clauses, " AND ") + `
ORDER BY m.total_score DESC, v.collected_at DESC
LIMIT $` + fmt.Sprint(len(args)+1) + ` OFFSET $` + fmt.Sprint(len(args)+2)
	args = append(args, filter.PerPage, filter.Page*filter.PerPage)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]store.VacancyWithMatch, 0)
	for rows.Next() {
		vacancy, err := scanVacancy(rows)
		if err != nil {
			return nil, err
		}
		company, err := s.GetCompanyByID(ctx, vacancy.CompanyID)
		if err != nil {
			return nil, err
		}
		match, err := s.GetVacancyMatch(ctx, vacancy.ID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		if match != nil && match.TotalScore >= minScore && match.HardFilterPassed {
			out = append(out, store.VacancyWithMatch{Vacancy: vacancy, Company: company, Match: match})
		}
	}
	return out, rows.Err()
}

func (s *Store) UpsertVacancyMatch(ctx context.Context, match *core.VacancyMatch) error {
	if match.ID == "" {
		match.ID = core.NewID()
	}
	if match.CalculatedAt.IsZero() {
		match.CalculatedAt = time.Now().UTC()
	}
	row := s.db.QueryRowContext(ctx, `
INSERT INTO vacancy_matches (
    id, vacancy_id, candidate_profile_id, total_score, skills_score, experience_score,
    location_score, salary_score, grade_score, role_score, positive_reasons, negative_reasons,
    missing_skills, hard_filter_passed, calculated_at, scoring_version
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11, $12,
    $13, $14, $15, $16
) ON CONFLICT (vacancy_id, candidate_profile_id) DO UPDATE SET
    total_score = EXCLUDED.total_score,
    skills_score = EXCLUDED.skills_score,
    experience_score = EXCLUDED.experience_score,
    location_score = EXCLUDED.location_score,
    salary_score = EXCLUDED.salary_score,
    grade_score = EXCLUDED.grade_score,
    role_score = EXCLUDED.role_score,
    positive_reasons = EXCLUDED.positive_reasons,
    negative_reasons = EXCLUDED.negative_reasons,
    missing_skills = EXCLUDED.missing_skills,
    hard_filter_passed = EXCLUDED.hard_filter_passed,
    calculated_at = EXCLUDED.calculated_at,
    scoring_version = EXCLUDED.scoring_version
RETURNING id`,
		match.ID,
		match.VacancyID,
		match.CandidateProfileID,
		match.TotalScore,
		match.SkillsScore,
		match.ExperienceScore,
		match.LocationScore,
		match.SalaryScore,
		match.GradeScore,
		match.RoleScore,
		mustJSON(match.PositiveReasons),
		mustJSON(match.NegativeReasons),
		mustJSON(match.MissingSkills),
		match.HardFilterPassed,
		match.CalculatedAt,
		match.ScoringVersion,
	)
	return row.Scan(&match.ID)
}

func (s *Store) GetVacancyMatch(ctx context.Context, vacancyID string) (*core.VacancyMatch, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, vacancy_id, candidate_profile_id, total_score, skills_score, experience_score,
       location_score, salary_score, grade_score, role_score, positive_reasons, negative_reasons,
       missing_skills, hard_filter_passed, calculated_at, scoring_version
FROM vacancy_matches
WHERE vacancy_id = $1`, vacancyID)
	match, err := scanVacancyMatch(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return &match, nil
}

func (s *Store) WithinImportTransaction(ctx context.Context, fn func(store.ImportStore) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	txStore := &txStore{tx: tx}
	if err := fn(txStore); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

type txStore struct {
	tx *sql.Tx
}

func (s *txStore) GetOrCreateCompany(ctx context.Context, company *core.Company) (*core.Company, error) {
	if company.ID == "" {
		company.ID = core.NewID()
	}
	now := time.Now().UTC()
	if company.CreatedAt.IsZero() {
		company.CreatedAt = now
	}
	company.UpdatedAt = now
	row := s.tx.QueryRowContext(ctx, `
INSERT INTO companies (
    id, normalized_name, display_name, website, career_page, blacklisted, notes, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) ON CONFLICT (normalized_name) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    website = COALESCE(EXCLUDED.website, companies.website),
    career_page = COALESCE(EXCLUDED.career_page, companies.career_page),
    blacklisted = companies.blacklisted OR EXCLUDED.blacklisted,
    notes = COALESCE(EXCLUDED.notes, companies.notes),
    updated_at = EXCLUDED.updated_at
RETURNING id, normalized_name, display_name, website, career_page, blacklisted, notes, created_at, updated_at`,
		company.ID,
		company.NormalizedName,
		company.DisplayName,
		nullString(company.Website),
		nullString(company.CareerPage),
		company.Blacklisted,
		nullString(company.Notes),
		company.CreatedAt,
		company.UpdatedAt,
	)
	return scanCompany(row)
}

func (s *txStore) FindVacancyBySourceExternalID(ctx context.Context, sourceID, externalID string) (*core.Vacancy, error) {
	row := s.tx.QueryRowContext(ctx, vacancySelectBy("source_id = $1 AND external_id = $2"), sourceID, externalID)
	vacancy, err := scanVacancy(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return &vacancy, nil
}

func (s *txStore) FindVacancyByContentHash(ctx context.Context, contentHash string) (*core.Vacancy, error) {
	row := s.tx.QueryRowContext(ctx, `
SELECT id, source_id, external_id, source_url, canonical_url, title, normalized_title, company_id,
       description, requirements, responsibilities, location, remote_type, employment_type, grade,
       salary_from, salary_to, currency, skills, language_requirements, work_authorization_requirements,
       published_at, collected_at, content_hash, status, duplicate_of_vacancy_id, dedup_reason,
       created_at, updated_at
FROM vacancies
WHERE content_hash = $1 AND duplicate_of_vacancy_id IS NULL
ORDER BY created_at ASC
LIMIT 1`, contentHash)
	vacancy, err := scanVacancy(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return &vacancy, nil
}

func (s *txStore) UpsertVacancy(ctx context.Context, vacancy *core.Vacancy) error {
	if vacancy.ID == "" {
		vacancy.ID = core.NewID()
	}
	now := time.Now().UTC()
	if vacancy.CreatedAt.IsZero() {
		vacancy.CreatedAt = now
	}
	vacancy.UpdatedAt = now
	var duplicateOf any = nil
	if vacancy.DuplicateOfVacancyID != nil {
		duplicateOf = *vacancy.DuplicateOfVacancyID
	}
	var dedupReason any = nil
	if vacancy.DedupReason != nil {
		dedupReason = *vacancy.DedupReason
	}
	row := s.tx.QueryRowContext(ctx, `
INSERT INTO vacancies (
    id, source_id, external_id, source_url, canonical_url, title, normalized_title, company_id,
    description, requirements, responsibilities, location, remote_type, employment_type, grade,
    salary_from, salary_to, currency, skills, language_requirements, work_authorization_requirements,
    published_at, collected_at, content_hash, status, duplicate_of_vacancy_id, dedup_reason,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8,
    $9, $10, $11, $12, $13, $14, $15,
    $16, $17, $18, $19, $20, $21,
    $22, $23, $24, $25, $26, $27,
    $28, $29
) ON CONFLICT (source_id, external_id) DO UPDATE SET
    source_url = EXCLUDED.source_url,
    canonical_url = EXCLUDED.canonical_url,
    title = EXCLUDED.title,
    normalized_title = EXCLUDED.normalized_title,
    company_id = EXCLUDED.company_id,
    description = EXCLUDED.description,
    requirements = EXCLUDED.requirements,
    responsibilities = EXCLUDED.responsibilities,
    location = EXCLUDED.location,
    remote_type = EXCLUDED.remote_type,
    employment_type = EXCLUDED.employment_type,
    grade = EXCLUDED.grade,
    salary_from = EXCLUDED.salary_from,
    salary_to = EXCLUDED.salary_to,
    currency = EXCLUDED.currency,
    skills = EXCLUDED.skills,
    language_requirements = EXCLUDED.language_requirements,
    work_authorization_requirements = EXCLUDED.work_authorization_requirements,
    published_at = EXCLUDED.published_at,
    collected_at = EXCLUDED.collected_at,
    content_hash = EXCLUDED.content_hash,
    status = EXCLUDED.status,
    duplicate_of_vacancy_id = EXCLUDED.duplicate_of_vacancy_id,
    dedup_reason = EXCLUDED.dedup_reason,
    updated_at = EXCLUDED.updated_at
RETURNING id`,
		vacancy.ID,
		vacancy.SourceID,
		vacancy.ExternalID,
		vacancy.SourceURL,
		vacancy.CanonicalURL,
		vacancy.Title,
		vacancy.NormalizedTitle,
		vacancy.CompanyID,
		vacancy.Description,
		vacancy.Requirements,
		vacancy.Responsibilities,
		vacancy.Location,
		vacancy.RemoteType,
		vacancy.EmploymentType,
		vacancy.Grade,
		nullInt(vacancy.SalaryFrom),
		nullInt(vacancy.SalaryTo),
		vacancy.Currency,
		mustJSON(vacancy.Skills),
		mustJSON(vacancy.LanguageRequirements),
		mustJSON(vacancy.WorkAuthorizationRequirements),
		vacancy.PublishedAt,
		vacancy.CollectedAt,
		vacancy.ContentHash,
		string(vacancy.Status),
		duplicateOf,
		dedupReason,
		vacancy.CreatedAt,
		vacancy.UpdatedAt,
	)
	return row.Scan(&vacancy.ID)
}

func (s *txStore) UpsertVacancyMatch(ctx context.Context, match *core.VacancyMatch) error {
	if match.ID == "" {
		match.ID = core.NewID()
	}
	if match.CalculatedAt.IsZero() {
		match.CalculatedAt = time.Now().UTC()
	}
	row := s.tx.QueryRowContext(ctx, `
INSERT INTO vacancy_matches (
    id, vacancy_id, candidate_profile_id, total_score, skills_score, experience_score,
    location_score, salary_score, grade_score, role_score, positive_reasons, negative_reasons,
    missing_skills, hard_filter_passed, calculated_at, scoring_version
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11, $12,
    $13, $14, $15, $16
) ON CONFLICT (vacancy_id, candidate_profile_id) DO UPDATE SET
    total_score = EXCLUDED.total_score,
    skills_score = EXCLUDED.skills_score,
    experience_score = EXCLUDED.experience_score,
    location_score = EXCLUDED.location_score,
    salary_score = EXCLUDED.salary_score,
    grade_score = EXCLUDED.grade_score,
    role_score = EXCLUDED.role_score,
    positive_reasons = EXCLUDED.positive_reasons,
    negative_reasons = EXCLUDED.negative_reasons,
    missing_skills = EXCLUDED.missing_skills,
    hard_filter_passed = EXCLUDED.hard_filter_passed,
    calculated_at = EXCLUDED.calculated_at,
    scoring_version = EXCLUDED.scoring_version
RETURNING id`,
		match.ID,
		match.VacancyID,
		match.CandidateProfileID,
		match.TotalScore,
		match.SkillsScore,
		match.ExperienceScore,
		match.LocationScore,
		match.SalaryScore,
		match.GradeScore,
		match.RoleScore,
		mustJSON(match.PositiveReasons),
		mustJSON(match.NegativeReasons),
		mustJSON(match.MissingSkills),
		match.HardFilterPassed,
		match.CalculatedAt,
		match.ScoringVersion,
	)
	return row.Scan(&match.ID)
}

func vacancySelectBy(where string) string {
	return `
SELECT id, source_id, external_id, source_url, canonical_url, title, normalized_title, company_id,
       description, requirements, responsibilities, location, remote_type, employment_type, grade,
       salary_from, salary_to, currency, skills, language_requirements, work_authorization_requirements,
       published_at, collected_at, content_hash, status, duplicate_of_vacancy_id, dedup_reason,
       created_at, updated_at
FROM vacancies
WHERE ` + where
}

func scanCandidateProfile(scanner interface{ Scan(...any) error }) (core.CandidateProfile, error) {
	var profile core.CandidateProfile
	var desiredRoles, desiredGrades, primarySkills, secondarySkills, excludedSkills, desiredLocations, currencies, employmentTypes, excludedCompanies, stopWords, languages, workAuthorization []string
	var minimumSalary sql.NullInt64
	if err := scanner.Scan(
		&profile.ID,
		jsonSliceScanner{target: &desiredRoles},
		jsonSliceScanner{target: &desiredGrades},
		jsonSliceScanner{target: &primarySkills},
		jsonSliceScanner{target: &secondarySkills},
		jsonSliceScanner{target: &excludedSkills},
		jsonSliceScanner{target: &desiredLocations},
		&profile.RemoteAllowed,
		&profile.RelocationAllowed,
		&minimumSalary,
		jsonSliceScanner{target: &currencies},
		jsonSliceScanner{target: &employmentTypes},
		jsonSliceScanner{target: &excludedCompanies},
		jsonSliceScanner{target: &stopWords},
		&profile.YearsOfCommercialExperience,
		jsonSliceScanner{target: &languages},
		jsonSliceScanner{target: &workAuthorization},
		&profile.CreatedAt,
		&profile.UpdatedAt,
	); err != nil {
		return core.CandidateProfile{}, err
	}
	profile.DesiredRoles = desiredRoles
	profile.DesiredGrades = desiredGrades
	profile.PrimarySkills = primarySkills
	profile.SecondarySkills = secondarySkills
	profile.ExcludedSkills = excludedSkills
	profile.DesiredLocations = desiredLocations
	if minimumSalary.Valid {
		v := int(minimumSalary.Int64)
		profile.MinimumSalary = &v
	}
	profile.Currencies = currencies
	profile.EmploymentTypes = employmentTypes
	profile.ExcludedCompanies = excludedCompanies
	profile.StopWords = stopWords
	profile.Languages = languages
	profile.WorkAuthorization = workAuthorization
	return profile, nil
}

func scanJobSource(scanner interface{ Scan(...any) error }) (core.JobSource, error) {
	var source core.JobSource
	var config map[string]any
	var lastSync, lastErr sql.NullTime
	if err := scanner.Scan(
		&source.ID,
		&source.Type,
		&source.Name,
		&source.Enabled,
		jsonMapScanner{target: &config},
		&lastSync,
		&lastErr,
		&source.CreatedAt,
		&source.UpdatedAt,
	); err != nil {
		return core.JobSource{}, err
	}
	source.Configuration = config
	if lastSync.Valid {
		t := lastSync.Time
		source.LastSuccessfulSyncAt = &t
	}
	if lastErr.Valid {
		t := lastErr.Time
		source.LastErrorAt = &t
	}
	return source, nil
}

func scanCompany(scanner interface{ Scan(...any) error }) (*core.Company, error) {
	var company core.Company
	var website, careerPage, notes sql.NullString
	if err := scanner.Scan(
		&company.ID,
		&company.NormalizedName,
		&company.DisplayName,
		&website,
		&careerPage,
		&company.Blacklisted,
		&notes,
		&company.CreatedAt,
		&company.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if website.Valid {
		company.Website = website.String
	}
	if careerPage.Valid {
		company.CareerPage = careerPage.String
	}
	if notes.Valid {
		company.Notes = notes.String
	}
	return &company, nil
}

func scanVacancy(scanner interface{ Scan(...any) error }) (core.Vacancy, error) {
	var vacancy core.Vacancy
	var salaryFrom, salaryTo sql.NullInt64
	var skills, languages, auth []string
	var publishedAt sql.NullTime
	var duplicateOf, dedupReason sql.NullString
	if err := scanner.Scan(
		&vacancy.ID,
		&vacancy.SourceID,
		&vacancy.ExternalID,
		&vacancy.SourceURL,
		&vacancy.CanonicalURL,
		&vacancy.Title,
		&vacancy.NormalizedTitle,
		&vacancy.CompanyID,
		&vacancy.Description,
		&vacancy.Requirements,
		&vacancy.Responsibilities,
		&vacancy.Location,
		&vacancy.RemoteType,
		&vacancy.EmploymentType,
		&vacancy.Grade,
		&salaryFrom,
		&salaryTo,
		&vacancy.Currency,
		jsonSliceScanner{target: &skills},
		jsonSliceScanner{target: &languages},
		jsonSliceScanner{target: &auth},
		&publishedAt,
		&vacancy.CollectedAt,
		&vacancy.ContentHash,
		&vacancy.Status,
		&duplicateOf,
		&dedupReason,
		&vacancy.CreatedAt,
		&vacancy.UpdatedAt,
	); err != nil {
		return core.Vacancy{}, err
	}
	if salaryFrom.Valid {
		v := int(salaryFrom.Int64)
		vacancy.SalaryFrom = &v
	}
	if salaryTo.Valid {
		v := int(salaryTo.Int64)
		vacancy.SalaryTo = &v
	}
	vacancy.Skills = skills
	vacancy.LanguageRequirements = languages
	vacancy.WorkAuthorizationRequirements = auth
	if publishedAt.Valid {
		vacancy.PublishedAt = publishedAt.Time
	}
	if duplicateOf.Valid {
		v := duplicateOf.String
		vacancy.DuplicateOfVacancyID = &v
	}
	if dedupReason.Valid {
		v := dedupReason.String
		vacancy.DedupReason = &v
	}
	return vacancy, nil
}

func scanVacancyMatch(scanner interface{ Scan(...any) error }) (core.VacancyMatch, error) {
	var match core.VacancyMatch
	var positive, negative, missing []string
	if err := scanner.Scan(
		&match.ID,
		&match.VacancyID,
		&match.CandidateProfileID,
		&match.TotalScore,
		&match.SkillsScore,
		&match.ExperienceScore,
		&match.LocationScore,
		&match.SalaryScore,
		&match.GradeScore,
		&match.RoleScore,
		jsonSliceScanner{target: &positive},
		jsonSliceScanner{target: &negative},
		jsonSliceScanner{target: &missing},
		&match.HardFilterPassed,
		&match.CalculatedAt,
		&match.ScoringVersion,
	); err != nil {
		return core.VacancyMatch{}, err
	}
	match.PositiveReasons = positive
	match.NegativeReasons = negative
	match.MissingSkills = missing
	return match, nil
}

func mustJSON(values []string) []byte {
	out, _ := json.Marshal(values)
	return out
}

func mustJSONMap(values map[string]any) ([]byte, error) {
	if values == nil {
		values = map[string]any{}
	}
	return json.Marshal(values)
}

func nullString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

type jsonSliceScanner struct {
	target *[]string
}

func (s jsonSliceScanner) Scan(src any) error {
	switch value := src.(type) {
	case nil:
		*s.target = nil
		return nil
	case []byte:
		if len(value) == 0 {
			*s.target = nil
			return nil
		}
		return json.Unmarshal(value, s.target)
	case string:
		if value == "" {
			*s.target = nil
			return nil
		}
		return json.Unmarshal([]byte(value), s.target)
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, s.target)
	}
}

type jsonMapScanner struct {
	target *map[string]any
}

func (s jsonMapScanner) Scan(src any) error {
	switch value := src.(type) {
	case nil:
		*s.target = map[string]any{}
		return nil
	case []byte:
		if len(value) == 0 {
			*s.target = map[string]any{}
			return nil
		}
		return json.Unmarshal(value, s.target)
	case string:
		if value == "" {
			*s.target = map[string]any{}
			return nil
		}
		return json.Unmarshal([]byte(value), s.target)
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, s.target)
	}
}
