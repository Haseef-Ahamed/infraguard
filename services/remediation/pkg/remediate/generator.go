package remediate

import (
	"fmt"
	"time"
)

// DriftInput is the minimal data needed to generate a remediation PR
type DriftInput struct {
	ResourceID   string
	ResourceType string
	ChangeType   string
	Severity     string
	Actor        string
	Violations   []Violation
	DetectedAt   time.Time
}

type Violation struct {
	RuleID      string
	Framework   string
	Description string
	Remediation string
}

// GenerateBranchName produces a unique branch name for this remediation
func GenerateBranchName(severity, resourceID string) string {
	ts := time.Now().Unix()
	return fmt.Sprintf("remediation/%s-%s-%d", severity, resourceID, ts)
}

// GeneratePRTitle produces the PR title
func GeneratePRTitle(d DriftInput) string {
	return fmt.Sprintf("[%s] Remediate drift on %s (%s)", d.Severity, d.ResourceID, d.ChangeType)
}

// GeneratePRBody produces the full markdown PR description
func GeneratePRBody(d DriftInput) string {
	body := fmt.Sprintf(`## InfraGuard Auto-Remediation

**Resource:** %s (%s)
**Change Type:** %s
**Severity:** %s
**Actor:** %s
**Detected At:** %s

### Compliance Violations
`, d.ResourceID, d.ResourceType, d.ChangeType, d.Severity, d.Actor, d.DetectedAt.Format(time.RFC3339))

	if len(d.Violations) == 0 {
		body += "\n_No compliance violations recorded for this event._\n"
	}
	for _, v := range d.Violations {
		body += fmt.Sprintf("\n- **%s** (%s): %s\n  - Fix: %s\n", v.RuleID, v.Framework, v.Description, v.Remediation)
	}

	body += `
---
This PR was opened automatically by InfraGuard's remediation engine.
Review the diff below and merge to apply the fix, or close this PR to reject.
`
	return body
}

// GenerateSGFix produces the corrected Terraform HCL for a security group
// that reverts an unauthorized ingress rule addition.
func GenerateSGFix(resourceID string) string {
	return fmt.Sprintf(`resource "aws_security_group" "app_sg" {
  name        = "infraguard-app-sg"
  description = "Application SG — HTTPS only. Managed by OpenTofu."
  vpc_id      = var.vpc_id

  # Reconciled by InfraGuard — unauthorized rule removed
  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name      = "infraguard-app-sg"
    ManagedBy = "opentofu"
  }
}
# Reconciled resource: %s
`, resourceID)
}
