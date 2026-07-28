#!/usr/bin/env bash
echo "════════════════════════════════════════════"
echo "  InfraGuard — Full System Verification"
echo "════════════════════════════════════════════"
PASS=0; FAIL=0
check() { if [ "$2" = "true" ]; then echo "  [PASS] $1"; PASS=$((PASS+1)); else echo "  [FAIL] $1"; FAIL=$((FAIL+1)); fi; }

echo ""; echo "── Docker Services ──"
docker compose ps --format "{{.Name}}: {{.Status}}"
RUNNING=$(docker compose ps --format "{{.Status}}" | grep -c "Up\|running\|healthy")
check "8 Docker services running" "$([ "$RUNNING" -ge 8 ] && echo true || echo false)"

echo ""; echo "── LocalStack ──"
LS_OK=$(curl -s http://localhost:4566/_localstack/health 2>/dev/null | grep -q '"ec2"' && echo true || echo false)
check "LocalStack responding" "$LS_OK"
SG_PORT=$(aws --endpoint-url=http://localhost:4566 --profile localstack ec2 describe-security-groups --query "SecurityGroups[?GroupName=='infraguard-app-sg'].IpPermissions[0].FromPort" --output text 2>/dev/null)
check "Security group baseline (port 443)" "$([ "$SG_PORT" = "443" ] && echo true || echo false)"

echo ""; echo "── PostgreSQL ──"
PG_OK=$(docker compose exec -T postgres pg_isready -U infraguard -q 2>/dev/null && echo true || echo false)
check "PostgreSQL accepting connections" "$PG_OK"
TABLE_COUNT=$(docker compose exec -T postgres psql -U infraguard -d infraguard -c "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'" -t 2>/dev/null | tr -d ' ')
check "5 tables exist" "$([ "$TABLE_COUNT" = "5" ] && echo true || echo false)"

echo ""; echo "── TimescaleDB ──"
TS_COUNT=$(docker compose exec -T timescaledb psql -U infraguard -d infraguard_ts -c "SELECT count(*) FROM telemetry" -t 2>/dev/null | tr -d ' ')
check "Telemetry data exists" "$([ -n "$TS_COUNT" ] && [ "$TS_COUNT" -gt 0 ] && echo true || echo false)"

echo ""; echo "── Vault ──"
VAULT_OK=$(curl -s http://localhost:8200/v1/sys/health 2>/dev/null | grep -q '"initialized":true' && echo true || echo false)
check "Vault initialised" "$VAULT_OK"
export VAULT_ADDR=http://localhost:8200
export VAULT_TOKEN=root
SECRET_COUNT=$(vault kv list infraguard/ 2>/dev/null | grep -v -E "^Keys|^----" | grep -c . || echo 0)
check "6 Vault secrets stored" "$([ "$SECRET_COUNT" = "6" ] && echo true || echo false)"

echo ""; echo "── NATS ──"
NATS_OK=$(curl -s http://localhost:8222 2>/dev/null | grep -qi "nats" && echo true || echo false)
check "NATS running" "$NATS_OK"

echo ""; echo "── kind Kubernetes Cluster ──"
NODES_READY=$(kubectl get nodes --no-headers 2>/dev/null | grep -c Ready)
check "3 nodes Ready" "$([ "$NODES_READY" = "3" ] && echo true || echo false)"
DRIFT_PODS=$(kubectl get pods -n drift-engine --no-headers 2>/dev/null | grep -c Running)
check "Drift-engine pods running" "$([ "$DRIFT_PODS" -ge 1 ] && echo true || echo false)"

echo ""; echo "── ArgoCD ──"
ARGOCD_PODS=$(kubectl get pods -n argocd --no-headers 2>/dev/null | grep -c "1/1\|2/2")
check "ArgoCD pods running" "$([ "$ARGOCD_PODS" -ge 5 ] && echo true || echo false)"
ARGOCD_APP=$(argocd app get infraguard-platform 2>/dev/null | grep -c "Name:")
ARGOCD_APP=${ARGOCD_APP:-0}
check "ArgoCD application registered" "$([ "$ARGOCD_APP" -ge 1 ] 2>/dev/null && echo true || echo false)"

echo ""; echo "── Go Drift Engine ──"
cd ~/infraguard/services/drift-engine
GO_BUILD=$(go build ./... 2>&1 && echo true || echo false)
check "Go build succeeds" "$GO_BUILD"
cd ~/infraguard

echo ""; echo "── Go Remediation Engine ──"
cd ~/infraguard/services/remediation
REM_BUILD=$(go build ./... 2>&1 && echo true || echo false)
check "Remediation engine build succeeds" "$REM_BUILD"
cd ~/infraguard

fuser -k 8082/tcp 2>/dev/null || true
sleep 1
nohup /tmp/remediation-test > /tmp/remediation-test.log 2>&1 &
RPID=$!
disown
REM_HEALTH=false
for i in $(seq 1 10); do
  curl -s http://localhost:8082/healthz 2>/dev/null | grep -q "ok" && REM_HEALTH=true && break
  sleep 1
done
check "Remediation engine /healthz" "$REM_HEALTH"
kill $RPID 2>/dev/null

echo ""; echo "── MLflow ──"
MLFLOW_OK=$(curl -s http://localhost:5000/health 2>/dev/null | grep -q "OK" && echo true || echo false)
check "MLflow (Docker) responding" "$MLFLOW_OK"
MODEL_FILE=$([ -f /tmp/infraguard_model.pkl ] && echo true || echo false)
check "ML model file exists" "$MODEL_FILE"

echo ""; echo "── ML Predictor API ──"
fuser -k 8001/tcp 2>/dev/null || true
sleep 2
cd ~/infraguard/services/ml-predictor
source venv/bin/activate
nohup python3 -m uvicorn main:app --app-dir src --host 0.0.0.0 --port 8001 > /tmp/ml-test.log 2>&1 &
MLPID=$!
disown
ML_HEALTH=false
for i in $(seq 1 15); do
  curl -s http://localhost:8001/healthz 2>/dev/null | grep -q "ok" && ML_HEALTH=true && break
  sleep 1
done
check "ML predictor /healthz" "$ML_HEALTH"
ML_READY=$(curl -s http://localhost:8001/readyz 2>/dev/null | grep -q "ready" && echo true || echo false)
check "ML predictor /readyz (model loaded)" "$ML_READY"
kill $MLPID 2>/dev/null
cd ~/infraguard

echo ""; echo "── React Dashboard ──"
DASH_OK=$(curl -s http://localhost:3000 2>/dev/null | grep -qi "<title>" && echo true || echo false)
check "Dashboard responding" "$DASH_OK"

echo ""
echo "════════════════════════════════════════════"
echo "  RESULTS: $PASS passed, $FAIL failed"
echo "════════════════════════════════════════════"
