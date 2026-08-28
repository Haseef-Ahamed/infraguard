#!/usr/bin/env bash
echo "════════════════════════════════════════════"
echo "  InfraGuard — Full System Verification"
echo "  (Phases 1-5)"
echo "════════════════════════════════════════════"
PASS=0; FAIL=0
check() { if [ "$2" = "true" ]; then echo "  [PASS] $1"; PASS=$((PASS+1)); else echo "  [FAIL] $1"; FAIL=$((FAIL+1)); fi; }

echo ""; echo "── Docker Services ──"
docker compose ps --format "{{.Name}}: {{.Status}}"
RUNNING=$(docker compose ps --format "{{.Status}}" | grep -c "Up\|running\|healthy")
check "9 Docker services running (incl. SonarQube)" "$([ "$RUNNING" -ge 9 ] && echo true || echo false)"

echo ""; echo "── LocalStack + IaC Baseline ──"
LS_OK=$(curl -s http://localhost:4566/_localstack/health 2>/dev/null | grep -q '"ec2"' && echo true || echo false)
check "LocalStack responding" "$LS_OK"
SG_PORT=$(aws --endpoint-url=http://localhost:4566 --profile localstack ec2 describe-security-groups --query "SecurityGroups[?GroupName=='infraguard-app-sg'].IpPermissions[0].FromPort" --output text 2>/dev/null)
check "Security group baseline (port 443)" "$([ "$SG_PORT" = "443" ] && echo true || echo false)"

echo ""; echo "── PostgreSQL / TimescaleDB ──"
PG_OK=$(docker compose exec -T postgres pg_isready -U infraguard -q 2>/dev/null && echo true || echo false)
check "PostgreSQL accepting connections" "$PG_OK"
TS_COUNT=$(docker compose exec -T timescaledb psql -U infraguard -d infraguard_ts -c "SELECT count(*) FROM telemetry" -t 2>/dev/null | tr -d ' ')
check "Telemetry data exists" "$([ -n "$TS_COUNT" ] && [ "$TS_COUNT" -gt 0 ] && echo true || echo false)"

echo ""; echo "── Vault (7 secrets) ──"
export VAULT_ADDR=http://localhost:8200
export VAULT_TOKEN=root
SECRET_COUNT=$(vault kv list infraguard/ 2>/dev/null | grep -v -E "^Keys|^----" | grep -c . || echo 0)
check "6+ Vault secrets stored" "$([ "$SECRET_COUNT" -ge 6 ] && echo true || echo false)"

echo ""; echo "── kind Cluster + ArgoCD ──"
NODES_READY=$(kubectl get nodes --no-headers 2>/dev/null | grep -c Ready)
check "3 nodes Ready" "$([ "$NODES_READY" = "3" ] && echo true || echo false)"
ARGOCD_PODS=$(kubectl get pods -n argocd --no-headers 2>/dev/null | grep -c "1/1\|2/2")
check "ArgoCD pods running" "$([ "$ARGOCD_PODS" -ge 5 ] && echo true || echo false)"

echo ""; echo "── Helm Release ──"
HELM_STATUS=$(helm list 2>/dev/null | grep -c "^infraguard\s")
check "Helm release 'infraguard' deployed" "$([ "$HELM_STATUS" -ge 1 ] && echo true || echo false)"
kubectl get pods -n drift-engine 2>/dev/null
kubectl get pods -n remediation 2>/dev/null

echo ""; echo "── Go Services (build + host health) ──"
cd ~/infraguard/services/drift-engine
DE_BUILD=$(go build ./... 2>&1 && echo true || echo false)
check "Drift-engine build" "$DE_BUILD"
cd ~/infraguard/services/remediation
REM_BUILD=$(go build ./... 2>&1 && echo true || echo false)
check "Remediation-engine build" "$REM_BUILD"
cd ~/infraguard
DE_HEALTH=$(curl -s http://localhost:8080/healthz 2>/dev/null | grep -q "ok" && echo true || echo false)
check "Drift engine (host) /healthz" "$DE_HEALTH"
REM_HEALTH=$(curl -s http://localhost:8082/healthz 2>/dev/null | grep -q "ok" && echo true || echo false)
check "Remediation engine (host) /healthz" "$REM_HEALTH"

echo ""; echo "── ML Predictor ──"
MODEL_FILE=$([ -f /tmp/infraguard_model.pkl ] && echo true || echo false)
check "ML model file exists" "$MODEL_FILE"
ML_HEALTH=$(curl -s http://localhost:8001/healthz 2>/dev/null | grep -q "ok" && echo true || echo false)
check "ML predictor /healthz" "$ML_HEALTH"

echo ""; echo "── Dashboard / Observability ──"
DASH_OK=$(curl -s http://localhost:3000 2>/dev/null | grep -qi "<title>" && echo true || echo false)
check "React dashboard responding" "$DASH_OK"
PROM_TARGETS_UP=$(curl -s http://localhost:9090/api/v1/targets 2>/dev/null | python3 -c "
import json,sys
try:
    d=json.load(sys.stdin)
    print(sum(1 for t in d['data']['activeTargets'] if t['health']=='up'))
except: print(0)
" 2>/dev/null)
check "Prometheus targets up" "$([ "$PROM_TARGETS_UP" -ge 2 ] && echo true || echo false)"
GRAFANA_DASH=$(curl -s -u admin:admin http://localhost:3001/api/search?query=InfraGuard 2>/dev/null | grep -c "InfraGuard")
check "Grafana dashboard provisioned" "$([ "$GRAFANA_DASH" -ge 1 ] && echo true || echo false)"

echo ""; echo "── Supply Chain Security ──"
SBOM_FILES=$(ls ~/infraguard/sbom/*.json 2>/dev/null | wc -l)
check "SBOM files generated" "$([ "$SBOM_FILES" -ge 1 ] && echo true || echo false)"
PRECOMMIT_HOOK=$([ -x ~/infraguard/.git/hooks/pre-commit ] && echo true || echo false)
check "Gitleaks pre-commit hook active" "$PRECOMMIT_HOOK"

echo ""; echo "── SonarQube ──"
SONAR_UP=$(curl -s http://localhost:9000/api/system/status 2>/dev/null | grep -c '"status":"UP"')
check "SonarQube server up" "$([ "$SONAR_UP" -ge 1 ] && echo true || echo false)"

echo ""; echo "── Helm Chart ──"
cd ~/infraguard/charts
HELM_LINT=$(helm lint infraguard/ 2>&1 | grep -c "0 chart(s) failed")
check "Helm chart lints cleanly" "$([ "$HELM_LINT" -ge 1 ] && echo true || echo false)"
cd ~/infraguard

echo ""; echo "── E2E Test Suite ──"
cd ~/infraguard/test/e2e
E2E_RESULT=$(go test -run "TestInfraBaseline|TestCompliance" -timeout 60s ./... 2>&1 | tail -1)
check "E2E baseline + compliance tests pass" "$(echo "$E2E_RESULT" | grep -q "^ok" && echo true || echo false)"
cd ~/infraguard

echo ""
echo "════════════════════════════════════════════"
echo "  RESULTS: $PASS passed, $FAIL failed"
echo "════════════════════════════════════════════"
