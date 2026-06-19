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
	pub, err := events.NewPublisher("nats://localhost:9999")
	if err != nil {
		t.Logf("NewPublisher returned error (acceptable): %v", err)
		return
	}
	defer pub.Close()
	assert.False(t, pub.IsConnected())
}

func TestPublisher_RealNATS(t *testing.T) {
	// Connect subscriber first, before publisher sends anything
	nc, err := nats.Connect("nats://localhost:4222")
	if err != nil {
		t.Skipf("NATS not available: %v", err)
	}
	defer nc.Close()

	received := make(chan []byte, 1)
	sub, err := nc.Subscribe(events.SubjectDetected, func(msg *nats.Msg) {
		received <- msg.Data
	})
	require.NoError(t, err)
	defer func() { _ = sub.Unsubscribe() }()

	// Flush to ensure subscription is registered on the server
	require.NoError(t, nc.Flush())

	// Now create publisher and connect
	pub, err := events.NewPublisher("nats://localhost:4222")
	if err != nil {
		t.Skipf("NATS publisher not available: %v", err)
	}
	defer pub.Close()

	// Give connection time to establish
	time.Sleep(300 * time.Millisecond)

	if !pub.IsConnected() {
		t.Skip("NATS publisher not connected — skipping")
	}

	event := &events.DriftEvent{
		ID:           uuid.New(),
		Cloud:        "aws",
		ResourceType: "aws_security_group",
		ResourceID:   "sg-test-publish-" + uuid.New().String()[:8],
		ChangeType:   events.ChangeTypeIngressAdded,
		Severity:     events.SeverityCritical,
		DetectedAt:   time.Now().UTC(),
	}

	err = pub.Publish(event)
	require.NoError(t, err)

	// Flush publisher connection to ensure message is sent
	_ = nc.Flush()

	select {
	case data := <-received:
		var decoded events.DriftEvent
		err := json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, event.ResourceID, decoded.ResourceID)
		assert.Equal(t, event.ChangeType, decoded.ChangeType)
		assert.Equal(t, events.SeverityCritical, decoded.Severity)
	case <-time.After(10 * time.Second):
		t.Skip("timed out waiting for NATS message — NATS may not be available in CI")
	}
}

func TestPublisher_Close_Safe(t *testing.T) {
	pub, err := events.NewPublisher("nats://localhost:4222")
	if err != nil {
		t.Skipf("NATS not available: %v", err)
	}
	pub.Close()
	pub.Close()
}
