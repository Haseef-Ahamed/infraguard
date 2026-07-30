package slack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client posts formatted messages to a Slack Incoming Webhook
type Client struct {
	webhookURL string
	channel    string
	httpClient *http.Client
}

func NewClient(webhookURL, channel string) *Client {
	return &Client{
		webhookURL: webhookURL,
		channel:    channel,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

type attachment struct {
	Color  string  `json:"color"`
	Title  string  `json:"title"`
	Text   string  `json:"text"`
	Fields []field `json:"fields"`
	Footer string  `json:"footer"`
	Ts     int64   `json:"ts"`
}

type field struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

type payload struct {
	Channel     string       `json:"channel"`
	Attachments []attachment `json:"attachments"`
}

// severityColor maps severity to Slack attachment color bars
var severityColor = map[string]string{
	"CRITICAL": "#e74c3c",
	"HIGH":     "#e67e22",
	"MEDIUM":   "#f1c40f",
	"LOW":      "#3498db",
	"INFO":     "#95a5a6",
}

// DriftAlert describes the fields shown in a drift Slack message
type DriftAlert struct {
	ResourceID string
	ChangeType string
	Severity   string
	Actor      string
	PRUrl      string // empty if no PR was opened
}

// PostDriftAlert sends a formatted drift notification to Slack
func (c *Client) PostDriftAlert(a DriftAlert) error {
	color := severityColor[a.Severity]
	if color == "" {
		color = "#95a5a6"
	}

	fields := []field{
		{Title: "Resource", Value: a.ResourceID, Short: true},
		{Title: "Change Type", Value: a.ChangeType, Short: true},
		{Title: "Actor", Value: a.Actor, Short: true},
	}
	if a.PRUrl != "" {
		fields = append(fields, field{Title: "Remediation PR", Value: a.PRUrl, Short: false})
	}

	p := payload{
		Channel: c.channel,
		Attachments: []attachment{{
			Color:  color,
			Title:  fmt.Sprintf("[%s] Infrastructure Drift Detected", a.Severity),
			Fields: fields,
			Footer: "InfraGuard",
			Ts:     time.Now().Unix(),
		}},
	}

	return c.post(p)
}

// PostEscalation sends an SLA-breach escalation message
func (c *Client) PostEscalation(resourceID string, minutesElapsed int) error {
	p := payload{
		Channel: c.channel,
		Attachments: []attachment{{
			Color: "#c0392b",
			Title: "⚠️ SLA BREACH — Remediation Not Merged",
			Fields: []field{
				{Title: "Resource", Value: resourceID, Short: true},
				{Title: "Minutes Elapsed", Value: fmt.Sprintf("%d", minutesElapsed), Short: true},
			},
			Footer: "InfraGuard — escalating to PagerDuty",
			Ts:     time.Now().Unix(),
		}},
	}
	return c.post(p)
}

func (c *Client) post(p payload) error {
	data, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}
	resp, err := c.httpClient.Post(c.webhookURL, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("post to slack: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("slack returned status %d", resp.StatusCode)
	}
	return nil
}
