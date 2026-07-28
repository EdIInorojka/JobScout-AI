package core

import (
	"context"
	"fmt"
	"strings"
)

type CoverLetterGenerator interface {
	Generate(ctx context.Context, input CoverLetterInput) (string, error)
}

type CoverLetterInput struct {
	CandidateName             string
	TargetRole                ResumeTargetRole
	CommercialExperienceYears int
	ProjectExperience         string
	ResumeSkills              []string
	Vacancy                   Vacancy
	Match                     *VacancyMatch
	PositiveReasons           []string
	MissingSkills             []string
	CompanyName               string
}

type DeterministicCoverLetterGenerator struct{}

func NewDeterministicCoverLetterGenerator() DeterministicCoverLetterGenerator {
	return DeterministicCoverLetterGenerator{}
}

func (DeterministicCoverLetterGenerator) Generate(ctx context.Context, input CoverLetterInput) (string, error) {
	_ = ctx
	candidateName := strings.TrimSpace(input.CandidateName)
	companyName := strings.TrimSpace(input.CompanyName)
	if companyName == "" {
		companyName = strings.TrimSpace(input.Vacancy.Title)
	}
	roleLabel := ResumeTargetRoleLabel(input.TargetRole)
	resumeSkills := NormalizeStringList(input.ResumeSkills)
	positiveReasons := dedupeAndTrim(input.PositiveReasons)
	projectExperience := strings.TrimSpace(input.ProjectExperience)
	missingSkills := dedupeAndTrim(input.MissingSkills)

	parts := make([]string, 0, 8)
	if candidateName != "" {
		parts = append(parts, fmt.Sprintf("Здравствуйте, %s.", candidateName))
	} else {
		parts = append(parts, "Здравствуйте.")
	}
	if companyName != "" {
		parts = append(parts, fmt.Sprintf("Меня заинтересовала вакансия %q в компании %s.", safeQuote(input.Vacancy.Title), companyName))
	} else {
		parts = append(parts, fmt.Sprintf("Меня заинтересовала вакансия %q.", safeQuote(input.Vacancy.Title)))
	}

	if input.CommercialExperienceYears > 0 {
		parts = append(parts, fmt.Sprintf("У меня подтверждено %d лет коммерческого опыта в направлении %s.", input.CommercialExperienceYears, roleLabel))
	} else if projectExperience != "" {
		parts = append(parts, fmt.Sprintf("Есть подтвержденный проектный опыт: %s.", projectExperience))
	} else {
		parts = append(parts, fmt.Sprintf("Работаю в направлении %s и готов обсудить релевантный опыт по задачам вакансии.", roleLabel))
	}

	if len(resumeSkills) > 0 {
		parts = append(parts, fmt.Sprintf("В резюме подтверждены навыки: %s.", joinWithLimit(resumeSkills, 5)))
	}

	if len(positiveReasons) > 0 {
		parts = append(parts, fmt.Sprintf("По совпадениям с вакансией отмечаю: %s.", joinWithLimit(positiveReasons, 3)))
	}

	if projectExperience != "" && input.CommercialExperienceYears > 0 {
		parts = append(parts, fmt.Sprintf("Отдельно могу показать проектные задачи: %s.", projectExperience))
	}

	if len(missingSkills) > 0 {
		parts = append(parts, "Если в команде есть дополнительные ожидания по стеку, готов обсудить их на созвоне.")
	}

	parts = append(parts, "Буду рад обсудить детали роли и вручную пройти следующий шаг отклика.")
	text := strings.Join(parts, "\n\n")

	if len(text) < 500 {
		text += "\n\nГотов аккуратно адаптировать это письмо под формат компании и уточнить детали по задачам."
	}
	if len(text) < 500 {
		text += "\n\nСпасибо за рассмотрение моего отклика."
	}
	if len([]rune(text)) > 1200 {
		text = string([]rune(text)[:1200])
		text = strings.TrimSpace(text)
	}
	return text, nil
}

func BuildApplicationWarnings(profile CandidateProfile, resume Resume, vacancy Vacancy, match *VacancyMatch, projectExperience string) []string {
	warnings := make([]string, 0, 6)
	if match != nil && len(match.MissingSkills) > 0 {
		warnings = append(warnings, fmt.Sprintf("Не все навыки из вакансии подтверждены: %s.", joinWithLimit(match.MissingSkills, 5)))
	}
	if strings.TrimSpace(projectExperience) != "" {
		warnings = append(warnings, "В письме использован проектный опыт, а не коммерческий.")
	}
	requiredYears := gradeRank(vacancy.Grade)
	candidateYears := profile.YearsOfCommercialExperience
	if requiredYears > 0 && experienceRank(candidateYears) < requiredYears {
		warnings = append(warnings, "Вакансия может требовать большего стажа, чем подтверждено в профиле.")
	}
	if strings.TrimSpace(vacancy.RemoteType) == "" && strings.TrimSpace(vacancy.Location) == "" && strings.TrimSpace(vacancy.EmploymentType) == "" {
		warnings = append(warnings, "Условия работы в вакансии описаны не полностью.")
	}
	if vacancy.SalaryFrom == nil && vacancy.SalaryTo == nil {
		warnings = append(warnings, "Вакансия не содержит зарплату.")
	}
	if len(profile.WorkAuthorization) == 0 || len(vacancy.WorkAuthorizationRequirements) == 0 {
		warnings = append(warnings, "Разрешение на работу не подтверждено отдельно.")
	}
	return dedupeAndTrim(warnings)
}

func ExtractProjectExperience(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, 2)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if containsAny(trimmed, "project", "проект", "pet project", "кейc", "case", "portfolio", "портфолио") {
			out = append(out, trimmed)
		}
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "; ")
}

func safeQuote(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.TrimSpace(strings.Trim(value, `"'`))
}

func joinWithLimit(values []string, limit int) string {
	values = dedupeAndTrim(values)
	if len(values) == 0 {
		return ""
	}
	if limit > 0 && len(values) > limit {
		values = values[:limit]
	}
	return strings.Join(values, ", ")
}
