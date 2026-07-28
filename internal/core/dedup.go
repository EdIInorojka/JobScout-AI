package core

import "fmt"

type DedupReason string

const (
	DedupReasonSourceExternalID DedupReason = "SOURCE_EXTERNAL_ID"
	DedupReasonContentHash      DedupReason = "CONTENT_HASH"
)

type DedupDecision struct {
	IsDuplicate      bool   `json:"isDuplicate"`
	PrimaryVacancyID string `json:"primaryVacancyId,omitempty"`
	Reason           string `json:"reason,omitempty"`
	Explanation      string `json:"explanation,omitempty"`
}

func ExplainDedup(reason DedupReason, primary *Vacancy, sourceType SourceType) DedupDecision {
	if primary == nil {
		return DedupDecision{}
	}
	switch reason {
	case DedupReasonSourceExternalID:
		return DedupDecision{
			IsDuplicate:      false,
			PrimaryVacancyID: primary.ID,
			Reason:           string(reason),
			Explanation:      fmt.Sprintf("vacancy already exists for source %s and external id", sourceType),
		}
	case DedupReasonContentHash:
		return DedupDecision{
			IsDuplicate:      true,
			PrimaryVacancyID: primary.ID,
			Reason:           string(reason),
			Explanation:      fmt.Sprintf("vacancy content matches primary vacancy %s", primary.ID),
		}
	default:
		return DedupDecision{}
	}
}
