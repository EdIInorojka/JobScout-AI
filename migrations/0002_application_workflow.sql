CREATE TABLE IF NOT EXISTS resumes (
    id TEXT PRIMARY KEY,
    candidate_profile_id TEXT NOT NULL REFERENCES candidate_profiles(id) ON DELETE RESTRICT,
    name TEXT NOT NULL,
    target_role TEXT NOT NULL,
    language TEXT NOT NULL,
    text_content TEXT NOT NULL,
    skills JSONB NOT NULL DEFAULT '[]'::jsonb,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT resumes_target_role_check CHECK (target_role IN (
        'GO_BACKEND',
        'PYTHON_BACKEND',
        'SYSTEM_ANALYST',
        'GENERAL_BACKEND'
    )),
    CONSTRAINT resumes_language_check CHECK (language IN ('RU', 'EN')),
    CONSTRAINT resumes_name_not_blank CHECK (length(trim(name)) > 0),
    CONSTRAINT resumes_text_not_blank CHECK (length(trim(text_content)) > 0)
);

CREATE INDEX IF NOT EXISTS resumes_candidate_profile_idx ON resumes(candidate_profile_id, is_active, target_role, created_at ASC, id ASC);

CREATE TABLE IF NOT EXISTS applications (
    id TEXT PRIMARY KEY,
    vacancy_id TEXT NOT NULL REFERENCES vacancies(id) ON DELETE RESTRICT,
    candidate_profile_id TEXT NOT NULL REFERENCES candidate_profiles(id) ON DELETE RESTRICT,
    resume_id TEXT NOT NULL REFERENCES resumes(id) ON DELETE RESTRICT,
    status TEXT NOT NULL,
    application_method TEXT NOT NULL DEFAULT 'MANUAL_LINK',
    cover_letter TEXT NOT NULL,
    prepared_at TIMESTAMPTZ NOT NULL,
    approved_at TIMESTAMPTZ NULL,
    submitted_at TIMESTAMPTZ NULL,
    response_received_at TIMESTAMPTZ NULL,
    rejection_reason TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT applications_status_check CHECK (status IN (
        'DRAFT',
        'WAITING_APPROVAL',
        'APPROVED',
        'MANUAL_ACTION_REQUIRED',
        'SUBMITTED',
        'CANCELLED',
        'HR_CONTACT',
        'INTERVIEW',
        'OFFER',
        'REJECTED'
    )),
    CONSTRAINT applications_method_check CHECK (application_method IN ('MANUAL_LINK', 'OFFICIAL_API'))
);

CREATE UNIQUE INDEX IF NOT EXISTS applications_active_unique_idx
    ON applications(vacancy_id, candidate_profile_id)
    WHERE status NOT IN ('CANCELLED', 'REJECTED');

CREATE INDEX IF NOT EXISTS applications_candidate_profile_idx ON applications(candidate_profile_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS applications_vacancy_idx ON applications(vacancy_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS applications_status_idx ON applications(status, created_at DESC);
CREATE INDEX IF NOT EXISTS applications_resume_idx ON applications(resume_id);

CREATE TABLE IF NOT EXISTS audit_events (
    id TEXT PRIMARY KEY,
    actor TEXT NOT NULL,
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS audit_events_entity_idx ON audit_events(entity_type, entity_id, created_at ASC, id ASC);
CREATE INDEX IF NOT EXISTS audit_events_action_idx ON audit_events(action, created_at DESC);
