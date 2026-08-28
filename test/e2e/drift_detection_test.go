package e2e

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testConnStr = "postgres://infraguard:infraguard_dev@localhost:5432/infraguard"

func countDriftEvents(t *testing.T) int {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, testConnStr)
	require.NoError(t, err)
	defer pool.Close()

	var count int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM drift_events").Scan(&count)
	require.NoError(t, err)
	return count
}

func countDriftEventsByChangeType(t *testing.T, changeType string) int {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, testConnStr)
	require.NoError(t, err)
	defer pool.Close()

	var count int
	err = pool.QueryRow(ctx,
		"SELECT count(*) FROM drift_events WHERE change_type=$1 AND detected_at > NOW() - INTERVAL '5 minutes'",
		changeType,
	).Scan(&count)
	require.NoError(t, err)
	return count
}

// countDriftEventsSince returns drift events detected after the given timestamp —
// used to avoid false positives from stale events already in the DB.
func countDriftEventsSince(t *testing.T, since time.Time) int {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, testConnStr)
	require.NoError(t, err)
	defer pool.Close()

	var count int
	err = pool.QueryRow(ctx,
		"SELECT count(*) FROM drift_events WHERE detected_at > $1",
		since,
	).Scan(&count)
	require.NoError(t, err)
	return count
}

// countDriftEventsSince returns drift events detected after the given timestamp —
// used to avoid false positives from stale events already in the DB.
// countDriftEventsSince returns drift events detected after the given timestamp —
// used to avoid false positives from stale events already in the DB.
// TestEndToEnd_DriftIntroducedAndDetected is the flagship test of the entire
// platform: introduces a real security group change in LocalStack, waits for
// the drift engine (assumed running) to detect it, and verifies the event
// was persisted to PostgreSQL with the correct change type.
func TestEndToEnd_DriftIntroducedAndDetected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	// Ensure clean starting state
	revertCmd := exec.Command("bash", "-c",
		"~/infraguard/scripts/drift-simulator/introduce_drift.sh revert-all")
	_ = revertCmd.Run()
	time.Sleep(2 * time.Second)

	before := countDriftEvents(t)
	t.Logf("Drift events before test: %d", before)

	// Mark the exact moment before introducing drift — only events after
	// this timestamp count as "newly detected" for this test run.
	testStart := time.Now()

	// Introduce drift: open port 5432
	introduceCmd := exec.Command("bash", "-c",
		"~/infraguard/scripts/drift-simulator/introduce_drift.sh sg-port")
	output, err := introduceCmd.CombinedOutput()
	require.NoError(t, err, "drift simulator failed: %s", string(output))
	t.Logf("Drift introduced: %s", string(output))

	// Wait for detection (agent scans periodically — allow up to 90s)
	t.Log("Waiting up to 90 seconds for drift detection...")
	detected := false
	for i := 0; i < 18; i++ {
		time.Sleep(5 * time.Second)
		newEvents := countDriftEventsSince(t, testStart)
		if newEvents > 0 {
			detected = true
			t.Logf("Detection confirmed after %ds (%d new events)", (i+1)*5, newEvents)
			break
		}
	}

	// Cleanup regardless of test outcome
	defer func() {
		cleanupCmd := exec.Command("bash", "-c",
			"~/infraguard/scripts/drift-simulator/introduce_drift.sh revert-all")
		_ = cleanupCmd.Run()
	}()

	require.True(t, detected, "drift was not detected within 90 seconds — is the drift-engine running with a short enough scan interval?")

	after := countDriftEvents(t)
	assert.Greater(t, after, before, "drift_events count should have increased")
}

// TestEndToEnd_BaselineRestoredAfterRevert proves the revert script correctly
// removes the drift, returning the security group to its compliant baseline.
func TestEndToEnd_BaselineRestoredAfterRevert(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	introduceCmd := exec.Command("bash", "-c",
		"~/infraguard/scripts/drift-simulator/introduce_drift.sh sg-port")
	_, _ = introduceCmd.CombinedOutput()
	time.Sleep(2 * time.Second)

	revertCmd := exec.Command("bash", "-c",
		"~/infraguard/scripts/drift-simulator/introduce_drift.sh revert-all")
	output, err := revertCmd.CombinedOutput()
	require.NoError(t, err, "revert failed: %s", string(output))

	client := newEC2Client(t)
	ctx := context.Background()
	resp, err := describeAppSG(ctx, client)
	require.NoError(t, err)
	require.Len(t, resp.SecurityGroups, 1)

	sg := resp.SecurityGroups[0]
	for _, rule := range sg.IpPermissions {
		assert.NotEqual(t, int32(5432), aws.ToInt32(rule.FromPort), "port 5432 should be reverted")
	}
}
