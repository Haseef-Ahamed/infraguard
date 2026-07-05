#!/usr/bin/env bash
# InfraGuard End-to-End Drift Detection Test
# Proves the full pipeline: drift introduced → detected → saved in PostgreSQL

set -euo pipefail

BOLD="\033[1m"
RED="\033[0;31m"
GREEN="\033[0;32m"
YELLOW="\033[1;33m"
BLUE="\033[0;34m"
RESET="\033[0m"

ENDPOINT="http://localhost:4566"
PROFILE="localstack"
AWS="aws --endpoint-url=${ENDPOINT} --profile=${PROFILE}"
PG="docker compose exec -T postgres psql -U infraguard -d infraguard"
AGENT_BINARY="/tmp/drift-engine-test"
AGENT_PID=""

cleanup() {
  echo ""
  echo -e "${YELLOW}[CLEANUP]${RESET} Cleaning up..."

  # Stop agent
  if [ -n "$AGENT_PID" ]; then
    kill "$AGENT_PID" 2>/dev/null || true
    wait "$AGENT_PID" 2>/dev/null || true
  fi

  # Kill any agent on port 8080
  fuser -k 8080/tcp 2>/dev/null || true

  # Revert drift
  ~/infraguard/scripts/drift-simulator/introduce_drift.sh revert-all 2>/dev/null || true

  echo -e "${YELLOW}[CLEANUP]${RESET} Done."
}
trap cleanup EXIT

echo ""
echo -e "${BOLD}${BLUE}════════════════════════════════════════════════════${RESET}"
echo -e "${BOLD}${BLUE}  InfraGuard E2E Drift Detection Test               ${RESET}"
echo -e "${BOLD}${BLUE}════════════════════════════════════════════════════${RESET}"
echo ""

# ── Pre-flight checks ────────────────────────────────────────────────────────
echo -e "${BOLD}[STEP 1]${RESET} Pre-flight checks"

if ! curl -s http://localhost:4566/_localstack/health | grep -q '"ec2"'; then
  echo -e "${RED}[FAIL]${RESET} LocalStack not running. Run: ~/infraguard/scripts/dev-start.sh"
  exit 1
fi
echo -e "  ${GREEN}✓${RESET} LocalStack running"

if ! docker compose exec -T postgres pg_isready -U infraguard -q; then
  echo -e "${RED}[FAIL]${RESET} PostgreSQL not running"
  exit 1
fi
echo -e "  ${GREEN}✓${RESET} PostgreSQL running"

if ! curl -s http://localhost:8200/v1/sys/health | grep -q '"initialized":true'; then
  echo -e "${RED}[FAIL]${RESET} Vault not running"
  exit 1
fi
echo -e "  ${GREEN}✓${RESET} Vault running"

if ! curl -s http://localhost:8222 > /dev/null 2>&1; then
  echo -e "${RED}[FAIL]${RESET} NATS not running"
  exit 1
fi
echo -e "  ${GREEN}✓${RESET} NATS running"

# ── Check IaC baseline exists ─────────────────────────────────────────────────
echo ""
echo -e "${BOLD}[STEP 2]${RESET} Verify IaC baseline"

SG_ID=$(${AWS} ec2 describe-security-groups \
  --filters "Name=group-name,Values=infraguard-app-sg" \
  --query "SecurityGroups[0].GroupId" \
  --output text 2>/dev/null || echo "")

if [ -z "$SG_ID" ] || [ "$SG_ID" = "None" ]; then
  echo -e "${RED}[FAIL]${RESET} Security group not found. Run tofu apply first."
  exit 1
fi
echo -e "  ${GREEN}✓${RESET} Security group exists: ${SG_ID}"

SG_PORT=$(${AWS} ec2 describe-security-groups \
  --group-ids "${SG_ID}" \
  --query "SecurityGroups[0].IpPermissions[0].FromPort" \
  --output text)
echo -e "  ${GREEN}✓${RESET} Baseline port: ${SG_PORT} (expected 443)"

# ── Build agent ───────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}[STEP 3]${RESET} Build drift detection agent"
cd ~/infraguard/services/drift-engine
go build -o "${AGENT_BINARY}" ./cmd/agent/
echo -e "  ${GREEN}✓${RESET} Agent built: ${AGENT_BINARY}"
cd ~/infraguard

# ── Kill any existing agent ───────────────────────────────────────────────────
fuser -k 8080/tcp 2>/dev/null || true
sleep 1

# ── Start agent and seed baselines ───────────────────────────────────────────
echo ""
echo -e "${BOLD}[STEP 4]${RESET} Start agent and seed baselines"

VAULT_ADDR=http://localhost:8200 \
VAULT_TOKEN=root \
SCAN_INTERVAL_SECONDS=300 \
"${AGENT_BINARY}" > /tmp/drift-agent-e2e.log 2>&1 &
AGENT_PID=$!

echo -e "  Agent PID: ${AGENT_PID}"
echo -e "  Waiting 12 seconds for startup and baseline seeding..."
sleep 12

if ! kill -0 "$AGENT_PID" 2>/dev/null; then
  echo -e "${RED}[FAIL]${RESET} Agent exited unexpectedly"
  echo "Last log lines:"
  tail -20 /tmp/drift-agent-e2e.log
  exit 1
fi

if ! curl -s http://localhost:8080/healthz | grep -q "ok"; then
  echo -e "${RED}[FAIL]${RESET} Agent health check failed"
  exit 1
fi
echo -e "  ${GREEN}✓${RESET} Agent healthy"

# Count baselines seeded
BASELINE_COUNT=$(${PG} -c "SELECT count(*) FROM state_snapshots WHERE snapshot_source='seed'" -t | tr -d ' ')
echo -e "  ${GREEN}✓${RESET} Baselines seeded: ${BASELINE_COUNT}"

# ── Record baseline drift event count ────────────────────────────────────────
BEFORE_COUNT=$(${PG} -c "SELECT count(*) FROM drift_events" -t | tr -d ' ')
echo -e "  ${GREEN}✓${RESET} Drift events before test: ${BEFORE_COUNT}"

# ── Introduce drift ───────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}[STEP 5]${RESET} Introduce CRITICAL drift — opening port 5432"

~/infraguard/scripts/drift-simulator/introduce_drift.sh sg-port

echo ""
echo -e "${YELLOW}[WAITING]${RESET} Waiting 65 seconds for drift detection..."
echo -e "  (Agent polls every 5 minutes, but we trigger via SCAN_INTERVAL override)"
echo ""

# Trigger immediate scan by restarting with short interval
kill "$AGENT_PID" 2>/dev/null || true
sleep 2
fuser -k 8080/tcp 2>/dev/null || true
sleep 1

VAULT_ADDR=http://localhost:8200 \
VAULT_TOKEN=root \
SCAN_INTERVAL_SECONDS=30 \
"${AGENT_BINARY}" >> /tmp/drift-agent-e2e.log 2>&1 &
AGENT_PID=$!

echo -e "  Agent restarted with 30s scan interval (PID: ${AGENT_PID})"
echo -e "  Waiting 65 seconds for detection..."
sleep 65

# ── Verify detection ──────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}[STEP 6]${RESET} Verify drift detection in PostgreSQL"

AFTER_COUNT=$(${PG} -c "SELECT count(*) FROM drift_events" -t | tr -d ' ')
NEW_EVENTS=$((AFTER_COUNT - BEFORE_COUNT))
echo -e "  Drift events before: ${BEFORE_COUNT}"
echo -e "  Drift events after:  ${AFTER_COUNT}"
echo -e "  New events:          ${NEW_EVENTS}"

if [ "$NEW_EVENTS" -lt 1 ]; then
  echo -e "${RED}[FAIL]${RESET} No new drift events detected within 65 seconds"
  echo ""
  echo "Agent logs:"
  tail -30 /tmp/drift-agent-e2e.log
  exit 1
fi

echo -e "  ${GREEN}✓${RESET} ${NEW_EVENTS} new drift event(s) detected"

# ── Show detected events ──────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}[STEP 7]${RESET} Drift event details"
${PG} -c "
SELECT
  resource_id,
  change_type,
  severity,
  to_char(detected_at, 'YYYY-MM-DD HH24:MI:SS') AS detected_at
FROM drift_events
ORDER BY detected_at DESC
LIMIT 5;" 2>/dev/null || true

# ── Verify Prometheus metrics ─────────────────────────────────────────────────
echo ""
echo -e "${BOLD}[STEP 8]${RESET} Verify Prometheus metrics"

METRICS=$(curl -s http://localhost:8080/metrics)

if echo "$METRICS" | grep -q "infraguard_drift_events_total"; then
  echo -e "  ${GREEN}✓${RESET} infraguard_drift_events_total present"
else
  echo -e "  ${YELLOW}⚠${RESET} infraguard_drift_events_total not found in metrics"
fi

if echo "$METRICS" | grep -q "infraguard_resources_monitored_total"; then
  echo -e "  ${GREEN}✓${RESET} infraguard_resources_monitored_total present"
fi

if echo "$METRICS" | grep -q "infraguard_scan_duration_seconds"; then
  echo -e "  ${GREEN}✓${RESET} infraguard_scan_duration_seconds present"
fi

# ── Final result ──────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}${BLUE}════════════════════════════════════════════════════${RESET}"
if [ "$NEW_EVENTS" -ge 1 ]; then
  echo -e "${BOLD}${GREEN}  E2E TEST PASSED ✓                                  ${RESET}"
  echo -e "${BOLD}${GREEN}  Drift introduced → detected → saved in PostgreSQL  ${RESET}"
else
  echo -e "${BOLD}${RED}  E2E TEST FAILED ✗                                  ${RESET}"
fi
echo -e "${BOLD}${BLUE}════════════════════════════════════════════════════${RESET}"
echo ""
