#!/usr/bin/env bash
#
# infraguard-startup.sh
#
# Post-reboot startup runbook for the InfraGuard dev environment.
#
# USAGE:
#   ./infraguard-startup.sh              # run everything in order
#   ./infraguard-startup.sh deps         # just bring up docker compose deps
#   ./infraguard-startup.sh baseline     # just re-apply IaC baseline check
#   ./infraguard-startup.sh build        # just build the drift-engine binary
#   ./infraguard-startup.sh smoke        # build + run locally + hit health endpoints
#   ./infraguard-startup.sh k8s          # check/reapply kind cluster manifests
#   ./infraguard-startup.sh e2e          # run the full E2E test script
#   ./infraguard-startup.sh ml           # ml-predictor data gen + preprocess
#
# Steps are independent functions, so you can also just source this file
# and call e.g. `smoke` directly in your shell.

set -euo pipefail

REPO_ROOT="$HOME/infraguard"
DRIFT_ENGINE_DIR="$REPO_ROOT/services/drift-engine"
ML_DIR="$REPO_ROOT/services/ml-predictor"
CHARTS_DIR="$REPO_ROOT/charts/infraguard/templates"
BIN="/tmp/drift-engine-test"

log() { echo -e "\n\033[1;34m==> $1\033[0m"; }

deps() {
  log "Starting docker compose dependencies (Postgres, Vault, NATS, LocalStack, TimescaleDB)"
  cd "$REPO_ROOT"
  docker compose up -d
}

baseline() {
  log "Re-checking IaC baseline with OpenTofu"
  cd "$REPO_ROOT/infra"
  # NOTE: use 'command tofu' (not bare 'tofu') so this never collides with
  # a shell function of the same name — that collision is what caused the
  # infinite "Re-checking IaC baseline" loop previously.
  TF_VAR_db_password=infraguard_dev command tofu apply -auto-approve
}

build() {
  log "Building drift-engine agent"
  cd "$DRIFT_ENGINE_DIR"
  go mod tidy
  go build -o "$BIN" ./cmd/agent/
  echo "Build OK: $BIN"
}

smoke() {
  build
  log "Running agent locally for a smoke test"
  # Free port 8080 first in case a stale instance is still around
  fuser -k 8080/tcp 2>/dev/null || true
  sleep 1

  VAULT_ADDR=http://localhost:8200 VAULT_TOKEN=root "$BIN" &
  local pid=$!
  sleep 8

  echo "-- healthz --"
  curl -s http://localhost:8080/healthz; echo
  echo "-- readyz --"
  curl -s http://localhost:8080/readyz; echo
  echo "-- metrics (infraguard_*) --"
  curl -s http://localhost:8080/metrics | grep infraguard

  kill "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
}

k8s() {
  log "Checking kind cluster pods"
  if ! kubectl get pods -n drift-engine >/dev/null 2>&1; then
    echo "Namespace/pods not found, reapplying manifests..."
    kubectl apply -f "$CHARTS_DIR/drift-engine-rbac.yaml"
    kubectl apply -f "$CHARTS_DIR/drift-engine-config.yaml"
    kubectl apply -f "$CHARTS_DIR/drift-engine-daemonset.yaml"
    kubectl apply -f "$CHARTS_DIR/drift-engine-service.yaml"
  fi
  kubectl rollout status daemonset/drift-engine -n drift-engine --timeout=120s
  kubectl get pods -n drift-engine -o wide
}

e2e() {
  log "Running full E2E drift detection test"
  bash "$REPO_ROOT/scripts/e2e/test_drift_detection.sh"
}

ml() {
  log "Running ml-predictor data generation + preprocessing"
  cd "$ML_DIR"
  # shellcheck disable=SC1091
  source venv/bin/activate
  python3 src/generate_data.py
  python3 src/preprocess.py
  echo
  echo "NOTE: 'python3 src/train.py' is known to crash with 'Illegal instruction'"
  echo "on this VM's virtual CPU (tensorflow-cpu AVX mismatch). Not fixed by a reboot."
}

all() {
  deps
  baseline
  smoke
  k8s
  e2e
}

# ---- entrypoint ----
cmd="${1:-all}"
case "$cmd" in
  deps|baseline|build|smoke|k8s|e2e|ml|all) "$cmd" ;;
  *)
    echo "Unknown command: $cmd"
    echo "Usage: $0 [deps|baseline|build|smoke|k8s|e2e|ml|all]"
    exit 1
    ;;
esac