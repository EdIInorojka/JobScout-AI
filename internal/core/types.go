package core

import "time"

type SourceType string

const (
	SourceTypeHeadhunterAPI     SourceType = "HEADHUNTER_API"
	SourceTypeManualURL         SourceType = "MANUAL_URL"
	SourceTypeTelegramMessage   SourceType = "TELEGRAM_MESSAGE"
	SourceTypeEmailImport       SourceType = "EMAIL_IMPORT"
	SourceTypeCompanyCareerPage SourceType = "COMPANY_CAREER_PAGE"
)

func (t SourceType) Valid() bool {
	switch t {
	case SourceTypeHeadhunterAPI, SourceTypeManualURL, SourceTypeTelegramMessage, SourceTypeEmailImport, SourceTypeCompanyCareerPage:
		return true
	default:
		return false
	}
}

type VacancyStatus string

const (
	VacancyStatusDiscovered          VacancyStatus = "DISCOVERED"
	VacancyStatusNormalized          VacancyStatus = "NORMALIZED"
	VacancyStatusDuplicate           VacancyStatus = "DUPLICATE"
	VacancyStatusFilteredOut         VacancyStatus = "FILTERED_OUT"
	VacancyStatusRecommended         VacancyStatus = "RECOMMENDED"
	VacancyStatusApplicationPrepared VacancyStatus = "APPLICATION_PREPARED"
	VacancyStatusWaitingApproval     VacancyStatus = "WAITING_APPROVAL"
	VacancyStatusSubmitted           VacancyStatus = "SUBMITTED"
	VacancyStatusViewed              VacancyStatus = "VIEWED"
	VacancyStatusHRContact           VacancyStatus = "HR_CONTACT"
	VacancyStatusInterview           VacancyStatus = "INTERVIEW"
	VacancyStatusOffer               VacancyStatus = "OFFER"
	VacancyStatusRejected            VacancyStatus = "REJECTED"
	VacancyStatusArchived            VacancyStatus = "ARCHIVED"
)

func (s VacancyStatus) Valid() bool {
	switch s {
	case VacancyStatusDiscovered, VacancyStatusNormalized, VacancyStatusDuplicate, VacancyStatusFilteredOut, VacancyStatusRecommended, VacancyStatusApplicationPrepared, VacancyStatusWaitingApproval, VacancyStatusSubmitted, VacancyStatusViewed, VacancyStatusHRContact, VacancyStatusInterview, VacancyStatusOffer, VacancyStatusRejected, VacancyStatusArchived:
		return true
	default:
		return false
	}
}

var vacancyTransitions = map[VacancyStatus]map[VacancyStatus]struct{}{
	VacancyStatusDiscovered: {
		VacancyStatusNormalized:  {},
		VacancyStatusDuplicate:   {},
		VacancyStatusFilteredOut: {},
		VacancyStatusRecommended: {},
		VacancyStatusArchived:    {},
	},
	VacancyStatusNormalized: {
		VacancyStatusDuplicate:   {},
		VacancyStatusFilteredOut: {},
		VacancyStatusRecommended: {},
		VacancyStatusArchived:    {},
	},
	VacancyStatusDuplicate: {
		VacancyStatusArchived: {},
	},
	VacancyStatusFilteredOut: {
		VacancyStatusArchived: {},
	},
	VacancyStatusRecommended: {
		VacancyStatusApplicationPrepared: {},
		VacancyStatusWaitingApproval:     {},
		VacancyStatusSubmitted:           {},
		VacancyStatusViewed:              {},
		VacancyStatusHRContact:           {},
		VacancyStatusInterview:           {},
		VacancyStatusOffer:               {},
		VacancyStatusRejected:            {},
		VacancyStatusArchived:            {},
	},
	VacancyStatusApplicationPrepared: {
		VacancyStatusWaitingApproval: {},
		VacancyStatusArchived:        {},
	},
	VacancyStatusWaitingApproval: {
		VacancyStatusSubmitted: {},
		VacancyStatusArchived:  {},
	},
	VacancyStatusSubmitted: {
		VacancyStatusViewed:    {},
		VacancyStatusHRContact: {},
		VacancyStatusInterview: {},
		VacancyStatusOffer:     {},
		VacancyStatusRejected:  {},
		VacancyStatusArchived:  {},
	},
	VacancyStatusViewed: {
		VacancyStatusHRContact: {},
		VacancyStatusInterview: {},
		VacancyStatusOffer:     {},
		VacancyStatusRejected:  {},
		VacancyStatusArchived:  {},
	},
	VacancyStatusHRContact: {
		VacancyStatusInterview: {},
		VacancyStatusOffer:     {},
		VacancyStatusRejected:  {},
		VacancyStatusArchived:  {},
	},
	VacancyStatusInterview: {
		VacancyStatusOffer:    {},
		VacancyStatusRejected: {},
		VacancyStatusArchived: {},
	},
	VacancyStatusOffer: {
		VacancyStatusArchived: {},
	},
	VacancyStatusRejected: {
		VacancyStatusArchived: {},
	},
	VacancyStatusArchived: {},
}

func CanTransitionVacancyStatus(from, to VacancyStatus) bool {
	if from == to {
		return true
	}
	next, ok := vacancyTransitions[from]
	if !ok {
		return false
	}
	_, ok = next[to]
	return ok
}

type Recommendation string

const (
	RecommendationApply  Recommendation = "APPLY"
	RecommendationReview Recommendation = "REVIEW"
	RecommendationSkip   Recommendation = "SKIP"
)

type CandidateProfile struct {
	ID                          string    `json:"id"`
	DesiredRoles                []string  `json:"desiredRoles"`
	DesiredGrades               []string  `json:"desiredGrades"`
	PrimarySkills               []string  `json:"primarySkills"`
	SecondarySkills             []string  `json:"secondarySkills"`
	ExcludedSkills              []string  `json:"excludedSkills"`
	DesiredLocations            []string  `json:"desiredLocations"`
	RemoteAllowed               bool      `json:"remoteAllowed"`
	RelocationAllowed           bool      `json:"relocationAllowed"`
	MinimumSalary               *int      `json:"minimumSalary,omitempty"`
	Currencies                  []string  `json:"currencies"`
	EmploymentTypes             []string  `json:"employmentTypes"`
	ExcludedCompanies           []string  `json:"excludedCompanies"`
	StopWords                   []string  `json:"stopWords"`
	YearsOfCommercialExperience int       `json:"yearsOfCommercialExperience"`
	Languages                   []string  `json:"languages"`
	WorkAuthorization           []string  `json:"workAuthorization"`
	CreatedAt                   time.Time `json:"createdAt"`
	UpdatedAt                   time.Time `json:"updatedAt"`
}

type JobSource struct {
	ID                   string         `json:"id"`
	Type                 SourceType     `json:"type"`
	Name                 string         `json:"name"`
	Enabled              bool           `json:"enabled"`
	Configuration        map[string]any `json:"configuration,omitempty"`
	LastSuccessfulSyncAt *time.Time     `json:"lastSuccessfulSyncAt,omitempty"`
	LastErrorAt          *time.Time     `json:"lastErrorAt,omitempty"`
	CreatedAt            time.Time      `json:"createdAt"`
	UpdatedAt            time.Time      `json:"updatedAt"`
}

type Company struct {
	ID             string    `json:"id"`
	NormalizedName string    `json:"normalizedName"`
	DisplayName    string    `json:"displayName"`
	Website        string    `json:"website,omitempty"`
	CareerPage     string    `json:"careerPage,omitempty"`
	Blacklisted    bool      `json:"blacklisted"`
	Notes          string    `json:"notes,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type Vacancy struct {
	ID                            string        `json:"id"`
	SourceID                      string        `json:"sourceId"`
	ExternalID                    string        `json:"externalId"`
	SourceURL                     string        `json:"sourceUrl"`
	CanonicalURL                  string        `json:"canonicalUrl,omitempty"`
	Title                         string        `json:"title"`
	NormalizedTitle               string        `json:"normalizedTitle"`
	CompanyID                     string        `json:"companyId"`
	Description                   string        `json:"description"`
	Requirements                  string        `json:"requirements"`
	Responsibilities              string        `json:"responsibilities"`
	Location                      string        `json:"location"`
	RemoteType                    string        `json:"remoteType"`
	EmploymentType                string        `json:"employmentType"`
	Grade                         string        `json:"grade"`
	SalaryFrom                    *int          `json:"salaryFrom,omitempty"`
	SalaryTo                      *int          `json:"salaryTo,omitempty"`
	Currency                      string        `json:"currency,omitempty"`
	Skills                        []string      `json:"skills"`
	LanguageRequirements          []string      `json:"languageRequirements"`
	WorkAuthorizationRequirements []string      `json:"workAuthorizationRequirements"`
	PublishedAt                   time.Time     `json:"publishedAt"`
	CollectedAt                   time.Time     `json:"collectedAt"`
	ContentHash                   string        `json:"contentHash"`
	Status                        VacancyStatus `json:"status"`
	DuplicateOfVacancyID          *string       `json:"duplicateOfVacancyId,omitempty"`
	DedupReason                   *string       `json:"dedupReason,omitempty"`
	CreatedAt                     time.Time     `json:"createdAt"`
	UpdatedAt                     time.Time     `json:"updatedAt"`
}

type VacancyMatch struct {
	ID                 string    `json:"id"`
	VacancyID          string    `json:"vacancyId"`
	CandidateProfileID string    `json:"candidateProfileId"`
	TotalScore         int       `json:"totalScore"`
	SkillsScore        int       `json:"skillsScore"`
	ExperienceScore    int       `json:"experienceScore"`
	LocationScore      int       `json:"locationScore"`
	SalaryScore        int       `json:"salaryScore"`
	GradeScore         int       `json:"gradeScore"`
	RoleScore          int       `json:"roleScore"`
	PositiveReasons    []string  `json:"positiveReasons"`
	NegativeReasons    []string  `json:"negativeReasons"`
	MissingSkills      []string  `json:"missingSkills"`
	HardFilterPassed   bool      `json:"hardFilterPassed"`
	CalculatedAt       time.Time `json:"calculatedAt"`
	ScoringVersion     string    `json:"scoringVersion"`
}

type RawVacancy struct {
	SourceType                    SourceType
	SourceID                      string
	ExternalID                    string
	SourceURL                     string
	CanonicalURL                  string
	Title                         string
	CompanyName                   string
	Description                   string
	Requirements                  string
	Responsibilities              string
	Location                      string
	RemoteType                    string
	EmploymentType                string
	Grade                         string
	SalaryFrom                    *int
	SalaryTo                      *int
	Currency                      string
	Skills                        []string
	LanguageRequirements          []string
	WorkAuthorizationRequirements []string
	PublishedAt                   time.Time
	CollectedAt                   time.Time
}

type NormalizedVacancy struct {
	RawVacancy
	NormalizedTitle          string
	NormalizedCompanyName    string
	ContentHash              string
	StrippedDescription      string
	StrippedRequirements     string
	StrippedResponsibilities string
}

type HardFilterResult struct {
	Passed  bool     `json:"passed"`
	Reasons []string `json:"reasons"`
}

type ScoringWeights struct {
	RoleWeight       int `json:"roleWeight"`
	SkillsWeight     int `json:"skillsWeight"`
	ExperienceWeight int `json:"experienceWeight"`
	GradeWeight      int `json:"gradeWeight"`
	LocationWeight   int `json:"locationWeight"`
	SalaryWeight     int `json:"salaryWeight"`
	DomainWeight     int `json:"domainWeight"`
}

type ScoringConfig struct {
	Weights         ScoringWeights `json:"weights"`
	ApplyThreshold  int            `json:"applyThreshold"`
	ReviewThreshold int            `json:"reviewThreshold"`
	Version         string         `json:"version"`
}

type ScoringResult struct {
	TotalScore       int            `json:"totalScore"`
	RoleScore        int            `json:"roleScore"`
	SkillsScore      int            `json:"skillsScore"`
	ExperienceScore  int            `json:"experienceScore"`
	LocationScore    int            `json:"locationScore"`
	SalaryScore      int            `json:"salaryScore"`
	GradeScore       int            `json:"gradeScore"`
	DomainScore      int            `json:"domainScore"`
	PositiveReasons  []string       `json:"positiveReasons"`
	NegativeReasons  []string       `json:"negativeReasons"`
	MissingSkills    []string       `json:"missingSkills"`
	Warnings         []string       `json:"warnings"`
	Recommendation   Recommendation `json:"recommendation"`
	HardFilterPassed bool           `json:"hardFilterPassed"`
	Version          string         `json:"version"`
}
