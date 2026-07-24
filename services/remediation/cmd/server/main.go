package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/infraguard/remediation/pkg/github"
	"github.com/infraguard/remediation/pkg/remediate"
)

const subjectDetected = "infraguard.drift.detected"

// DriftEvent mirrors the drift-engine's event structure
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
	autoApply := getEnv("AUTO_APPLY_LOW", "false") == "true"

	if ghToken == "" {
		log.Warn("GITHUB_TOKEN not set — PRs will not be created, running in log-only mode")
	}

	var ghClient *github.Client
	if ghToken != "" {
		ghClient = github.NewClient(ghToken, ghOwner, ghRepo)
	}

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

		switch severity {
		case "CRITICAL", "HIGH":
			if ghClient == nil {
				log.Info("would open PR (log-only mode)", zap.String("resource_id", event.ResourceID))
				return
			}
			handleRemediation(ghClient, event, log)
			prsOpened++
		case "LOW":
			if autoApply {
				log.Info("LOW severity auto-apply enabled — would apply directly", zap.String("resource_id", event.ResourceID))
			}
		default:
			log.Debug("severity below remediation threshold", zap.String("severity", severity))
		}
	})
	if err != nil {
		log.Fatal("subscribe", zap.Error(err))
	}

	// HTTP health endpoint
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			fmt.Fprint(w, "ok")
		})
		mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
			if nc.IsConnected() {
				w.WriteHeader(200)
				fmt.Fprint(w, "ready")
			} else {
				w.WriteHeader(503)
			}
		})
		http.ListenAndServe(getEnv("METRICS_ADDR", ":8082"), mux)
	}()

	log.Info("remediation engine listening", zap.String("subject", subjectDetected))

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Info("shutting down", zap.Int("prs_opened_this_session", prsOpened))
}

func handleRemediation(gh *github.Client, event DriftEvent, log *zap.Logger) {
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
		BranchName:  branch,
		Title:       title,
		Body:        body,
		FilePath:    "infra/aws/security_groups/main.tf",
		FileContent: fix,
		CommitMsg:   fmt.Sprintf("fix: revert unauthorized change on %s", event.ResourceID),
	}

	url, num, err := gh.OpenRemediationPR(ctx, pr)
	if err != nil {
		log.Error("failed to open remediation PR", zap.Error(err), zap.String("resource_id", event.ResourceID))
		return
	}
	log.Info("remediation PR opened", zap.String("url", url), zap.Int("pr_number", num))
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
