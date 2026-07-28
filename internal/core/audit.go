package core

import "time"

type AuditAction string

const (
	AuditActionApplicationPrepared        AuditAction = "APPLICATION_PREPARED"
	AuditActionApplicationApproved        AuditAction = "APPLICATION_APPROVED"
	AuditActionApplicationCancelled       AuditAction = "APPLICATION_CANCELLED"
	AuditActionApplicationMarkedSubmitted AuditAction = "APPLICATION_MARKED_SUBMITTED"
	AuditActionApplicationHRContact       AuditAction = "APPLICATION_HR_CONTACT"
	AuditActionApplicationInterview       AuditAction = "APPLICATION_INTERVIEW"
	AuditActionApplicationOffer           AuditAction = "APPLICATION_OFFER"
	AuditActionApplicationRejected        AuditAction = "APPLICATION_REJECTED"
)

type AuditEvent struct {
	ID         string         `json:"id"`
	Actor      string         `json:"actor"`
	Action     AuditAction    `json:"action"`
	EntityType string         `json:"entityType"`
	EntityID   string         `json:"entityId"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  time.Time      `json:"createdAt"`
}

const AuditEntityTypeApplication = "APPLICATION"
