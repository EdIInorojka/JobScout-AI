package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var ErrNoSuitableResume = errors.New("no suitable resume found")
var ErrResumeNotFound = errors.New("resume not found")

type ResumeTargetRole string

const (
	ResumeTargetRoleGoBackend      ResumeTargetRole = "GO_BACKEND"
	ResumeTargetRolePythonBackend  ResumeTargetRole = "PYTHON_BACKEND"
	ResumeTargetRoleSystemAnalyst  ResumeTargetRole = "SYSTEM_ANALYST"
	ResumeTargetRoleGeneralBackend ResumeTargetRole = "GENERAL_BACKEND"
)

func (r ResumeTargetRole) Valid() bool {
	switch r {
	case ResumeTargetRoleGoBackend, ResumeTargetRolePythonBackend, ResumeTargetRoleSystemAnalyst, ResumeTargetRoleGeneralBackend:
		return true
	default:
		return false
	}
}

func ParseResumeTargetRole(value string) (ResumeTargetRole, error) {
	role := ResumeTargetRole(value)
	if !role.Valid() {
		return "", fmt.Errorf("invalid resume target role: %s", value)
	}
	return role, nil
}

func ResumeTargetRoleLabel(role ResumeTargetRole) string {
	switch role {
	case ResumeTargetRoleGoBackend:
		return "Go backend"
	case ResumeTargetRolePythonBackend:
		return "Python backend"
	case ResumeTargetRoleSystemAnalyst:
		return "системный аналитик"
	default:
		return "backend-разработчик"
	}
}

type ResumeLanguage string

const (
	ResumeLanguageRU ResumeLanguage = "RU"
	ResumeLanguageEN ResumeLanguage = "EN"
)

func (l ResumeLanguage) Valid() bool {
	switch l {
	case ResumeLanguageRU, ResumeLanguageEN:
		return true
	default:
		return false
	}
}

func ParseResumeLanguage(value string) (ResumeLanguage, error) {
	language := ResumeLanguage(value)
	if !language.Valid() {
		return "", fmt.Errorf("invalid resume language: %s", value)
	}
	return language, nil
}

type Resume struct {
	ID                 string           `json:"id"`
	CandidateProfileID string           `json:"candidateProfileId"`
	Name               string           `json:"name"`
	TargetRole         ResumeTargetRole `json:"targetRole"`
	Language           ResumeLanguage   `json:"language"`
	TextContent        string           `json:"textContent"`
	Skills             []string         `json:"skills"`
	IsActive           bool             `json:"isActive"`
	CreatedAt          time.Time        `json:"createdAt"`
	UpdatedAt          time.Time        `json:"updatedAt"`
}

func (r Resume) Validate() error {
	switch {
	case strings.TrimSpace(r.CandidateProfileID) == "":
		return errors.New("candidate profile id is required")
	case strings.TrimSpace(r.Name) == "":
		return errors.New("resume name is required")
	case !r.TargetRole.Valid():
		return fmt.Errorf("invalid resume target role: %s", r.TargetRole)
	case !r.Language.Valid():
		return fmt.Errorf("invalid resume language: %s", r.Language)
	case strings.TrimSpace(r.TextContent) == "":
		return errors.New("resume text content is required")
	}
	return nil
}

type ResumeSelectionInput struct {
	ManualResumeID string
	Vacancy        Vacancy
	Match          *VacancyMatch
	Resumes        []Resume
}

type ResumeSelector interface {
	Select(ctx context.Context, input ResumeSelectionInput) (Resume, error)
}

type DeterministicResumeSelector struct{}

func NewDeterministicResumeSelector() DeterministicResumeSelector {
	return DeterministicResumeSelector{}
}

func (DeterministicResumeSelector) Select(ctx context.Context, input ResumeSelectionInput) (Resume, error) {
	if manual := strings.TrimSpace(input.ManualResumeID); manual != "" {
		for _, resume := range input.Resumes {
			if resume.ID == manual {
				return resume, nil
			}
		}
		return Resume{}, fmt.Errorf("%w: %s", ErrResumeNotFound, manual)
	}

	targetRole := inferResumeTargetRole(input.Vacancy, input.Match)
	candidates := filterResumesByRole(input.Resumes, targetRole, true)
	if len(candidates) == 0 && targetRole != ResumeTargetRoleGeneralBackend {
		candidates = filterResumesByRole(input.Resumes, ResumeTargetRoleGeneralBackend, true)
	}
	if len(candidates) == 0 {
		return Resume{}, fmt.Errorf("%w: %s", ErrNoSuitableResume, targetRole)
	}

	vacancySkills := normalizeSkillSet(input.Vacancy.Skills)
	type scoredResume struct {
		resume Resume
		score  int
	}
	scored := make([]scoredResume, 0, len(candidates))
	for _, resume := range candidates {
		score := overlapCount(resume.Skills, vacancySkills)
		scored = append(scored, scoredResume{resume: resume, score: score})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if !scored[i].resume.CreatedAt.Equal(scored[j].resume.CreatedAt) {
			return scored[i].resume.CreatedAt.Before(scored[j].resume.CreatedAt)
		}
		return scored[i].resume.ID < scored[j].resume.ID
	})
	return scored[0].resume, nil
}

func inferResumeTargetRole(vacancy Vacancy, match *VacancyMatch) ResumeTargetRole {
	blob := strings.Join([]string{
		NormalizeText(vacancy.NormalizedTitle),
		NormalizeText(vacancy.Title),
		NormalizeText(vacancy.Description),
		NormalizeText(vacancy.Requirements),
		NormalizeText(vacancy.Responsibilities),
		strings.Join(NormalizeStringList(vacancy.Skills), " "),
	}, " ")

	if match != nil {
		for _, reason := range match.PositiveReasons {
			blob += " " + NormalizeText(reason)
		}
	}

	switch {
	case containsAny(blob, "system analyst", "systems analyst", "системный аналитик", "business analyst", "business system analyst"):
		return ResumeTargetRoleSystemAnalyst
	case containsAny(blob, "python", "django", "flask", "fastapi", "pandas", "sqlalchemy"):
		return ResumeTargetRolePythonBackend
	case containsAny(blob, "go", "golang", "grpc", "microservice", "microservices"):
		return ResumeTargetRoleGoBackend
	default:
		return ResumeTargetRoleGeneralBackend
	}
}

func filterResumesByRole(resumes []Resume, role ResumeTargetRole, activeOnly bool) []Resume {
	out := make([]Resume, 0, len(resumes))
	for _, resume := range resumes {
		if activeOnly && !resume.IsActive {
			continue
		}
		if resume.TargetRole == role {
			out = append(out, resume)
		}
	}
	return out
}

func normalizeSkillSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range NormalizeStringList(values) {
		out[value] = struct{}{}
	}
	return out
}

func overlapCount(values []string, set map[string]struct{}) int {
	if len(values) == 0 || len(set) == 0 {
		return 0
	}
	score := 0
	for _, value := range NormalizeStringList(values) {
		if _, ok := set[value]; ok {
			score++
		}
	}
	return score
}

func containsAny(blob string, needles ...string) bool {
	blob = NormalizeText(blob)
	for _, needle := range needles {
		if strings.Contains(blob, NormalizeText(needle)) {
			return true
		}
	}
	return false
}
