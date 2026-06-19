package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	awscloud "github.com/infraguard/drift-engine/pkg/cloud/aws"
	"github.com/infraguard/drift-engine/pkg/events"
	"github.com/infraguard/drift-engine/pkg/metrics"
	"github.com/infraguard/drift-engine/pkg/state"
	"github.com/infraguard/drift-engine/pkg/vault"
)

func main() {
	// ── Logger ──────────────────────────────────────────────────────────────
	log, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = log.Sync() }()

	log.Info("InfraGuard Drift Detection Engine starting")

	// ── Context with graceful shutdown ──────────────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Vault client ────────────────────────────────────────────────────────
	vaultAddr  := getEnv("VAULT_ADDR", "http://localhost:8200")
	vaultToken := getEnv("VAULT_TOKEN", "root")

	vc, err := vault.NewClient(vaultAddr, vaultToken)
	if err != nil {
		log.Fatal("failed to create Vault client", zap.Error(err))
	}
	log.Info("Vault client initialised", zap.String("addr", vaultAddr))

	// ── Read secrets from Vault ──────────────────────────────────────────────
	awsCreds, err := vc.Get("aws")
	if err != nil {
		log.Fatal("failed to read AWS credentials from Vault", zap.Error(err))
	}

	pgCreds, err := vc.Get("postgres")
	if err != nil {
		log.Fatal("failed to read PostgreSQL credentials from Vault", zap.Error(err))
	}

	natsCreds, err := vc.Get("nats")
	if err != nil {
		log.Fatal("failed to read NATS URL from Vault", zap.Error(err))
	}

	// ── AWS Detector ─────────────────────────────────────────────────────────
	detector, err := awscloud.NewDetector(
		awsCreds["access_key"],
		awsCreds["secret_key"],
		awsCreds["region"],
		awsCreds["endpoint"],
		log,
	)
	if err != nil {
		log.Fatal("failed to create AWS detector", zap.Error(err))
	}
	log.Info("AWS detector initialised", zap.String("endpoint", awsCreds["endpoint"]))

	// ── PostgreSQL Store ─────────────────────────────────────────────────────
	connStr := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s",
		pgCreds["username"], pgCreds["password"],
		pgCreds["host"], pgCreds["port"], pgCreds["database"],
	)
	store, err := state.NewStore(ctx, connStr)
	if err != nil {
		log.Fatal("failed to connect to PostgreSQL", zap.Error(err))
	}
	defer store.Close()
	log.Info("PostgreSQL store connected")

	// ── NATS Publisher ───────────────────────────────────────────────────────
	publisher, err := events.NewPublisher(natsCreds["url"])
	if err != nil {
		log.Fatal("failed to connect to NATS", zap.Error(err))
	}
	defer publisher.Close()
	log.Info("NATS publisher connected", zap.String("url", natsCreds["url"]))

	// ── Comparator ───────────────────────────────────────────────────────────
	comparator := state.NewComparator()

	// ── Seed IaC baseline ────────────────────────────────────────────────────
	log.Info("seeding IaC baselines from current cloud state")
	if err := seedBaselines(ctx, detector, store, log); err != nil {
		log.Warn("baseline seeding failed", zap.Error(err))
	}

	// ── HTTP server for metrics + health ────────────────────────────────────
	metricsAddr := getEnv("METRICS_ADDR", ":8080")
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "ok")
		})
		mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
			if publisher.IsConnected() {
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, "ready")
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprint(w, "nats not connected")
			}
		})
		log.Info("HTTP server listening", zap.String("addr", metricsAddr))
		if err := http.ListenAndServe(metricsAddr, mux); err != nil {
			log.Error("HTTP server error", zap.Error(err))
		}
	}()

	// ── Drift scan function ──────────────────────────────────────────────────
	scan := func() {
		scanStart := time.Now()
		log.Info("starting drift scan")
		driftsFound := 0

		sgCtx, sgCancel := context.WithTimeout(ctx, 30*time.Second)
		defer sgCancel()

		liveGroups, err := detector.GetAllSecurityGroups(sgCtx)
		if err != nil {
			log.Error("failed to fetch security groups", zap.Error(err))
		} else {
			metrics.ResourcesMonitored.WithLabelValues("aws", "aws_security_group").
				Set(float64(len(liveGroups)))

			var baselineGroups []events.SecurityGroupState
			for _, live := range liveGroups {
				raw, err := store.GetLatestBaseline(ctx, live.GroupID)
				if err != nil || raw == nil {
					continue
				}
				baselineGroups = append(baselineGroups, events.SecurityGroupState{
					GroupID: live.GroupID,
				})
			}

			drifts := comparator.CompareSecurityGroups(liveGroups, baselineGroups)
			for i := range drifts {
				d := &drifts[i]
				detectionTime := time.Since(scanStart).Seconds()

				if err := store.SaveDriftEvent(ctx, d); err != nil {
					log.Error("failed to save drift event", zap.Error(err))
					continue
				}

				if err := publisher.Publish(d); err != nil {
					metrics.NATSPublishErrors.Inc()
					log.Error("failed to publish drift event", zap.Error(err))
				} else {
					metrics.DriftEventsTotal.WithLabelValues(
						d.Cloud, d.ResourceType,
						string(d.Severity), string(d.ChangeType),
					).Inc()
					metrics.DetectionLatency.Observe(detectionTime)
					log.Info("drift event detected and published",
						zap.String("resource_id", d.ResourceID),
						zap.String("change_type", string(d.ChangeType)),
						zap.String("cloud", d.Cloud),
					)
					driftsFound++
				}
			}
		}

		scanDuration := time.Since(scanStart)
		metrics.ScanDuration.Observe(scanDuration.Seconds())
		log.Info("drift scan complete",
			zap.Int("drifts_found", driftsFound),
			zap.Duration("duration", scanDuration),
		)
	}

	scan()

	interval := 5 * time.Minute
	if envInterval := getEnv("SCAN_INTERVAL_SECONDS", ""); envInterval != "" {
		if d, err := time.ParseDuration(envInterval + "s"); err == nil {
			interval = d
		}
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Info("polling loop started", zap.Duration("interval", interval))

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			scan()
		case s := <-sig:
			log.Info("received shutdown signal", zap.String("signal", s.String()))
			return
		case <-ctx.Done():
			return
		}
	}
}

func seedBaselines(ctx context.Context, detector *awscloud.Detector, store *state.Store, log *zap.Logger) error {
	seedCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	groups, err := detector.GetAllSecurityGroups(seedCtx)
	if err != nil {
		return fmt.Errorf("seed: fetch security groups: %w", err)
	}

	for _, sg := range groups {
		existing, err := store.GetLatestBaseline(ctx, sg.GroupID)
		if err != nil {
			return err
		}
		if existing != nil {
			continue
		}
		if err := store.UpsertBaseline(ctx, sg.GroupID, sg, "seed"); err != nil {
			log.Warn("failed to seed baseline",
				zap.String("group_id", sg.GroupID), zap.Error(err))
		} else {
			log.Info("seeded baseline", zap.String("resource_id", sg.GroupID))
		}
	}
	return nil
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
