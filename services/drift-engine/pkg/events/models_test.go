package events_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/infraguard/drift-engine/pkg/events"
)

func TestDriftEvent_JSONRoundTrip(t *testing.T) {
	// Build a complete drift event
	event := events.DriftEvent{
		ID:           uuid.New(),
		Cloud:        "aws",
		Region:       "us-east-1",
		ResourceType: "aws_security_group",
		ResourceID:   "sg-abc123",
		ChangeType:   events.ChangeTypeIngressAdded,
		Actor:        "arn:aws:iam::000000000000:root",
		Severity:     events.SeverityCritical,
		DetectedAt:   time.Now().UTC(),
		PreviousState: events.SecurityGroupState{
			GroupID:      "sg-abc123",
			IngressRules: []events.SGRule{
				{Protocol: "tcp", FromPort: 443, ToPort: 443, CIDRs: []string{"0.0.0.0/0"}},
			},
		},
		NewState: events.SecurityGroupState{
			GroupID:      "sg-abc123",
			IngressRules: []events.SGRule{
				{Protocol: "tcp", FromPort: 443, ToPort: 443, CIDRs: []string{"0.0.0.0/0"}},
				{Protocol: "tcp", FromPort: 5432, ToPort: 5432, CIDRs: []string{"0.0.0.0/0"}},
			},
		},
		Violations: []events.Violation{
			{
				RuleID:      "CIS_5.4",
				Framework:   "CIS",
				Severity:    "CRITICAL",
				Description: "Database port exposed to internet",
			},
		},
	}

	// Serialise to JSON
	data, err := json.Marshal(event)
	require.NoError(t, err)
	assert.Contains(t, string(data), "INGRESS_RULE_ADDED")
	assert.Contains(t, string(data), "sg-abc123")
	assert.Contains(t, string(data), "CIS_5.4")

	// Deserialise back
	var decoded events.DriftEvent
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, event.ResourceID, decoded.ResourceID)
	assert.Equal(t, event.ChangeType, decoded.ChangeType)
	assert.Equal(t, event.Severity, decoded.Severity)
	assert.Len(t, decoded.Violations, 1)
}

func TestSeverityConstants(t *testing.T) {
	assert.Equal(t, events.Severity("CRITICAL"), events.SeverityCritical)
	assert.Equal(t, events.Severity("HIGH"),     events.SeverityHigh)
	assert.Equal(t, events.Severity("MEDIUM"),   events.SeverityMedium)
	assert.Equal(t, events.Severity("LOW"),      events.SeverityLow)
	assert.Equal(t, events.Severity("INFO"),     events.SeverityInfo)
}

func TestChangeTypeConstants(t *testing.T) {
	assert.Equal(t, events.ChangeType("INGRESS_RULE_ADDED"),   events.ChangeTypeIngressAdded)
	assert.Equal(t, events.ChangeType("ENCRYPTION_DISABLED"),  events.ChangeTypeEncryptionOff)
	assert.Equal(t, events.ChangeType("PUBLIC_ACCESS_ENABLED"),events.ChangeTypePublicAccess)
}

func TestSecurityGroupState_RuleCount(t *testing.T) {
	sg := events.SecurityGroupState{
		GroupID:   "sg-test",
		GroupName: "test-sg",
		IngressRules: []events.SGRule{
			{Protocol: "tcp", FromPort: 443, ToPort: 443, CIDRs: []string{"0.0.0.0/0"}},
			{Protocol: "tcp", FromPort: 80,  ToPort: 80,  CIDRs: []string{"0.0.0.0/0"}},
		},
	}
	assert.Len(t, sg.IngressRules, 2)
	assert.Equal(t, int32(443), sg.IngressRules[0].FromPort)
	assert.Equal(t, int32(80),  sg.IngressRules[1].FromPort)
}

func TestNATSSubjectConstants(t *testing.T) {
	assert.Equal(t, "infraguard.drift.detected",   events.SubjectDetected)
	assert.Equal(t, "infraguard.drift.classified",  events.SubjectClassified)
	assert.Equal(t, "infraguard.drift.remediated",  events.SubjectRemediated)
}
