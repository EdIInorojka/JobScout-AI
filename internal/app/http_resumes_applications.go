package app

import (
	"errors"
	"net/http"
	"strings"

	"jobscout.ai/internal/core"
	"jobscout.ai/internal/store"
)

func (a *App) handleResumes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := a.ListResumes(r.Context())
		if err != nil {
			writeAPIError(w, err)
			return
		}
		if items == nil {
			items = []core.Resume{}
		}
		writeJSON(w, http.StatusOK, items)
	case http.MethodPost:
		var req ResumeCreateRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		resume, err := a.CreateResume(r.Context(), req)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, resume)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (a *App) handleResumeByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/resumes/")
	id = strings.Trim(id, "/")
	if id == "" {
		writeError(w, http.StatusNotFound, "resume id is required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		resume, err := a.GetResume(r.Context(), id)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resume)
	case http.MethodPatch:
		var req ResumePatchRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		resume, err := a.UpdateResume(r.Context(), id, req)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resume)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPatch)
	}
}

func (a *App) handleApplications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	items, err := a.ListApplications(r.Context())
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if items == nil {
		items = []ApplicationView{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *App) handleApplicationByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/applications/")
	if path == "" {
		writeError(w, http.StatusNotFound, "application id is required")
		return
	}
	switch {
	case strings.HasSuffix(path, "/approve"):
		id := strings.TrimSuffix(path, "/approve")
		id = strings.Trim(id, "/")
		a.handleApproveApplication(w, r, id)
	case strings.HasSuffix(path, "/cancel"):
		id := strings.TrimSuffix(path, "/cancel")
		id = strings.Trim(id, "/")
		a.handleCancelApplication(w, r, id)
	case strings.HasSuffix(path, "/mark-submitted"):
		id := strings.TrimSuffix(path, "/mark-submitted")
		id = strings.Trim(id, "/")
		a.handleMarkApplicationSubmitted(w, r, id)
	case strings.HasSuffix(path, "/outcome"):
		id := strings.TrimSuffix(path, "/outcome")
		id = strings.Trim(id, "/")
		a.handleUpdateApplicationOutcome(w, r, id)
	default:
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		id := strings.Trim(path, "/")
		view, err := a.GetApplication(r.Context(), id)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

func (a *App) handlePrepareApplication(w http.ResponseWriter, r *http.Request, vacancyID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var req PrepareApplicationRequest
	if err := readJSON(r, &req); err != nil && !errors.Is(err, errEmptyBody) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	view, err := a.PrepareApplication(r.Context(), vacancyID, req.ResumeID, requestActor(r))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	status := http.StatusOK
	if view.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, view)
}

func (a *App) handleApproveApplication(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	view, err := a.ApproveApplication(r.Context(), id, requestActor(r))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (a *App) handleCancelApplication(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	view, err := a.CancelApplication(r.Context(), id, requestActor(r))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (a *App) handleMarkApplicationSubmitted(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	view, err := a.MarkApplicationSubmitted(r.Context(), id, requestActor(r))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (a *App) handleUpdateApplicationOutcome(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPatch {
		methodNotAllowed(w, http.MethodPatch)
		return
	}
	var req ApplicationOutcomeRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	view, err := a.UpdateApplicationOutcome(r.Context(), id, req, requestActor(r))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func writeAPIError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, core.ErrResumeNotFound), errors.Is(err, ErrProfileNotConfigured):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, store.ErrConflict), errors.Is(err, ErrVacancyArchived), errors.Is(err, ErrInvalidApplicationTransition), errors.Is(err, core.ErrNoSuitableResume):
		writeError(w, http.StatusConflict, err.Error())
	case isValidationError(err):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func requestActor(r *http.Request) string {
	if actor := strings.TrimSpace(r.Header.Get("X-Actor")); actor != "" {
		return actor
	}
	return "http"
}

func isValidationError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "required") || strings.Contains(text, "invalid ") || strings.Contains(text, "must be") || strings.Contains(text, "empty")
}
