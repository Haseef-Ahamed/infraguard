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
	// With RetryOnFailedConnect the client connects lazily
	// so creation succeeds — only publish fails
	pub, err := events.NewPublisher("nats://localhost:9999")
	if err != nil {
		// Either behaviour is acceptable
		t.Logf("NewPublisher returned error (acceptable): %v", err)
		return
	}
	defer pub.Close()
	// Connection to port 9999 should not be established
	assert.False(t, pub.IsConnected(), "should not be connected to non-existent NATS")
}

func TestPublisher_RealNATS(t *testing.T) {
	pub, err := events.NewPublisher("nats://localhost:4222")
	if err != nil {
		t.Skipf("NATS not available: %v", err)
	}
	defer pub.Close()

	// Give connection a moment to establish
	time.Sleep(200 * time.Millisecond)

	if !pub.IsConnected() {
		t.Skip("NATS not connected — skipping")
	}

	// Subscribe to receive the event
	nc, err := nats.Connect("nats://localhost:4222")
	require.NoError(t, err)
	defer nc.Close()

	received := make(chan []byte, 1)
	_, err = nc.Subscribe(events.SubjectDetected, func(msg *nats.Msg) {
		received <- msg.Data
	})
	require.NoError(t, err)

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

	select {
	case data := <-received:
		var decoded events.DriftEvent
		err := json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, event.ResourceID, decoded.ResourceID)
		assert.Equal(t, event.ChangeType, decoded.ChangeType)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for NATS message")
	}
}

func TestPublisher_Close_Safe(t *testing.T) {
	pub, err := events.NewPublisher("nats://localhost:4222")
	if err != nil {
		t.Skipf("NATS not available: %v", err)
	}
	pub.Close()
	pub.Close() // second close must not panic
}
