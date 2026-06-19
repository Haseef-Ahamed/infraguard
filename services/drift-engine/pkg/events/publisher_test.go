package events_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/infraguard/drift-engine/pkg/events"
)

func TestNewPublisher_ConnectionRefused(t *testing.T) {
	// Port 9999 is not running NATS — should fail gracefully
	_, err := events.NewPublisher("nats://localhost:9999")
	assert.Error(t, err, "connecting to non-existent NATS should return an error")
}

func TestPublisher_RealNATS(t *testing.T) {
	// Connect to the real NATS running in Docker
	pub, err := events.NewPublisher("nats://localhost:4222")
	if err != nil {
		t.Skipf("NATS not available: %v", err)
	}
	defer pub.Close()

	assert.True(t, pub.IsConnected(), "publisher should be connected")

	// Subscribe to receive the event we are about to publish
	nc, err := nats.Connect("nats://localhost:4222")
	require.NoError(t, err)
	defer nc.Close()

	received := make(chan []byte, 1)
	_, err = nc.Subscribe(events.SubjectDetected, func(msg *nats.Msg) {
		received <- msg.Data
	})
	require.NoError(t, err)

	// Build and publish a test drift event
	event := &events.DriftEvent{
		ID:           uuid.New(),
		Cloud:        "aws",
		ResourceType: "aws_security_group",
		ResourceID:   "sg-test-publish",
		ChangeType:   events.ChangeTypeIngressAdded,
		Severity:     events.SeverityCritical,
		DetectedAt:   time.Now().UTC(),
	}

	err = pub.Publish(event)
	require.NoError(t, err)

	// Wait for the message to arrive
	select {
	case data := <-received:
		var decoded events.DriftEvent
		err := json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, event.ResourceID, decoded.ResourceID)
		assert.Equal(t, event.ChangeType, decoded.ChangeType)
		assert.Equal(t, events.SeverityCritical, decoded.Severity)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for NATS message")
	}
}

func TestPublisher_Close_Safe(t *testing.T) {
	pub, err := events.NewPublisher("nats://localhost:4222")
	if err != nil {
		t.Skipf("NATS not available: %v", err)
	}
	// Closing twice should not panic
	pub.Close()
	pub.Close()
}
