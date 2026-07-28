package core

import (
	"crypto/sha256"
	"encoding/hex"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

var htmlTagRe = regexp.MustCompile(`(?s)<[^>]*>`)

var skillAliases = map[string]string{
	"golang":         "go",
	"go lang":        "go",
	"js":             "javascript",
	"ts":             "typescript",
	"node":           "nodejs",
	"node.js":        "nodejs",
	"postgres":       "postgresql",
	"postgresql":     "postgresql",
	"py":             "python",
	"k8s":            "kubernetes",
	"docker compose": "docker-compose",
}

func NormalizeText(value string) string {
	value = html.UnescapeString(value)
	value = htmlTagRe.ReplaceAllString(value, " ")
	value = strings.ToLower(value)
	value = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			return r
		case r == '+', r == '#', r == '.', r == '/', r == '-', r == '_':
			return r
		default:
			return ' '
		}
	}, value)
	fields := strings.Fields(value)
	return strings.Join(fields, " ")
}

func NormalizeSkill(value string) string {
	value = NormalizeText(value)
	if alias, ok := skillAliases[value]; ok {
		return alias
	}
	return value
}

func NormalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = NormalizeSkill(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func NormalizeLocation(value string) string {
	return NormalizeText(value)
}

func CanonicalURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return strings.TrimSuffix(raw, "/")
	}
	parsed.Fragment = ""
	parsed.RawQuery = ""
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	return parsed.String()
}

func StripHTML(value string) string {
	value = html.UnescapeString(value)
	value = htmlTagRe.ReplaceAllString(value, " ")
	value = strings.Join(strings.Fields(value), " ")
	return strings.TrimSpace(value)
}

func HashVacancy(raw RawVacancy) string {
	h := sha256.New()
	salaryFrom := ""
	if raw.SalaryFrom != nil {
		salaryFrom = strconv.Itoa(*raw.SalaryFrom)
	}
	salaryTo := ""
	if raw.SalaryTo != nil {
		salaryTo = strconv.Itoa(*raw.SalaryTo)
	}
	parts := []string{
		NormalizeText(raw.Title),
		NormalizeText(raw.CompanyName),
		NormalizeText(raw.Location),
		NormalizeText(raw.CanonicalURL),
		NormalizeText(raw.RemoteType),
		NormalizeText(raw.EmploymentType),
		NormalizeText(raw.Grade),
		salaryFrom,
		salaryTo,
		NormalizeText(raw.Currency),
		NormalizeText(raw.Description),
		NormalizeText(raw.Requirements),
		NormalizeText(raw.Responsibilities),
		strings.Join(NormalizeStringList(raw.Skills), ","),
		strings.Join(NormalizeStringList(raw.LanguageRequirements), ","),
		strings.Join(NormalizeStringList(raw.WorkAuthorizationRequirements), ","),
	}
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func NormalizeVacancy(raw RawVacancy) NormalizedVacancy {
	description := StripHTML(raw.Description)
	requirements := StripHTML(raw.Requirements)
	responsibilities := StripHTML(raw.Responsibilities)
	hashInput := RawVacancy{
		Title:                         raw.Title,
		CompanyName:                   raw.CompanyName,
		Description:                   description,
		Requirements:                  requirements,
		Responsibilities:              responsibilities,
		Location:                      raw.Location,
		RemoteType:                    raw.RemoteType,
		EmploymentType:                raw.EmploymentType,
		Grade:                         raw.Grade,
		SalaryFrom:                    raw.SalaryFrom,
		SalaryTo:                      raw.SalaryTo,
		Currency:                      raw.Currency,
		Skills:                        raw.Skills,
		LanguageRequirements:          raw.LanguageRequirements,
		WorkAuthorizationRequirements: raw.WorkAuthorizationRequirements,
	}
	return NormalizedVacancy{
		RawVacancy:               raw,
		NormalizedTitle:          NormalizeText(raw.Title),
		NormalizedCompanyName:    NormalizeText(raw.CompanyName),
		ContentHash:              HashVacancy(hashInput),
		StrippedDescription:      description,
		StrippedRequirements:     requirements,
		StrippedResponsibilities: responsibilities,
	}
}
