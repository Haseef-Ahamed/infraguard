package pagerduty

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const eventsAPIURL = "https://events.pagerduty.com/v2/enqueue"

// Client triggers PagerDuty incidents via the Events API v2
type Client struct {
	routingKey string
	httpClient *http.Client
	apiURL     string // overridable for tests
}

func NewClient(routingKey string) *Client {
	return &Client{
		routingKey: routingKey,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		apiURL:     eventsAPIURL,
	}
}

// NewClientWithURL is used by tests to point at a mock server
func NewClientWithURL(routingKey, url string) *Client {
	c := NewClient(routingKey)
	c.apiURL = url
	return c
}

type eventPayload struct {
	Summary  string `json:"summary"`
	Source   string `json:"source"`
	Severity string `json:"severity"`
}

type triggerEvent struct {
	RoutingKey  string       `json:"routing_key"`
	EventAction string       `json:"event_action"`
	DedupKey    string       `json:"dedup_key"`
	Payload     eventPayload `json:"payload"`
}

// TriggerIncident opens a new PagerDuty incident for an SLA breach
func (c *Client) TriggerIncident(resourceID string, minutesElapsed int) error {
	pdSeverity := "critical"
	event := triggerEvent{
		RoutingKey:  c.routingKey,
		EventAction: "trigger",
		DedupKey:    "infraguard-" + resourceID,
		Payload: eventPayload{
			Summary:  fmt.Sprintf("InfraGuard: CRITICAL drift on %s not remediated after %d minutes", resourceID, minutesElapsed),
			Source:   "infraguard-remediation-engine",
			Severity: pdSeverity,
		},
	}
	return c.send(event)
}

// ResolveIncident closes a previously triggered incident (e.g. after PR merge)
func (c *Client) ResolveIncident(resourceID string) error {
	event := triggerEvent{
		RoutingKey:  c.routingKey,
		EventAction: "resolve",
		DedupKey:    "infraguard-" + resourceID,
	}
	return c.send(event)
}

func (c *Client) send(event triggerEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal pagerduty event: %w", err)
	}
	resp, err := c.httpClient.Post(c.apiURL, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("post to pagerduty: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("pagerduty returned status %d", resp.StatusCode)
	}
	return nil
}
