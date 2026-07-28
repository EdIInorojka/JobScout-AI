package core

import "fmt"

func ParseSourceType(value string) (SourceType, error) {
	t := SourceType(value)
	if !t.Valid() {
		return "", fmt.Errorf("invalid source type: %s", value)
	}
	return t, nil
}

func ParseVacancyStatus(value string) (VacancyStatus, error) {
	s := VacancyStatus(value)
	if !s.Valid() {
		return "", fmt.Errorf("invalid vacancy status: %s", value)
	}
	return s, nil
}
