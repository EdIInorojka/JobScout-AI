package core

import (
	"fmt"
	"math"
	"strings"
)

func DefaultScoringConfig() ScoringConfig {
	return ScoringConfig{
		Weights: ScoringWeights{
			RoleWeight:       20,
			SkillsWeight:     30,
			ExperienceWeight: 10,
			GradeWeight:      10,
			LocationWeight:   10,
			SalaryWeight:     10,
			DomainWeight:     10,
		},
		ApplyThreshold:  80,
		ReviewThreshold: 55,
		Version:         "v1",
	}
}

func (cfg ScoringConfig) Validate() error {
	sum := cfg.Weights.RoleWeight + cfg.Weights.SkillsWeight + cfg.Weights.ExperienceWeight + cfg.Weights.GradeWeight + cfg.Weights.LocationWeight + cfg.Weights.SalaryWeight + cfg.Weights.DomainWeight
	if sum != 100 {
		return fmt.Errorf("scoring weights must sum to 100, got %d", sum)
	}
	if cfg.ApplyThreshold < cfg.ReviewThreshold {
		return fmt.Errorf("apply threshold must be >= review threshold")
	}
	return nil
}

func ScoreVacancy(profile CandidateProfile, vacancy Vacancy, company Company, cfg ScoringConfig, hardFilterPassed bool) ScoringResult {
	roleScore, roleReasons, roleNegatives := scoreRole(profile, vacancy)
	skillsScore, skillsReasons, missingSkills, _, skillsNegatives := scoreSkills(profile, vacancy)
	experienceScore, experienceReasons, experienceNegatives := scoreExperience(profile, vacancy)
	gradeScore, gradeReasons, gradeNegatives := scoreGrade(profile, vacancy)
	locationScore, locationReasons, locationNegatives := scoreLocation(profile, vacancy)
	salaryScore, salaryReasons, salaryNegatives, salaryWarnings := scoreSalary(profile, vacancy)
	domainScore, domainReasons, domainNegatives := scoreDomain(profile, vacancy, company)

	total := weightedAverage(cfg, roleScore, skillsScore, experienceScore, gradeScore, locationScore, salaryScore, domainScore)
	positives := append(append(append(append(append(append(roleReasons, skillsReasons...), experienceReasons...), gradeReasons...), locationReasons...), salaryReasons...), domainReasons...)
	negatives := append(append(append(append(append(append(roleNegatives, skillsNegatives...), experienceNegatives...), gradeNegatives...), locationNegatives...), salaryNegatives...), domainNegatives...)
	reco := RecommendationSkip
	switch {
	case !hardFilterPassed:
		reco = RecommendationSkip
	case total >= cfg.ApplyThreshold:
		reco = RecommendationApply
	case total >= cfg.ReviewThreshold:
		reco = RecommendationReview
	default:
		reco = RecommendationSkip
	}
	return ScoringResult{
		TotalScore:       total,
		RoleScore:        roleScore,
		SkillsScore:      skillsScore,
		ExperienceScore:  experienceScore,
		LocationScore:    locationScore,
		SalaryScore:      salaryScore,
		GradeScore:       gradeScore,
		DomainScore:      domainScore,
		PositiveReasons:  dedupeAndTrim(positives),
		NegativeReasons:  dedupeAndTrim(negatives),
		MissingSkills:    dedupeAndTrim(missingSkills),
		Warnings:         dedupeAndTrim(salaryWarnings),
		Recommendation:   reco,
		HardFilterPassed: hardFilterPassed,
		Version:          cfg.Version,
	}
}

func weightedAverage(cfg ScoringConfig, role, skills, experience, grade, location, salary, domain int) int {
	total := float64(role*cfg.Weights.RoleWeight + skills*cfg.Weights.SkillsWeight + experience*cfg.Weights.ExperienceWeight + grade*cfg.Weights.GradeWeight + location*cfg.Weights.LocationWeight + salary*cfg.Weights.SalaryWeight + domain*cfg.Weights.DomainWeight)
	return int(math.Round(total / 100))
}

func scoreRole(profile CandidateProfile, vacancy Vacancy) (int, []string, []string) {
	desired := NormalizeStringList(profile.DesiredRoles)
	if len(desired) == 0 {
		return 100, []string{"profile has no role restriction"}, nil
	}
	joined := vacancy.NormalizedTitle + " " + NormalizeText(vacancy.Title)
	for _, role := range desired {
		if role == vacancy.NormalizedTitle || strings.Contains(joined, role) {
			return 100, []string{fmt.Sprintf("role matches %s", role)}, nil
		}
	}
	return 0, nil, []string{"role does not match desired roles"}
}

func scoreSkills(profile CandidateProfile, vacancy Vacancy) (int, []string, []string, []string, []string) {
	required := map[string]int{}
	for _, skill := range profile.PrimarySkills {
		s := NormalizeSkill(skill)
		if s != "" {
			required[s] += 2
		}
	}
	for _, skill := range profile.SecondarySkills {
		s := NormalizeSkill(skill)
		if s != "" {
			required[s] += 1
		}
	}
	if len(required) == 0 {
		return 100, []string{"profile has no skill restriction"}, nil, nil, nil
	}
	excluded := map[string]struct{}{}
	for _, skill := range profile.ExcludedSkills {
		excluded[NormalizeSkill(skill)] = struct{}{}
	}
	vacancySkills := map[string]struct{}{}
	for _, skill := range vacancy.Skills {
		vacancySkills[NormalizeSkill(skill)] = struct{}{}
	}
	totalWeight := 0
	matchedWeight := 0
	missing := make([]string, 0)
	positives := make([]string, 0)
	for skill, weight := range required {
		if _, bad := excluded[skill]; bad {
			continue
		}
		totalWeight += weight
		if _, ok := vacancySkills[skill]; ok {
			matchedWeight += weight
			positives = append(positives, fmt.Sprintf("skill match: %s", skill))
		} else {
			missing = append(missing, skill)
		}
	}
	if totalWeight == 0 {
		return 100, positives, missing, nil, nil
	}
	score := int(math.Round(float64(matchedWeight) * 100 / float64(totalWeight)))
	var negatives []string
	if len(missing) > 0 {
		negatives = append(negatives, fmt.Sprintf("missing skills: %s", strings.Join(missing, ", ")))
	}
	return score, positives, missing, nil, negatives
}

func scoreExperience(profile CandidateProfile, vacancy Vacancy) (int, []string, []string) {
	required := gradeRank(vacancy.Grade)
	candidate := experienceRank(profile.YearsOfCommercialExperience)
	if required == 0 {
		return 70, []string{"vacancy grade is not explicit"}, nil
	}
	if candidate >= required {
		return 100, []string{fmt.Sprintf("experience fits grade %s", vacancy.Grade)}, nil
	}
	if candidate+1 == required {
		return 60, nil, []string{fmt.Sprintf("experience is slightly below grade %s", vacancy.Grade)}
	}
	return 0, nil, []string{fmt.Sprintf("experience is too low for grade %s", vacancy.Grade)}
}

func scoreGrade(profile CandidateProfile, vacancy Vacancy) (int, []string, []string) {
	desired := NormalizeStringList(profile.DesiredGrades)
	if len(desired) == 0 {
		return 100, []string{"profile has no grade restriction"}, nil
	}
	vacancyGrade := NormalizeSkill(vacancy.Grade)
	for _, grade := range desired {
		if grade == vacancyGrade || strings.Contains(vacancyGrade, grade) {
			return 100, []string{fmt.Sprintf("grade matches %s", grade)}, nil
		}
	}
	return 30, nil, []string{"grade is outside the preferred range"}
}

func scoreLocation(profile CandidateProfile, vacancy Vacancy) (int, []string, []string) {
	remote := strings.Contains(NormalizeText(vacancy.RemoteType), "remote") || strings.Contains(NormalizeText(vacancy.Location), "remote")
	if remote && profile.RemoteAllowed {
		return 100, []string{"remote format matches"}, nil
	}
	if len(profile.DesiredLocations) == 0 {
		if remote && !profile.RemoteAllowed {
			return 0, nil, []string{"remote work is not allowed"}
		}
		return 80, []string{"location is not restricted"}, nil
	}
	location := NormalizeLocation(vacancy.Location)
	for _, desired := range profile.DesiredLocations {
		if location != "" && strings.Contains(location, NormalizeLocation(desired)) {
			return 100, []string{fmt.Sprintf("location matches %s", desired)}, nil
		}
	}
	if profile.RelocationAllowed {
		return 60, []string{"relocation is allowed by profile"}, nil
	}
	return 0, nil, []string{"location does not match desired cities or regions"}
}

func scoreSalary(profile CandidateProfile, vacancy Vacancy) (int, []string, []string, []string) {
	if profile.MinimumSalary == nil || *profile.MinimumSalary <= 0 {
		return 100, []string{"salary not constrained by profile"}, nil, nil
	}
	if vacancy.SalaryFrom == nil && vacancy.SalaryTo == nil {
		return 50, nil, nil, []string{"salary is not specified"}
	}
	if vacancy.Currency != "" && len(profile.Currencies) > 0 {
		matchedCurrency := false
		for _, currency := range profile.Currencies {
			if strings.EqualFold(strings.TrimSpace(currency), strings.TrimSpace(vacancy.Currency)) {
				matchedCurrency = true
				break
			}
		}
		if !matchedCurrency {
			return 20, nil, []string{"salary currency does not match profile"}, nil
		}
	}
	min := *profile.MinimumSalary
	if vacancy.SalaryFrom != nil && *vacancy.SalaryFrom >= min {
		return 100, []string{fmt.Sprintf("salary from %d meets minimum %d", *vacancy.SalaryFrom, min)}, nil, nil
	}
	if vacancy.SalaryTo != nil && *vacancy.SalaryTo < min {
		return 0, nil, []string{fmt.Sprintf("salary is below minimum %d", min)}, nil
	}
	if vacancy.SalaryFrom != nil && vacancy.SalaryTo != nil && *vacancy.SalaryFrom < min && *vacancy.SalaryTo >= min {
		return 70, []string{"salary range overlaps minimum"}, nil, nil
	}
	return 40, nil, nil, nil
}

func scoreDomain(profile CandidateProfile, vacancy Vacancy, company Company) (int, []string, []string) {
	if len(profile.EmploymentTypes) == 0 && len(profile.Languages) == 0 && len(profile.WorkAuthorization) == 0 {
		return 100, []string{"no extra domain constraints"}, nil
	}
	score := 100
	var positives []string
	var negatives []string
	if len(profile.EmploymentTypes) > 0 {
		employment := NormalizeSkill(vacancy.EmploymentType)
		ok := false
		for _, desired := range profile.EmploymentTypes {
			if NormalizeSkill(desired) == employment {
				ok = true
				positives = append(positives, fmt.Sprintf("employment type matches %s", desired))
				break
			}
		}
		if !ok {
			score -= 30
			negatives = append(negatives, "employment type differs from profile")
		}
	}
	if company.Blacklisted {
		score = 0
		negatives = append(negatives, "company is blacklisted")
	}
	if len(vacancy.LanguageRequirements) > 0 && len(profile.Languages) > 0 {
		required := map[string]struct{}{}
		for _, lang := range vacancy.LanguageRequirements {
			required[NormalizeSkill(lang)] = struct{}{}
		}
		ok := false
		for _, lang := range profile.Languages {
			if _, exists := required[NormalizeSkill(lang)]; exists {
				ok = true
				break
			}
		}
		if !ok {
			score -= 20
			negatives = append(negatives, "language requirements are not satisfied")
		}
	}
	if len(profile.WorkAuthorization) > 0 && len(vacancy.WorkAuthorizationRequirements) > 0 {
		required := map[string]struct{}{}
		for _, item := range vacancy.WorkAuthorizationRequirements {
			required[NormalizeSkill(item)] = struct{}{}
		}
		ok := false
		for _, auth := range profile.WorkAuthorization {
			if _, exists := required[NormalizeSkill(auth)]; exists {
				ok = true
				break
			}
		}
		if !ok {
			score -= 30
			negatives = append(negatives, "work authorization requirement is not satisfied")
		}
	}
	if score < 0 {
		score = 0
	}
	if len(positives) == 0 && len(negatives) == 0 {
		positives = append(positives, "domain constraints are neutral")
	}
	return score, positives, negatives
}

func experienceRank(years int) int {
	switch {
	case years <= 1:
		return 1
	case years <= 4:
		return 2
	case years <= 7:
		return 3
	default:
		return 4
	}
}

func gradeRank(grade string) int {
	grade = NormalizeText(grade)
	switch {
	case strings.Contains(grade, "trainee"), strings.Contains(grade, "intern"), strings.Contains(grade, "junior"), strings.Contains(grade, "jun"):
		return 1
	case strings.Contains(grade, "middle"), strings.Contains(grade, "mid"):
		return 2
	case strings.Contains(grade, "senior"), strings.Contains(grade, "sr"):
		return 3
	case strings.Contains(grade, "lead"), strings.Contains(grade, "principal"), strings.Contains(grade, "staff"):
		return 4
	default:
		return 0
	}
}

func dedupeAndTrim(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
