package core

import (
	"strings"
	"time"
)

type FilterConfig struct {
	MaxVacancyAgeDays int
}

func ApplyHardFilters(profile CandidateProfile, vacancy Vacancy, company Company, now time.Time, cfg FilterConfig) HardFilterResult {
	reasons := make([]string, 0, 8)

	if company.Blacklisted {
		reasons = append(reasons, "company is blacklisted")
	}
	if len(profile.ExcludedCompanies) > 0 {
		companyName := NormalizeText(company.DisplayName)
		normalizedCompany := NormalizeText(company.NormalizedName)
		for _, excluded := range profile.ExcludedCompanies {
			excludedNormalized := NormalizeText(excluded)
			if excludedNormalized != "" && (strings.Contains(companyName, excludedNormalized) || strings.Contains(normalizedCompany, excludedNormalized)) {
				reasons = append(reasons, "company is in excluded list")
				break
			}
		}
	}
	if len(profile.DesiredRoles) > 0 {
		roleOK := false
		for _, role := range profile.DesiredRoles {
			roleNormalized := NormalizeText(role)
			if roleNormalized == "" {
				continue
			}
			if strings.Contains(vacancy.NormalizedTitle, roleNormalized) || strings.Contains(NormalizeText(vacancy.Title), roleNormalized) {
				roleOK = true
				break
			}
		}
		if !roleOK {
			reasons = append(reasons, "role does not match allowed roles")
		}
	}
	if !profile.RemoteAllowed {
		remote := strings.Contains(NormalizeText(vacancy.RemoteType), "remote") || strings.Contains(NormalizeText(vacancy.Location), "remote")
		if remote {
			reasons = append(reasons, "remote work is not allowed by profile")
		}
	}
	if len(profile.DesiredLocations) > 0 && !profile.RelocationAllowed {
		locationOK := false
		location := NormalizeLocation(vacancy.Location)
		for _, desired := range profile.DesiredLocations {
			if desiredNormalized := NormalizeLocation(desired); desiredNormalized != "" && strings.Contains(location, desiredNormalized) {
				locationOK = true
				break
			}
		}
		if !locationOK && vacancy.Location != "" {
			reasons = append(reasons, "location is outside the allowed set")
		}
	}
	if profile.MinimumSalary != nil && *profile.MinimumSalary > 0 {
		if vacancy.SalaryTo != nil && *vacancy.SalaryTo < *profile.MinimumSalary {
			reasons = append(reasons, "salary is below the minimum")
		}
	}
	if len(profile.WorkAuthorization) > 0 && len(vacancy.WorkAuthorizationRequirements) > 0 {
		required := map[string]struct{}{}
		for _, item := range vacancy.WorkAuthorizationRequirements {
			required[NormalizeSkill(item)] = struct{}{}
		}
		authOK := false
		for _, auth := range profile.WorkAuthorization {
			if _, ok := required[NormalizeSkill(auth)]; ok {
				authOK = true
				break
			}
		}
		if !authOK {
			reasons = append(reasons, "work authorization is not available")
		}
	}
	if len(profile.StopWords) > 0 {
		blob := strings.Join([]string{
			NormalizeText(vacancy.Title),
			NormalizeText(vacancy.Description),
			NormalizeText(vacancy.Requirements),
			NormalizeText(vacancy.Responsibilities),
			NormalizeText(company.DisplayName),
		}, " ")
		for _, stopWord := range profile.StopWords {
			stop := NormalizeText(stopWord)
			if stop != "" && strings.Contains(blob, stop) {
				reasons = append(reasons, "vacancy contains stop words")
				break
			}
		}
	}
	if cfg.MaxVacancyAgeDays > 0 && !vacancy.PublishedAt.IsZero() {
		if vacancy.PublishedAt.Before(now.Add(-time.Duration(cfg.MaxVacancyAgeDays) * 24 * time.Hour)) {
			reasons = append(reasons, "vacancy is too old")
		}
	}
	return HardFilterResult{
		Passed:  len(reasons) == 0,
		Reasons: dedupeAndTrim(reasons),
	}
}
