package e2e

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompliance_Port5432TriggersAllThreeFrameworks proves that the exact
// drift scenario introduced by the simulator (opening port 5432) is
// correctly classified as a violation across CIS, HIPAA, and SOC2.
func TestCompliance_Port5432TriggersAllThreeFrameworks(t *testing.T) {
	input := map[string]interface{}{
		"resource_type": "aws_security_group",
		"resource_id":   "sg-e2e-test",
		"ingress_rules": []map[string]interface{}{
			{"from_port": 443, "to_port": 443, "cidrs": []string{"0.0.0.0/0"}},
			{"from_port": 5432, "to_port": 5432, "cidrs": []string{"0.0.0.0/0"}},
		},
	}
	inputJSON, err := json.Marshal(input)
	require.NoError(t, err)

	cmd := exec.Command("opa", "eval", "-I",
		"-d", "../../policies/rego/cis/network.rego",
		"-d", "../../policies/rego/hipaa/encryption.rego",
		"-d", "../../policies/rego/soc2/access.rego",
		"data.infraguard",
	)
	cmd.Stdin = bytes.NewReader(inputJSON)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "opa eval failed: %s", string(output))

	outputStr := string(output)
	assert.Contains(t, outputStr, "CIS_5.4", "CIS violation should fire for port 5432")
	assert.Contains(t, outputStr, "HIPAA_164.312e2ii", "HIPAA violation should fire for port 5432")
	assert.Contains(t, outputStr, "SOC2_CC6.1", "SOC2 violation should fire for port 5432")
}

// TestCompliance_Port443OnlyProducesNoViolations proves the clean baseline
// does not trigger any false-positive compliance violations.
func TestCompliance_Port443OnlyProducesNoViolations(t *testing.T) {
	input := map[string]interface{}{
		"resource_type": "aws_security_group",
		"resource_id":   "sg-e2e-clean",
		"ingress_rules": []map[string]interface{}{
			{"from_port": 443, "to_port": 443, "cidrs": []string{"0.0.0.0/0"}},
		},
	}
	inputJSON, err := json.Marshal(input)
	require.NoError(t, err)

	cmd := exec.Command("opa", "eval", "-I",
		"-d", "../../policies/rego/cis/network.rego",
		"data.infraguard.cis.network.deny",
	)
	cmd.Stdin = bytes.NewReader(inputJSON)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err)

	assert.Contains(t, string(output), `"value": []`, "clean baseline should produce zero violations")
}
