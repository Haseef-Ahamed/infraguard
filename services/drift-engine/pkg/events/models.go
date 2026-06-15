package events

import (
	"time"

	"github.com/google/uuid"
)

// Severity levels for drift events
type Severity string

const (
	SeverityCritical Severity = "CRITICAL"
	SeverityHigh     Severity = "HIGH"
	SeverityMedium   Severity = "MEDIUM"
	SeverityLow      Severity = "LOW"
	SeverityInfo     Severity = "INFO"
)

// ChangeType describes what kind of drift occurred
type ChangeType string

const (
	ChangeTypeIngressAdded   ChangeType = "INGRESS_RULE_ADDED"
	ChangeTypeIngressRemoved ChangeType = "INGRESS_RULE_REMOVED"
	ChangeTypeEgressAdded    ChangeType = "EGRESS_RULE_ADDED"
	ChangeTypeEncryptionOff  ChangeType = "ENCRYPTION_DISABLED"
	ChangeTypePublicAccess   ChangeType = "PUBLIC_ACCESS_ENABLED"
	ChangeTypeIAMChange      ChangeType = "IAM_POLICY_CHANGED"
	ChangeTypeTagMissing     ChangeType = "REQUIRED_TAG_MISSING"
	ChangeTypeGeneric        ChangeType = "STATE_MISMATCH"
)

// NATS subject constants — all services use these
const (
	SubjectDetected   = "infraguard.drift.detected"
	SubjectClassified = "infraguard.drift.classified"
	SubjectRemediated = "infraguard.drift.remediated"
)

// DriftEvent is the canonical message published to NATS
// and stored in the PostgreSQL drift_events table
type DriftEvent struct {
	ID            uuid.UUID  `json:"id"`
	CorrelationID string     `json:"correlation_id"`
	Cloud         string     `json:"cloud"`         // aws | azure | gcp
	Region        string     `json:"region"`
	ResourceType  string     `json:"resource_type"` // aws_security_group
	ResourceID    string     `json:"resource_id"`   // sg-abc123
	ChangeType    ChangeType `json:"change_type"`
	Actor         string     `json:"actor"`         // arn:aws:iam::000:user/john
	ActorDisplay  string     `json:"actor_display"`
	PreviousState interface{} `json:"previous_state"`
	NewState      interface{} `json:"new_state"`
	Severity      Severity   `json:"severity,omitempty"`
	Violations    []Violation `json:"violations,omitempty"`
	DetectedAt    time.Time  `json:"detected_at"`
}

// Violation holds a single compliance rule breach
type Violation struct {
	RuleID      string `json:"rule_id"`      // CIS_5.4
	Framework   string `json:"framework"`    // CIS | HIPAA | SOC2
	Control     string `json:"control"`      // CC6.1
	Description string `json:"description"`
	Severity    string `json:"severity"`
	Remediation string `json:"remediation"`
}

// SecurityGroupState is the canonical representation
// of an AWS Security Group — used for comparison
type SecurityGroupState struct {
	GroupID      string            `json:"group_id"`
	GroupName    string            `json:"group_name"`
	VPCID        string            `json:"vpc_id"`
	Description  string            `json:"description"`
	IngressRules []SGRule          `json:"ingress_rules"`
	EgressRules  []SGRule          `json:"egress_rules"`
	Tags         map[string]string `json:"tags"`
}

// SGRule represents one inbound or outbound rule
type SGRule struct {
	Protocol  string   `json:"protocol"`
	FromPort  int32    `json:"from_port"`
	ToPort    int32    `json:"to_port"`
	CIDRs     []string `json:"cidrs"`
	IPv6CIDRs []string `json:"ipv6_cidrs,omitempty"`
}

// S3BucketState is the canonical state of an S3 bucket
type S3BucketState struct {
	BucketName           string `json:"bucket_name"`
	BlockPublicAcls      bool   `json:"block_public_acls"`
	BlockPublicPolicy    bool   `json:"block_public_policy"`
	IgnorePublicAcls     bool   `json:"ignore_public_acls"`
	RestrictPublicBuckets bool  `json:"restrict_public_buckets"`
	VersioningEnabled    bool   `json:"versioning_enabled"`
	EncryptionEnabled    bool   `json:"encryption_enabled"`
}
