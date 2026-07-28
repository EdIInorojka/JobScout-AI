package core

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type ApplicationMethod string

const (
	ApplicationMethodManualLink  ApplicationMethod = "MANUAL_LINK"
	ApplicationMethodOfficialAPI ApplicationMethod = "OFFICIAL_API"
)

func (m ApplicationMethod) Valid() bool {
	switch m {
	case ApplicationMethodManualLink, ApplicationMethodOfficialAPI:
		return true
	default:
		return false
	}
}

func ParseApplicationMethod(value string) (ApplicationMethod, error) {
	method := ApplicationMethod(value)
	if !method.Valid() {
		return "", fmt.Errorf("invalid application method: %s", value)
	}
	return method, nil
}

type ApplicationStatus string

const (
	ApplicationStatusDraft                ApplicationStatus = "DRAFT"
	ApplicationStatusWaitingApproval      ApplicationStatus = "WAITING_APPROVAL"
	ApplicationStatusApproved             ApplicationStatus = "APPROVED"
	ApplicationStatusManualActionRequired ApplicationStatus = "MANUAL_ACTION_REQUIRED"
	ApplicationStatusSubmitted            ApplicationStatus = "SUBMITTED"
	ApplicationStatusCancelled            ApplicationStatus = "CANCELLED"
	ApplicationStatusHRContact            ApplicationStatus = "HR_CONTACT"
	ApplicationStatusInterview            ApplicationStatus = "INTERVIEW"
	ApplicationStatusOffer                ApplicationStatus = "OFFER"
	ApplicationStatusRejected             ApplicationStatus = "REJECTED"
)

func (s ApplicationStatus) Valid() bool {
	switch s {
	case ApplicationStatusDraft, ApplicationStatusWaitingApproval, ApplicationStatusApproved, ApplicationStatusManualActionRequired, ApplicationStatusSubmitted, ApplicationStatusCancelled, ApplicationStatusHRContact, ApplicationStatusInterview, ApplicationStatusOffer, ApplicationStatusRejected:
		return true
	default:
		return false
	}
}

func ParseApplicationStatus(value string) (ApplicationStatus, error) {
	status := ApplicationStatus(value)
	if !status.Valid() {
		return "", fmt.Errorf("invalid application status: %s", value)
	}
	return status, nil
}

var applicationTransitions = map[ApplicationStatus]map[ApplicationStatus]struct{}{
	ApplicationStatusDraft: {
		ApplicationStatusWaitingApproval: {},
		ApplicationStatusCancelled:       {},
	},
	ApplicationStatusWaitingApproval: {
		ApplicationStatusApproved:  {},
		ApplicationStatusCancelled: {},
	},
	ApplicationStatusApproved: {
		ApplicationStatusManualActionRequired: {},
		ApplicationStatusCancelled:            {},
	},
	ApplicationStatusManualActionRequired: {
		ApplicationStatusSubmitted: {},
		ApplicationStatusCancelled: {},
	},
	ApplicationStatusSubmitted: {
		ApplicationStatusHRContact: {},
		ApplicationStatusInterview: {},
		ApplicationStatusRejected:  {},
	},
	ApplicationStatusHRContact: {
		ApplicationStatusInterview: {},
		ApplicationStatusRejected:  {},
	},
	ApplicationStatusInterview: {
		ApplicationStatusOffer:    {},
		ApplicationStatusRejected: {},
	},
	ApplicationStatusOffer:     {},
	ApplicationStatusRejected:  {},
	ApplicationStatusCancelled: {},
}

func CanTransitionApplicationStatus(from, to ApplicationStatus) bool {
	if from == to {
		return true
	}
	next, ok := applicationTransitions[from]
	if !ok {
		return false
	}
	_, ok = next[to]
	return ok
}

func (s ApplicationStatus) IsTerminal() bool {
	switch s {
	case ApplicationStatusCancelled, ApplicationStatusRejected, ApplicationStatusOffer:
		return true
	default:
		return false
	}
}

func (s ApplicationStatus) IsActive() bool {
	switch s {
	case ApplicationStatusCancelled, ApplicationStatusRejected:
		return false
	default:
		return true
	}
}

type Application struct {
	ID                 string            `json:"id"`
	VacancyID          string            `json:"vacancyId"`
	CandidateProfileID string            `json:"candidateProfileId"`
	ResumeID           string            `json:"resumeId"`
	Status             ApplicationStatus `json:"status"`
	ApplicationMethod  ApplicationMethod `json:"applicationMethod"`
	CoverLetter        string            `json:"coverLetter"`
	PreparedAt         time.Time         `json:"preparedAt"`
	ApprovedAt         *time.Time        `json:"approvedAt,omitempty"`
	SubmittedAt        *time.Time        `json:"submittedAt,omitempty"`
	ResponseReceivedAt *time.Time        `json:"responseReceivedAt,omitempty"`
	RejectionReason    string            `json:"rejectionReason,omitempty"`
	Notes              string            `json:"notes,omitempty"`
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
}

func (a Application) Validate() error {
	switch {
	case strings.TrimSpace(a.VacancyID) == "":
		return errors.New("vacancy id is required")
	case strings.TrimSpace(a.CandidateProfileID) == "":
		return errors.New("candidate profile id is required")
	case strings.TrimSpace(a.ResumeID) == "":
		return errors.New("resume id is required")
	case !a.Status.Valid():
		return fmt.Errorf("invalid application status: %s", a.Status)
	case !a.ApplicationMethod.Valid():
		return fmt.Errorf("invalid application method: %s", a.ApplicationMethod)
	case strings.TrimSpace(a.CoverLetter) == "":
		return errors.New("cover letter is required")
	case a.PreparedAt.IsZero():
		return errors.New("prepared at is required")
	}
	return nil
}

func (a Application) IsActive() bool {
	return a.Status.IsActive()
}
