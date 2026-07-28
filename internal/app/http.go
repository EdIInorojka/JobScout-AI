package app

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"jobscout.ai/internal/core"
	"jobscout.ai/internal/store"
)

func (a *App) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", a.handleHealth)
	mux.HandleFunc("/v1/profile", a.handleProfile)
	mux.HandleFunc("/v1/search", a.handleSearch)
	mux.HandleFunc("/v1/vacancies", a.handleVacancies)
	mux.HandleFunc("/v1/vacancies/recommended", a.handleRecommendedVacancies)
	mux.HandleFunc("/v1/vacancies/import-url", a.handleImportVacancy)
	mux.HandleFunc("/v1/vacancies/", a.handleVacancyByID)
	return mux
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) handleProfile(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		profile, err := a.GetProfile(r.Context())
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, profile)
	case http.MethodPost, http.MethodPut:
		var profile core.CandidateProfile
		if err := readJSON(r, &profile); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		saved, err := a.SaveProfile(r.Context(), &profile)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, saved)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost, http.MethodPut)
	}
}

func (a *App) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var req SearchRequest
	if err := readJSON(r, &req); err != nil && !errors.Is(err, errEmptyBody) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	summary, err := a.RunSearch(r.Context(), req.Query)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (a *App) handleRecommendedVacancies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	page, perPage := paginationFromQuery(r)
	items, err := a.ListRecommendedVacancies(r.Context(), page, perPage)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if items == nil {
		items = []store.VacancyWithMatch{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *App) handleVacancies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	page, perPage := paginationFromQuery(r)
	var status *core.VacancyStatus
	if rawStatus := strings.TrimSpace(r.URL.Query().Get("status")); rawStatus != "" {
		parsed, err := core.ParseVacancyStatus(rawStatus)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		status = &parsed
	}
	items, err := a.ListVacancies(r.Context(), page, perPage, status)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if items == nil {
		items = []store.VacancyWithMatch{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *App) handleImportVacancy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var req ManualImportRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.ImportManualVacancy(r.Context(), req)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrManualTextRequired) {
			status = http.StatusUnprocessableEntity
		}
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *App) handleVacancyByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/vacancies/")
	if path == "" {
		writeError(w, http.StatusNotFound, "vacancy id is required")
		return
	}
	if strings.HasSuffix(path, "/status") {
		id := strings.TrimSuffix(path, "/status")
		id = strings.TrimSuffix(id, "/")
		a.handleVacancyStatus(w, r, id)
		return
	}
	id := strings.Trim(path, "/")
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	item, err := a.GetVacancy(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (a *App) handleVacancyStatus(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPatch {
		methodNotAllowed(w, http.MethodPatch)
		return
	}
	var req StatusUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status, err := core.ParseVacancyStatus(req.Status)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := a.UpdateVacancyStatus(r.Context(), id, status)
	if err != nil {
		if errors.Is(err, ErrInvalidStatusChange) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func paginationFromQuery(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	return normalizePagination(page, perPage)
}

var errEmptyBody = errors.New("empty body")

func readJSON(r *http.Request, dest any) error {
	if r.Body == nil {
		return errEmptyBody
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return errEmptyBody
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func methodNotAllowed(w http.ResponseWriter, allowed ...string) {
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}
