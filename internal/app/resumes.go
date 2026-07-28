package app

import (
	"context"
	"strings"

	"jobscout.ai/internal/core"
	"jobscout.ai/internal/store"
)

type ResumeCreateRequest struct {
	Name        string   `json:"name"`
	TargetRole  string   `json:"targetRole"`
	Language    string   `json:"language"`
	TextContent string   `json:"textContent"`
	Skills      []string `json:"skills"`
	IsActive    *bool    `json:"isActive,omitempty"`
}

type ResumePatchRequest struct {
	Name        *string   `json:"name,omitempty"`
	TargetRole  *string   `json:"targetRole,omitempty"`
	Language    *string   `json:"language,omitempty"`
	TextContent *string   `json:"textContent,omitempty"`
	Skills      *[]string `json:"skills,omitempty"`
	IsActive    *bool     `json:"isActive,omitempty"`
}

func (a *App) ListResumes(ctx context.Context) ([]core.Resume, error) {
	profile, err := a.GetProfile(ctx)
	if err != nil {
		return nil, err
	}
	items, err := a.storage.ListResumes(ctx, profile.ID)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []core.Resume{}
	}
	return items, nil
}

func (a *App) GetResume(ctx context.Context, id string) (*core.Resume, error) {
	resume, err := a.storage.GetResume(ctx, id)
	if err != nil {
		return nil, err
	}
	return resume, nil
}

func (a *App) CreateResume(ctx context.Context, req ResumeCreateRequest) (*core.Resume, error) {
	profile, err := a.GetProfile(ctx)
	if err != nil {
		return nil, err
	}
	role, err := core.ParseResumeTargetRole(strings.TrimSpace(req.TargetRole))
	if err != nil {
		return nil, err
	}
	language, err := core.ParseResumeLanguage(strings.TrimSpace(req.Language))
	if err != nil {
		return nil, err
	}
	active := true
	if req.IsActive != nil {
		active = *req.IsActive
	}
	resume := &core.Resume{
		CandidateProfileID: profile.ID,
		Name:               strings.TrimSpace(req.Name),
		TargetRole:         role,
		Language:           language,
		TextContent:        strings.TrimSpace(req.TextContent),
		Skills:             req.Skills,
		IsActive:           active,
	}
	if err := resume.Validate(); err != nil {
		return nil, err
	}
	if err := a.storage.UpsertResume(ctx, resume); err != nil {
		return nil, err
	}
	return a.storage.GetResume(ctx, resume.ID)
}

func (a *App) UpdateResume(ctx context.Context, id string, req ResumePatchRequest) (*core.Resume, error) {
	current, err := a.storage.GetResume(ctx, id)
	if err != nil {
		return nil, err
	}
	updated := *current
	if req.Name != nil {
		updated.Name = strings.TrimSpace(*req.Name)
	}
	if req.TargetRole != nil {
		role, err := core.ParseResumeTargetRole(strings.TrimSpace(*req.TargetRole))
		if err != nil {
			return nil, err
		}
		updated.TargetRole = role
	}
	if req.Language != nil {
		language, err := core.ParseResumeLanguage(strings.TrimSpace(*req.Language))
		if err != nil {
			return nil, err
		}
		updated.Language = language
	}
	if req.TextContent != nil {
		updated.TextContent = strings.TrimSpace(*req.TextContent)
	}
	if req.Skills != nil {
		updated.Skills = *req.Skills
	}
	if req.IsActive != nil {
		updated.IsActive = *req.IsActive
	}
	if err := updated.Validate(); err != nil {
		return nil, err
	}
	if err := a.storage.UpsertResume(ctx, &updated); err != nil {
		return nil, err
	}
	return a.storage.GetResume(ctx, id)
}

func isResumeErrorNotFound(err error) bool {
	return err != nil && (err == store.ErrNotFound || err == core.ErrResumeNotFound)
}
