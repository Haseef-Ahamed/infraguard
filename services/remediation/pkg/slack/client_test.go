package slack_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/infraguard/remediation/pkg/slack"
)

func TestPostDriftAlert_SendsCorrectPayload(t *testing.T) {
	var received map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := slack.NewClient(server.URL, "#ops-alerts")
	err := client.PostDriftAlert(slack.DriftAlert{
		ResourceID: "sg-abc123", ChangeType: "INGRESS_RULE_ADDED",
		Severity: "CRITICAL", Actor: "arn:aws:iam::000:root",
	})
	require.NoError(t, err)

	assert.Equal(t, "#ops-alerts", received["channel"])
	attachments := received["attachments"].([]interface{})
	require.Len(t, attachments, 1)
	att := attachments[0].(map[string]interface{})
	assert.Contains(t, att["title"], "CRITICAL")
	assert.Equal(t, "#e74c3c", att["color"])
}

func TestPostDriftAlert_IncludesPRLink(t *testing.T) {
	var received map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := slack.NewClient(server.URL, "#ops-alerts")
	err := client.PostDriftAlert(slack.DriftAlert{
		ResourceID: "sg-abc123", Severity: "HIGH",
		PRUrl: "https://github.com/org/repo/pull/42",
	})
	require.NoError(t, err)

	att := received["attachments"].([]interface{})[0].(map[string]interface{})
	fields := att["fields"].([]interface{})

	found := false
	for _, f := range fields {
		fm := f.(map[string]interface{})
		if fm["title"] == "Remediation PR" {
			found = true
			assert.Equal(t, "https://github.com/org/repo/pull/42", fm["value"])
		}
	}
	assert.True(t, found, "PR field should be present")
}

func TestPostEscalation_SendsCorrectPayload(t *testing.T) {
	var received map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := slack.NewClient(server.URL, "#ops-alerts")
	err := client.PostEscalation("sg-abc123", 35)
	require.NoError(t, err)

	att := received["attachments"].([]interface{})[0].(map[string]interface{})
	assert.Contains(t, att["title"], "SLA BREACH")
}

func TestPost_ReturnsErrorOnServerFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := slack.NewClient(server.URL, "#ops-alerts")
	err := client.PostDriftAlert(slack.DriftAlert{ResourceID: "sg-x", Severity: "LOW"})
	assert.Error(t, err)
}
