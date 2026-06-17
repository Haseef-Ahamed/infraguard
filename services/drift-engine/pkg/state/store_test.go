package state_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/infraguard/drift-engine/pkg/events"
	"github.com/infraguard/drift-engine/pkg/state"
)

const testConnStr = "postgres://infraguard:infraguard_dev@localhost:5432/infraguard"

func newTestStore(t *testing.T) *state.Store {
	ctx := context.Background()
	store, err := state.NewStore(ctx, testConnStr)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	return store
}

func TestSaveDriftEvent_Succeeds(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()
	ctx := context.Background()

	event := &events.DriftEvent{
		ID:           uuid.New(),
		Cloud:        "aws",
		ResourceID:   "sg-test-" + uuid.New().String()[:8],
		ResourceType: "aws_security_group",
		ChangeType:   events.ChangeTypeIngressAdded,
		Severity:     events.SeverityCritical,
		PreviousState: events.SecurityGroupState{GroupID: "sg-test"},
		NewState:      events.SecurityGroupState{GroupID: "sg-test"},
		DetectedAt:   time.Now().UTC(),
	}

	err := store.SaveDriftEvent(ctx, event)
	require.NoError(t, err)

	// Idempotency: saving the same event ID again must not error
	err = store.SaveDriftEvent(ctx, event)
	assert.NoError(t, err)
}

func TestUpsertAndGetBaseline(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()
	ctx := context.Background()

	resourceID := "sg-baseline-test-" + uuid.New().String()[:8]
	sgState := events.SecurityGroupState{
		GroupID:   resourceID,
		GroupName: "test-sg",
		IngressRules: []events.SGRule{
			{Protocol: "tcp", FromPort: 443, ToPort: 443, CIDRs: []string{"0.0.0.0/0"}},
		},
	}

	err := store.UpsertBaseline(ctx, resourceID, sgState, "terraform")
	require.NoError(t, err)

	got, err := store.GetLatestBaseline(ctx, resourceID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, resourceID, got["group_id"])
}

func TestGetLatestBaseline_NotFound(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()
	ctx := context.Background()

	got, err := store.GetLatestBaseline(ctx, "does-not-exist-"+uuid.New().String())
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestCountOpenDriftEvents_ReturnsNonNegative(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()
	ctx := context.Background()

	count, err := store.CountOpenDriftEvents(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 0)
}
