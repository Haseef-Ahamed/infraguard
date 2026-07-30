package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/infraguard/remediation/pkg/github"
	"github.com/infraguard/remediation/pkg/pagerduty"
	"github.com/infraguard/remediation/pkg/remediate"
	"github.com/infraguard/remediation/pkg/sla"
	"github.com/infraguard/remediation/pkg/slack"
	"github.com/infraguard/remediation/pkg/ws"
)

const subjectDetected = "infraguard.drift.detected"

type DriftEvent struct {
	ResourceID   string `json:"resource_id"`
	ResourceType string `json:"resource_type"`
	Cloud        string `json:"cloud"`
	ChangeType   string `json:"change_type"`
	Actor        string `json:"actor"`
	Severity     string `json:"severity"`
}

func main() {
	log, _ := zap.NewProduction()
	defer func() { _ = log.Sync() }()
	log.Info("InfraGuard Remediation Engine starting")

	natsURL := getEnv("NATS_URL", "nats://localhost:4222")
	ghToken := getEnv("GITHUB_TOKEN", "")
	ghOwner := getEnv("GITHUB_OWNER", "")
	ghRepo := getEnv("GITHUB_REPO", "infraguard")
	slackWebhook := getEnv("SLACK_WEBHOOK_URL", "")
	slackChannel := getEnv("SLACK_CHANNEL", "#ops-alerts")
	pdRoutingKey := getEnv("PAGERDUTY_ROUTING_KEY", "")
	slaMinutes := 30 // CRITICAL SLA — configurable via env if needed

	var ghClient *github.Client
	if ghToken != "" {
		ghClient = github.NewClient(ghToken, ghOwner, ghRepo)
	} else {
		log.Warn("GITHUB_TOKEN not set — PRs will not be created, log-only mode")
	}

	var slackClient *slack.Client
	if slackWebhook != "" && slackWebhook != "https://hooks.slack.com/services/PLACEHOLDER" {
		slackClient = slack.NewClient(slackWebhook, slackChannel)
	} else {
		log.Warn("SLACK_WEBHOOK_URL not set or placeholder — Slack alerts disabled")
	}

	var pdClient *pagerduty.Client
	if pdRoutingKey != "" {
		pdClient = pagerduty.NewClient(pdRoutingKey)
	} else {
		log.Warn("PAGERDUTY_ROUTING_KEY not set — escalation disabled")
	}

	hub := ws.NewHub(log)

	// SLA tracker — fires escalation when CRITICAL drift isn't resolved in time
	tracker := sla.NewTracker(slaMinutes, func(resourceID string, minutesElapsed int) {
		log.Warn("SLA BREACH", zap.String("resource_id", resourceID), zap.Int("minutes_elapsed", minutesElapsed))
		if slackClient != nil {
			_ = slackClient.PostEscalation(resourceID, minutesElapsed)
		}
		if pdClient != nil {
			if err := pdClient.TriggerIncident(resourceID, minutesElapsed); err != nil {
				log.Error("pagerduty trigger failed", zap.Error(err))
			}
		}
	})
	tracker.Run(1 * time.Minute) // check every minute
	defer tracker.Stop()

	nc, err := nats.Connect(natsURL, nats.RetryOnFailedConnect(true))
	if err != nil {
		log.Fatal("nats connect", zap.Error(err))
	}
	defer nc.Close()

	prsOpened := 0
	_, err = nc.Subscribe(subjectDetected, func(msg *nats.Msg) {
		var event DriftEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			log.Error("unmarshal drift event", zap.Error(err))
			return
		}
		severity := event.Severity
		if severity == "" {
			severity = "INFO"
		}

		log.Info("received drift event",
			zap.String("resource_id", event.ResourceID),
			zap.String("severity", severity),
			zap.String("change_type", event.ChangeType),
		)
		hub.Broadcast(event)

		var prURL string
		switch severity {
		case "CRITICAL", "HIGH":
			tracker.TrackEvent(event.ResourceID)

			if ghClient != nil {
				url, num, err := handleRemediation(ghClient, event, log)
				if err == nil {
					prURL = url
					prsOpened++
					log.Info("remediation PR opened", zap.String("url", url), zap.Int("pr_number", num))
				}
			} else {
				log.Info("would open PR (log-only mode)", zap.String("resource_id", event.ResourceID))
			}
		default:
			log.Debug("severity below remediation threshold", zap.String("severity", severity))
		}

		if slackClient != nil {
			_ = slackClient.PostDriftAlert(slack.DriftAlert{
				ResourceID: event.ResourceID, ChangeType: event.ChangeType,
				Severity: severity, Actor: event.Actor, PRUrl: prURL,
			})
		}
	})
	if err != nil {
		log.Fatal("subscribe", zap.Error(err))
	}

	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); fmt.Fprint(w, "ok") })
		mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
			if nc.IsConnected() {
				w.WriteHeader(200)
				fmt.Fprint(w, "ready")
			} else {
				w.WriteHeader(503)
			}
		})
		mux.HandleFunc("/ws", hub.HandleWS)
		mux.HandleFunc("/sla-status", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"tracked_events": %d}`, tracker.TrackedCount())
		})
		http.ListenAndServe(getEnv("METRICS_ADDR", ":8082"), mux)
	}()

	log.Info("remediation engine listening", zap.String("subject", subjectDetected))

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Info("shutting down", zap.Int("prs_opened_this_session", prsOpened))
}

func handleRemediation(gh *github.Client, event DriftEvent, log *zap.Logger) (string, int, error) {
	ctx := context.Background()
	input := remediate.DriftInput{
		ResourceID: event.ResourceID, ResourceType: event.ResourceType,
		ChangeType: event.ChangeType, Severity: event.Severity, Actor: event.Actor,
	}
	branch := remediate.GenerateBranchName(event.Severity, event.ResourceID)
	title := remediate.GeneratePRTitle(input)
	body := remediate.GeneratePRBody(input)
	fix := remediate.GenerateSGFix(event.ResourceID)

	pr := github.RemediationPR{
		BranchName: branch, Title: title, Body: body,
		FilePath: "infra/aws/security_groups/main.tf", FileContent: fix,
		CommitMsg: fmt.Sprintf("fix: revert unauthorized change on %s", event.ResourceID),
	}
	return gh.OpenRemediationPR(ctx, pr)
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
