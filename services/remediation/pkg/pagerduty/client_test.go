package pagerduty_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/infraguard/remediation/pkg/pagerduty"
)

func TestTriggerIncident_SendsCorrectEvent(t *testing.T) {
	var received map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := pagerduty.NewClientWithURL("test-routing-key", server.URL)
	err := client.TriggerIncident("sg-abc123", 35)
	require.NoError(t, err)

	assert.Equal(t, "trigger", received["event_action"])
	assert.Equal(t, "test-routing-key", received["routing_key"])
	assert.Equal(t, "infraguard-sg-abc123", received["dedup_key"])

	payload := received["payload"].(map[string]interface{})
	assert.Contains(t, payload["summary"], "35 minutes")
	assert.Equal(t, "critical", payload["severity"])
}

func TestResolveIncident_SendsResolveAction(t *testing.T) {
	var received map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := pagerduty.NewClientWithURL("test-key", server.URL)
	err := client.ResolveIncident("sg-abc123")
	require.NoError(t, err)

	assert.Equal(t, "resolve", received["event_action"])
	assert.Equal(t, "infraguard-sg-abc123", received["dedup_key"])
}

func TestTriggerIncident_ErrorOnServerFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := pagerduty.NewClientWithURL("test-key", server.URL)
	err := client.TriggerIncident("sg-x", 10)
	assert.Error(t, err)
}
