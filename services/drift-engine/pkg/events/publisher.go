package events

import (
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
)

// Publisher publishes DriftEvents to NATS subjects
type Publisher struct {
	nc *nats.Conn
}

// NewPublisher connects to NATS at the given URL.
// It retries connection automatically on disconnect.
func NewPublisher(url string) (*Publisher, error) {
	nc, err := nats.Connect(url,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(10),
		nats.Name("infraguard-drift-engine"),
	)
	if err != nil {
		return nil, fmt.Errorf("nats: connect to %s: %w", url, err)
	}
	return &Publisher{nc: nc}, nil
}

// Publish serialises the DriftEvent to JSON and publishes it
// to the infraguard.drift.detected NATS subject.
func (p *Publisher) Publish(event *DriftEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("nats: marshal event: %w", err)
	}
	if err := p.nc.Publish(SubjectDetected, data); err != nil {
		return fmt.Errorf("nats: publish to %s: %w", SubjectDetected, err)
	}
	return nil
}

// Close gracefully shuts down the NATS connection
func (p *Publisher) Close() {
	if p.nc != nil {
		p.nc.Drain()
	}
}

// IsConnected returns true if the NATS connection is active
func (p *Publisher) IsConnected() bool {
	return p.nc != nil && p.nc.IsConnected()
}
