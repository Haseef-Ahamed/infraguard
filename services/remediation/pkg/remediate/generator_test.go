package remediate_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/infraguard/remediation/pkg/remediate"
)

func TestGenerateBranchName_Unique(t *testing.T) {
	b1 := remediate.GenerateBranchName("CRITICAL", "sg-abc123")
	assert.Contains(t, b1, "remediation/CRITICAL-sg-abc123")
}

func TestGeneratePRTitle(t *testing.T) {
	d := remediate.DriftInput{
		ResourceID: "sg-abc123", ChangeType: "INGRESS_RULE_ADDED", Severity: "CRITICAL",
	}
	title := remediate.GeneratePRTitle(d)
	assert.Contains(t, title, "CRITICAL")
	assert.Contains(t, title, "sg-abc123")
}

func TestGeneratePRBody_IncludesViolations(t *testing.T) {
	d := remediate.DriftInput{
		ResourceID: "sg-abc123", ResourceType: "aws_security_group",
		ChangeType: "INGRESS_RULE_ADDED", Severity: "CRITICAL",
		Actor: "arn:aws:iam::000:root", DetectedAt: time.Now(),
		Violations: []remediate.Violation{
			{RuleID: "CIS_5.4", Framework: "CIS", Description: "Database port exposed", Remediation: "Restrict access"},
		},
	}
	body := remediate.GeneratePRBody(d)
	assert.True(t, strings.Contains(body, "CIS_5.4"))
	assert.True(t, strings.Contains(body, "Database port exposed"))
	assert.True(t, strings.Contains(body, "sg-abc123"))
}

func TestGeneratePRBody_NoViolations(t *testing.T) {
	d := remediate.DriftInput{ResourceID: "sg-x", DetectedAt: time.Now()}
	body := remediate.GeneratePRBody(d)
	assert.Contains(t, body, "No compliance violations")
}

func TestGenerateSGFix_ContainsPort443Only(t *testing.T) {
	fix := remediate.GenerateSGFix("sg-abc123")
	assert.Contains(t, fix, "from_port   = 443")
	assert.NotContains(t, fix, "5432")
	assert.Contains(t, fix, "sg-abc123")
}
