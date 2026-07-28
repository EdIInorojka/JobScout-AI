package app

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net/url"
	"path"
	"strings"
	"time"

	"jobscout.ai/internal/config"
	"jobscout.ai/internal/core"
	"jobscout.ai/internal/integrations/hh"
	tele "jobscout.ai/internal/integrations/telegram"
	"jobscout.ai/internal/store"
)

var (
	ErrProfileNotConfigured = errors.New("candidate profile is not configured")
	ErrManualTextRequired   = errors.New("manual vacancy text is required when automatic retrieval is unavailable")
	ErrUnknownUser          = errors.New("unknown telegram user")
	ErrInvalidStatusChange  = errors.New("invalid vacancy status transition")
)

type App struct {
	cfg      config.Config
	storage  store.Store
	hhClient *hh.Client
	tgClient *tele.Client
	logger   *slog.Logger
	now      func() time.Time
}

type SearchSummary struct {
	Found       int `json:"found"`
	Imported    int `json:"imported"`
	Duplicates  int `json:"duplicates"`
	Filtered    int `json:"filtered"`
	Recommended int `json:"recommended"`
	Errors      int `json:"errors"`
}

type ManualImportRequest struct {
	URL         string `json:"url"`
	Text        string `json:"text,omitempty"`
	Title       string `json:"title,omitempty"`
	CompanyName string `json:"companyName,omitempty"`
	Location    string `json:"location,omitempty"`
}

type SearchRequest struct {
	Query string `json:"query,omitempty"`
}

type StatusUpdateRequest struct {
	Status string `json:"status"`
}

func New(cfg config.Config, storage store.Store, hhClient *hh.Client, tgClient *tele.Client, logger *slog.Logger) *App {
	if logger == nil {
		logger = slog.Default()
	}
	return &App{
		cfg:      cfg,
		storage:  storage,
		hhClient: hhClient,
		tgClient: tgClient,
		logger:   logger,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (a *App) SeedCoreSources(ctx context.Context) error {
	sources, err := a.storage.ListJobSources(ctx, false)
	if err != nil {
		return err
	}
	if !hasSourceType(sources, core.SourceTypeHeadhunterAPI) {
		if err := a.storage.UpsertJobSource(ctx, &core.JobSource{
			ID:      core.NewID(),
			Type:    core.SourceTypeHeadhunterAPI,
			Name:    "HeadHunter",
			Enabled: true,
			Configuration: map[string]any{
				"host":     "hh.ru",
				"pageSize": a.cfg.HHPageSize,
			},
		}); err != nil {
			return err
		}
	}
	if !hasSourceType(sources, core.SourceTypeManualURL) {
		if err := a.storage.UpsertJobSource(ctx, &core.JobSource{
			ID:            core.NewID(),
			Type:          core.SourceTypeManualURL,
			Name:          "Manual URL",
			Enabled:       true,
			Configuration: map[string]any{},
		}); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) GetProfile(ctx context.Context) (*core.CandidateProfile, error) {
	profile, err := a.storage.GetCandidateProfile(ctx)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrProfileNotConfigured
		}
		return nil, err
	}
	return profile, nil
}

func (a *App) SaveProfile(ctx context.Context, profile *core.CandidateProfile) (*core.CandidateProfile, error) {
	if current, err := a.storage.GetCandidateProfile(ctx); err == nil && current != nil {
		profile.ID = current.ID
	}
	if profile.ID == "" {
		profile.ID = core.NewID()
	}
	if profile.CreatedAt.IsZero() {
		if current, err := a.storage.GetCandidateProfile(ctx); err == nil && current != nil {
			profile.CreatedAt = current.CreatedAt
		}
	}
	if err := a.storage.UpsertCandidateProfile(ctx, profile); err != nil {
		return nil, err
	}
	return a.storage.GetCandidateProfile(ctx)
}

func (a *App) ListVacancies(ctx context.Context, page, perPage int, status *core.VacancyStatus) ([]store.VacancyWithMatch, error) {
	page, perPage = normalizePagination(page, perPage)
	return a.storage.ListVacancies(ctx, store.ListVacanciesFilter{
		Page:    page,
		PerPage: perPage,
		Status:  status,
	})
}

func (a *App) ListRecommendedVacancies(ctx context.Context, page, perPage int) ([]store.VacancyWithMatch, error) {
	page, perPage = normalizePagination(page, perPage)
	return a.storage.ListRecommendedVacancies(ctx, store.ListVacanciesFilter{Page: page, PerPage: perPage}, a.cfg.Scoring.ReviewThreshold)
}

func (a *App) GetVacancy(ctx context.Context, id string) (*store.VacancyWithMatch, error) {
	vacancy, err := a.storage.GetVacancy(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		return nil, err
	}
	company, err := a.storage.GetCompanyByID(ctx, vacancy.CompanyID)
	if err != nil {
		return nil, err
	}
	match, err := a.storage.GetVacancyMatch(ctx, vacancy.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	return &store.VacancyWithMatch{Vacancy: *vacancy, Company: company, Match: match}, nil
}

func (a *App) UpdateVacancyStatus(ctx context.Context, id string, status core.VacancyStatus) (*store.VacancyWithMatch, error) {
	if !status.Valid() {
		return nil, fmt.Errorf("unknown vacancy status: %s", status)
	}
	vacancy, err := a.storage.GetVacancy(ctx, id)
	if err != nil {
		return nil, err
	}
	if !core.CanTransitionVacancyStatus(vacancy.Status, status) {
		return nil, ErrInvalidStatusChange
	}
	if err := a.storage.UpdateVacancyStatus(ctx, id, status, vacancy.DuplicateOfVacancyID, vacancy.DedupReason); err != nil {
		return nil, err
	}
	return a.GetVacancy(ctx, id)
}

func (a *App) RunSearch(ctx context.Context, query string) (SearchSummary, error) {
	profile, err := a.GetProfile(ctx)
	if err != nil {
		return SearchSummary{}, err
	}
	if a.hhClient == nil {
		return SearchSummary{}, fmt.Errorf("headhunter client is not configured")
	}
	sources, err := a.storage.ListJobSources(ctx, true)
	if err != nil {
		return SearchSummary{}, err
	}
	var hhSources []core.JobSource
	for _, source := range sources {
		if source.Type == core.SourceTypeHeadhunterAPI {
			hhSources = append(hhSources, source)
		}
	}
	if len(hhSources) == 0 {
		return SearchSummary{}, fmt.Errorf("no enabled headhunter sources configured")
	}
	searchQuery := buildSearchQuery(profile, query)
	if strings.TrimSpace(searchQuery) == "" {
		return SearchSummary{}, fmt.Errorf("search query is empty")
	}
	now := a.now()
	summary := SearchSummary{}
	for _, source := range hhSources {
		if err := a.runHeadhunterSource(ctx, source, searchQuery, profile, &summary, now); err != nil {
			source.LastErrorAt = &now
			_ = a.storage.UpsertJobSource(ctx, &source)
			return summary, err
		}
		source.LastSuccessfulSyncAt = &now
		source.LastErrorAt = nil
		_ = a.storage.UpsertJobSource(ctx, &source)
	}
	return summary, nil
}

func (a *App) ImportManualVacancy(ctx context.Context, req ManualImportRequest) (*store.VacancyWithMatch, error) {
	profile, err := a.GetProfile(ctx)
	if err != nil {
		return nil, err
	}
	now := a.now()
	sourceType := detectSourceType(req.URL)
	switch sourceType {
	case core.SourceTypeHeadhunterAPI:
		if a.hhClient == nil {
			return nil, fmt.Errorf("headhunter client is not configured")
		}
		source, err := a.ensureSourceByType(ctx, core.SourceTypeHeadhunterAPI)
		if err != nil {
			return nil, err
		}
		vacancyID := extractHHVacancyID(req.URL)
		if vacancyID == "" {
			return nil, fmt.Errorf("cannot parse headhunter vacancy id from url")
		}
		details, err := a.hhClient.GetVacancy(ctx, vacancyID)
		if err != nil {
			return nil, err
		}
		raw := details.ToRawVacancy(source.ID, vacancyID, req.URL, now)
		return a.importVacancyFromRaw(ctx, *source, raw, profile)
	default:
		if strings.TrimSpace(req.Text) == "" {
			return nil, ErrManualTextRequired
		}
		source, err := a.ensureSourceByType(ctx, core.SourceTypeManualURL)
		if err != nil {
			return nil, err
		}
		raw := core.RawVacancy{
			SourceType:       core.SourceTypeManualURL,
			SourceID:         source.ID,
			ExternalID:       canonicalManualExternalID(req.URL, req.Text, req.Title),
			SourceURL:        req.URL,
			CanonicalURL:     core.CanonicalURL(req.URL),
			Title:            firstNonEmpty(req.Title, extractFirstLine(req.Text), "Manual vacancy"),
			CompanyName:      firstNonEmpty(req.CompanyName, hostnameFromURL(req.URL), "Manual source"),
			Description:      req.Text,
			Requirements:     "",
			Responsibilities: "",
			Location:         req.Location,
			CollectedAt:      now,
			PublishedAt:      now,
		}
		return a.importVacancyFromRaw(ctx, *source, raw, profile)
	}
}

func (a *App) runHeadhunterSource(ctx context.Context, source core.JobSource, query string, profile *core.CandidateProfile, summary *SearchSummary, now time.Time) error {
	pageSize := a.cfg.HHPageSize
	maxPages := a.cfg.HHMaxPages
	for page := 0; page < maxPages; page++ {
		resp, err := a.hhClient.SearchVacancies(ctx, query, page, pageSize)
		if err != nil {
			return err
		}
		if page == 0 {
			summary.Found += resp.Found
		}
		if len(resp.Items) == 0 {
			break
		}
		for _, item := range resp.Items {
			details, err := a.hhClient.GetVacancy(ctx, item.ID)
			if err != nil {
				summary.Errors++
				a.logger.Warn("headhunter vacancy fetch failed", "vacancy_id", item.ID, "error", err)
				continue
			}
			raw := details.ToRawVacancy(source.ID, item.ID, item.AlternateURL, now)
			imported, err := a.importVacancyFromRaw(ctx, source, raw, profile)
			if err != nil {
				summary.Errors++
				a.logger.Warn("vacancy import failed", "vacancy_id", item.ID, "error", err)
				continue
			}
			summary.Imported++
			if imported.Vacancy.Status == core.VacancyStatusDuplicate {
				summary.Duplicates++
			}
			if imported.Vacancy.Status == core.VacancyStatusFilteredOut {
				summary.Filtered++
			}
			if imported.Match != nil && imported.Match.HardFilterPassed && imported.Match.TotalScore >= a.cfg.Scoring.ReviewThreshold {
				summary.Recommended++
			}
		}
		if page+1 >= resp.Pages {
			break
		}
	}
	return nil
}

func (a *App) importVacancyFromRaw(ctx context.Context, source core.JobSource, raw core.RawVacancy, profile *core.CandidateProfile) (*store.VacancyWithMatch, error) {
	now := a.now()
	normalized := core.NormalizeVacancy(raw)
	companyName := firstNonEmpty(raw.CompanyName, hostnameFromURL(raw.SourceURL), "unknown company")
	company, err := a.storage.GetOrCreateCompany(ctx, &core.Company{
		ID:             core.NewID(),
		NormalizedName: core.NormalizeText(companyName),
		DisplayName:    companyName,
		Blacklisted:    false,
	})
	if err != nil {
		return nil, err
	}
	existing, err := a.storage.FindVacancyBySourceExternalID(ctx, source.ID, raw.ExternalID)
	var existingStatus core.VacancyStatus
	var existingCreatedAt time.Time
	if err == nil {
		existingStatus = existing.Status
		existingCreatedAt = existing.CreatedAt
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	duplicateOf, dedupReason, isDuplicate, err := a.detectDedup(ctx, source.ID, raw.ExternalID, normalized.ContentHash, existing)
	if err != nil {
		return nil, err
	}
	vacancy := core.Vacancy{
		SourceID:                      source.ID,
		ExternalID:                    raw.ExternalID,
		SourceURL:                     raw.SourceURL,
		CanonicalURL:                  raw.CanonicalURL,
		Title:                         raw.Title,
		NormalizedTitle:               normalized.NormalizedTitle,
		CompanyID:                     company.ID,
		Description:                   normalized.StrippedDescription,
		Requirements:                  normalized.StrippedRequirements,
		Responsibilities:              normalized.StrippedResponsibilities,
		Location:                      raw.Location,
		RemoteType:                    raw.RemoteType,
		EmploymentType:                raw.EmploymentType,
		Grade:                         raw.Grade,
		SalaryFrom:                    raw.SalaryFrom,
		SalaryTo:                      raw.SalaryTo,
		Currency:                      raw.Currency,
		Skills:                        core.NormalizeStringList(raw.Skills),
		LanguageRequirements:          core.NormalizeStringList(raw.LanguageRequirements),
		WorkAuthorizationRequirements: core.NormalizeStringList(raw.WorkAuthorizationRequirements),
		PublishedAt:                   raw.PublishedAt,
		CollectedAt:                   raw.CollectedAt,
		ContentHash:                   normalized.ContentHash,
		Status:                        core.VacancyStatusDiscovered,
		DuplicateOfVacancyID:          duplicateOf,
		DedupReason:                   dedupReason,
	}
	hardFilter := core.ApplyHardFilters(*profile, vacancy, *company, now, core.FilterConfig{MaxVacancyAgeDays: a.cfg.MaxVacancyAgeDays})
	score := core.ScoreVacancy(*profile, vacancy, *company, a.cfg.Scoring, hardFilter.Passed)
	if isDuplicate {
		vacancy.Status = core.VacancyStatusDuplicate
	} else if !hardFilter.Passed {
		vacancy.Status = core.VacancyStatusFilteredOut
	} else if score.Recommendation == core.RecommendationApply || score.Recommendation == core.RecommendationReview {
		vacancy.Status = core.VacancyStatusRecommended
	} else {
		vacancy.Status = core.VacancyStatusNormalized
	}
	if shouldPreserveStatus(existingStatus, vacancy.Status) {
		vacancy.Status = existingStatus
	}
	if existing != nil {
		vacancy.ID = existing.ID
		if existingCreatedAt.IsZero() {
			existingCreatedAt = existing.CreatedAt
		}
	}
	if existingCreatedAt.IsZero() {
		existingCreatedAt = now
	}
	vacancy.CreatedAt = existingCreatedAt
	vacancy.UpdatedAt = now
	if err := a.storage.UpsertVacancy(ctx, &vacancy); err != nil {
		return nil, err
	}
	match := &core.VacancyMatch{
		VacancyID:          vacancy.ID,
		CandidateProfileID: profile.ID,
		TotalScore:         score.TotalScore,
		SkillsScore:        score.SkillsScore,
		ExperienceScore:    score.ExperienceScore,
		LocationScore:      score.LocationScore,
		SalaryScore:        score.SalaryScore,
		GradeScore:         score.GradeScore,
		RoleScore:          score.RoleScore,
		PositiveReasons:    score.PositiveReasons,
		NegativeReasons:    append(score.NegativeReasons, hardFilter.Reasons...),
		MissingSkills:      score.MissingSkills,
		HardFilterPassed:   hardFilter.Passed,
		CalculatedAt:       now,
		ScoringVersion:     score.Version,
	}
	if err := a.storage.UpsertVacancyMatch(ctx, match); err != nil {
		return nil, err
	}
	vacancyWithRelations := &store.VacancyWithMatch{Vacancy: vacancy, Company: company, Match: match}
	return vacancyWithRelations, nil
}

func (a *App) detectDedup(ctx context.Context, sourceID, externalID, contentHash string, existing *core.Vacancy) (*string, *string, bool, error) {
	if existing != nil {
		return nil, nil, false, nil
	}
	primary, err := a.storage.FindVacancyByContentHash(ctx, contentHash)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, false, nil
		}
		return nil, nil, false, err
	}
	reason := string(core.DedupReasonContentHash)
	return &primary.ID, &reason, true, nil
}

func (a *App) ensureSourceByType(ctx context.Context, typ core.SourceType) (*core.JobSource, error) {
	sources, err := a.storage.ListJobSources(ctx, false)
	if err != nil {
		return nil, err
	}
	for _, source := range sources {
		if source.Type == typ {
			cp := source
			return &cp, nil
		}
	}
	var source *core.JobSource
	switch typ {
	case core.SourceTypeHeadhunterAPI:
		source = &core.JobSource{
			ID:      core.NewID(),
			Type:    core.SourceTypeHeadhunterAPI,
			Name:    "HeadHunter",
			Enabled: true,
			Configuration: map[string]any{
				"host": "hh.ru",
			},
		}
	case core.SourceTypeManualURL:
		source = &core.JobSource{
			ID:            core.NewID(),
			Type:          core.SourceTypeManualURL,
			Name:          "Manual URL",
			Enabled:       true,
			Configuration: map[string]any{},
		}
	default:
		return nil, fmt.Errorf("unsupported source type: %s", typ)
	}
	if err := a.storage.UpsertJobSource(ctx, source); err != nil {
		return nil, err
	}
	return source, nil
}

func buildSearchQuery(profile *core.CandidateProfile, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	parts := append([]string{}, profile.DesiredRoles...)
	parts = append(parts, profile.PrimarySkills...)
	parts = append(parts, profile.SecondarySkills...)
	parts = append(parts, profile.Languages...)
	parts = append(parts, profile.DesiredLocations...)
	parts = core.NormalizeStringList(parts)
	return strings.Join(parts, " ")
}

func normalizePagination(page, perPage int) (int, int) {
	if page < 0 {
		page = 0
	}
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	return page, perPage
}

func shouldPreserveStatus(existing, computed core.VacancyStatus) bool {
	if existing == "" || existing == computed {
		return false
	}
	switch existing {
	case core.VacancyStatusArchived, core.VacancyStatusRejected, core.VacancyStatusOffer, core.VacancyStatusInterview, core.VacancyStatusHRContact, core.VacancyStatusSubmitted, core.VacancyStatusWaitingApproval, core.VacancyStatusApplicationPrepared, core.VacancyStatusViewed:
		return true
	default:
		return false
	}
}

func hasSourceType(sources []core.JobSource, typ core.SourceType) bool {
	for _, source := range sources {
		if source.Type == typ {
			return true
		}
	}
	return false
}

func detectSourceType(rawURL string) core.SourceType {
	host := strings.ToLower(hostnameFromURL(rawURL))
	if strings.Contains(host, "hh.ru") || strings.Contains(host, "headhunter") || strings.Contains(host, "api.hh.ru") {
		return core.SourceTypeHeadhunterAPI
	}
	return core.SourceTypeManualURL
}

func extractHHVacancyID(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i := 0; i < len(segments); i++ {
		if segments[i] == "vacancy" || segments[i] == "vacancies" {
			if i+1 < len(segments) {
				return segments[i+1]
			}
		}
	}
	return path.Base(parsed.Path)
}

func canonicalManualExternalID(rawURL, text, title string) string {
	seed := strings.TrimSpace(rawURL + "|" + title + "|" + extractFirstLine(text))
	if seed == "" {
		return core.NewID()
	}
	return fmt.Sprintf("manual-%x", coreHash(seed))
}

func coreHash(value string) []byte {
	h := fnv.New64a()
	_, _ = h.Write([]byte(value))
	sum := h.Sum(nil)
	return sum
}

func hostnameFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func extractFirstLine(text string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
