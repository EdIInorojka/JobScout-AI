CREATE TABLE IF NOT EXISTS candidate_profiles (
    id TEXT PRIMARY KEY,
    desired_roles JSONB NOT NULL DEFAULT '[]'::jsonb,
    desired_grades JSONB NOT NULL DEFAULT '[]'::jsonb,
    primary_skills JSONB NOT NULL DEFAULT '[]'::jsonb,
    secondary_skills JSONB NOT NULL DEFAULT '[]'::jsonb,
    excluded_skills JSONB NOT NULL DEFAULT '[]'::jsonb,
    desired_locations JSONB NOT NULL DEFAULT '[]'::jsonb,
    remote_allowed BOOLEAN NOT NULL DEFAULT FALSE,
    relocation_allowed BOOLEAN NOT NULL DEFAULT FALSE,
    minimum_salary INTEGER NULL,
    currencies JSONB NOT NULL DEFAULT '[]'::jsonb,
    employment_types JSONB NOT NULL DEFAULT '[]'::jsonb,
    excluded_companies JSONB NOT NULL DEFAULT '[]'::jsonb,
    stop_words JSONB NOT NULL DEFAULT '[]'::jsonb,
    years_of_commercial_experience INTEGER NOT NULL DEFAULT 0,
    languages JSONB NOT NULL DEFAULT '[]'::jsonb,
    work_authorization JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS job_sources (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    name TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    configuration JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_successful_sync_at TIMESTAMPTZ NULL,
    last_error_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT job_sources_type_check CHECK (type IN (
        'HEADHUNTER_API',
        'MANUAL_URL',
        'TELEGRAM_MESSAGE',
        'EMAIL_IMPORT',
        'COMPANY_CAREER_PAGE'
    ))
);

CREATE TABLE IF NOT EXISTS companies (
    id TEXT PRIMARY KEY,
    normalized_name TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    website TEXT NULL,
    career_page TEXT NULL,
    blacklisted BOOLEAN NOT NULL DEFAULT FALSE,
    notes TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS vacancies (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES job_sources(id) ON DELETE RESTRICT,
    external_id TEXT NOT NULL,
    source_url TEXT NOT NULL,
    canonical_url TEXT NULL,
    title TEXT NOT NULL,
    normalized_title TEXT NOT NULL,
    company_id TEXT NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    description TEXT NOT NULL DEFAULT '',
    requirements TEXT NOT NULL DEFAULT '',
    responsibilities TEXT NOT NULL DEFAULT '',
    location TEXT NOT NULL DEFAULT '',
    remote_type TEXT NOT NULL DEFAULT '',
    employment_type TEXT NOT NULL DEFAULT '',
    grade TEXT NOT NULL DEFAULT '',
    salary_from INTEGER NULL,
    salary_to INTEGER NULL,
    currency TEXT NULL,
    skills JSONB NOT NULL DEFAULT '[]'::jsonb,
    language_requirements JSONB NOT NULL DEFAULT '[]'::jsonb,
    work_authorization_requirements JSONB NOT NULL DEFAULT '[]'::jsonb,
    published_at TIMESTAMPTZ NULL,
    collected_at TIMESTAMPTZ NOT NULL,
    content_hash TEXT NOT NULL,
    status TEXT NOT NULL,
    duplicate_of_vacancy_id TEXT NULL REFERENCES vacancies(id) ON DELETE SET NULL,
    dedup_reason TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT vacancies_status_check CHECK (status IN (
        'DISCOVERED',
        'NORMALIZED',
        'DUPLICATE',
        'FILTERED_OUT',
        'RECOMMENDED',
        'APPLICATION_PREPARED',
        'WAITING_APPROVAL',
        'SUBMITTED',
        'VIEWED',
        'HR_CONTACT',
        'INTERVIEW',
        'OFFER',
        'REJECTED',
        'ARCHIVED'
    )),
    CONSTRAINT vacancies_source_external_unique UNIQUE (source_id, external_id)
);

CREATE INDEX IF NOT EXISTS vacancies_content_hash_idx ON vacancies(content_hash);
CREATE INDEX IF NOT EXISTS vacancies_status_idx ON vacancies(status);
CREATE INDEX IF NOT EXISTS vacancies_company_idx ON vacancies(company_id);
CREATE INDEX IF NOT EXISTS vacancies_collected_at_idx ON vacancies(collected_at DESC);

CREATE TABLE IF NOT EXISTS vacancy_matches (
    id TEXT PRIMARY KEY,
    vacancy_id TEXT NOT NULL REFERENCES vacancies(id) ON DELETE CASCADE,
    candidate_profile_id TEXT NOT NULL REFERENCES candidate_profiles(id) ON DELETE CASCADE,
    total_score INTEGER NOT NULL,
    skills_score INTEGER NOT NULL,
    experience_score INTEGER NOT NULL,
    location_score INTEGER NOT NULL,
    salary_score INTEGER NOT NULL,
    grade_score INTEGER NOT NULL,
    role_score INTEGER NOT NULL,
    positive_reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
    negative_reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
    missing_skills JSONB NOT NULL DEFAULT '[]'::jsonb,
    hard_filter_passed BOOLEAN NOT NULL DEFAULT FALSE,
    calculated_at TIMESTAMPTZ NOT NULL,
    scoring_version TEXT NOT NULL,
    CONSTRAINT vacancy_matches_unique UNIQUE (vacancy_id, candidate_profile_id)
);

CREATE INDEX IF NOT EXISTS vacancy_matches_total_score_idx ON vacancy_matches(total_score DESC);
CREATE INDEX IF NOT EXISTS vacancy_matches_hard_filter_idx ON vacancy_matches(hard_filter_passed);

