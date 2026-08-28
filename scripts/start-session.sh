#!/usr/bin/env bash
set -e
echo "=== InfraGuard Session Startup ==="
cd ~/infraguard

echo "[1/13] Starting Docker services..."
docker compose up -d
for i in $(seq 1 20); do
  curl -s http://localhost:4566/_localstack/health 2>/dev/null | grep -q '"ec2"' && break
  sleep 5
done
sleep 5

echo "[2/13] Starting local Docker registry (for Cosign signing)..."
if ! docker ps --format '{{.Names}}' | grep -q '^local-registry$'; then
  docker run -d -p 5001:5000 --restart=always --name local-registry registry:2 2>/dev/null || \
    docker start local-registry 2>/dev/null || true
fi

echo "[3/13] Starting SonarQube (may take 1-2 min to become ready)..."
docker compose up -d sonarqube

echo "[4/13] Restoring Vault secrets..."
export VAULT_ADDR=http://localhost:8200
export VAULT_TOKEN=root
bash scripts/vault-init.sh

echo "[5/13] Restoring IaC baseline..."
cd infra
TF_VAR_db_password=infraguard_dev tofu apply -auto-approve
cd ~/infraguard

echo "[6/13] Rebuilding K8s namespaces/RBAC if needed..."
if ! kubectl get namespace drift-engine &>/dev/null; then
  kubectl create namespace drift-engine
  kubectl create namespace compliance
  kubectl create namespace remediation
  kubectl create namespace dashboard
  kubectl create namespace observability
  kubectl create serviceaccount vault-reviewer -n default
  kubectl create clusterrolebinding vault-reviewer --clusterrole=system:auth-delegator --serviceaccount=default:vault-reviewer
  for ns in drift-engine compliance remediation dashboard; do
    kubectl create serviceaccount infraguard-sa -n $ns
  done
  kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
fi

echo "[7/13] Rebuilding ArgoCD if needed, then logging in..."
if ! kubectl get namespace argocd &>/dev/null; then
  kubectl create namespace argocd
  kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml --server-side --force-conflicts
  kubectl wait --namespace argocd --for=condition=ready pod \
    --selector=app.kubernetes.io/name=argocd-server --timeout=180s || true
fi
pkill -f "port-forward svc/argocd-server" 2>/dev/null || true
sleep 1
nohup kubectl port-forward svc/argocd-server -n argocd 8081:443 --address 0.0.0.0 > /tmp/argocd-portforward.log 2>&1 &
sleep 5
ARGOCD_PWD=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" 2>/dev/null | base64 -d)
if [ -n "$ARGOCD_PWD" ]; then
  argocd login localhost:8081 --username admin --password "$ARGOCD_PWD" --insecure 2>&1 | grep -v "^$" || true
fi
if ! argocd app get infraguard-platform &>/dev/null; then
  echo "  Recreating ArgoCD application..."
  argocd app create infraguard-platform \
    --repo https://github.com/Haseef-Ahamed/infraguard.git \
    --path charts/infraguard \
    --dest-server https://kubernetes.default.svc \
    --dest-namespace default \
    --sync-policy automated \
    --self-heal 2>&1 || true
fi

echo "[8/13] Starting standalone drift-engine on host (for Prometheus scraping)..."
fuser -k 8080/tcp 2>/dev/null || true
pkill -f drift-engine-test 2>/dev/null || true
sleep 2
cd services/drift-engine
go build -o /tmp/drift-engine-test ./cmd/agent/
nohup /tmp/drift-engine-test > /tmp/drift-engine-host.log 2>&1 &
disown
cd ~/infraguard
sleep 5
curl -s http://localhost:8080/healthz && echo " Drift engine (host) OK"

echo "[9/13] Deploying drift-engine + remediation via Helm..."
HOST_IP=$(docker inspect infraguard-worker --format '{{range .NetworkSettings.Networks}}{{.Gateway}}{{end}}' | head -1)
kind load docker-image infraguard/drift-engine:latest --name infraguard 2>/dev/null || true
kind load docker-image infraguard/remediation:latest --name infraguard 2>/dev/null || true

cd charts
if helm list | grep -q '^infraguard\s'; then
  echo "  Existing release found — upgrading..."
  helm upgrade infraguard ./infraguard \
    --set global.vaultAddr="http://${HOST_IP}:8200" \
    --set remediation.natsUrl="nats://${HOST_IP}:4222"
else
  echo "  No existing release — installing..."
  helm install infraguard ./infraguard \
    --set global.vaultAddr="http://${HOST_IP}:8200" \
    --set remediation.natsUrl="nats://${HOST_IP}:4222"
fi
cd ~/infraguard
kubectl rollout status daemonset/drift-engine -n drift-engine --timeout=60s || true
kubectl rollout status deployment/remediation-engine -n remediation --timeout=60s || true

echo "[10/13] Regenerating ML pipeline..."
cd services/ml-predictor
source venv/bin/activate
python3 src/generate_data.py
python3 src/preprocess.py
python3 src/train.py
python3 src/promote_model.py
deactivate
cd ~/infraguard

echo "[11/13] Starting ML inference server..."
fuser -k 8001/tcp 2>/dev/null || true
sleep 1
cd services/ml-predictor
source venv/bin/activate
nohup python3 -m uvicorn main:app --app-dir src --host 0.0.0.0 --port 8001 > /tmp/ml-server.log 2>&1 &
deactivate
cd ~/infraguard
sleep 5
curl -s http://localhost:8001/healthz && echo " ML server OK"

echo "[12/13] Starting standalone Remediation Engine on host (Slack/PagerDuty/GitHub from Vault)..."
fuser -k 8082/tcp 2>/dev/null || true
pkill -f remediation-test 2>/dev/null || true
sleep 2
GH_TOKEN=$(vault kv get -field=token infraguard/github 2>/dev/null || echo "")
GH_OWNER=$(vault kv get -field=owner infraguard/github 2>/dev/null || echo "")
SLACK_WEBHOOK=$(vault kv get -field=webhook_url infraguard/slack 2>/dev/null || echo "")
SLACK_CHAN=$(vault kv get -field=channel infraguard/slack 2>/dev/null || echo "#ops-alerts")
PD_KEY=$(vault kv get -field=routing_key infraguard/pagerduty 2>/dev/null || echo "")

cd services/remediation
go build -o /tmp/remediation-test ./cmd/server/
NATS_URL=nats://localhost:4222 \
GITHUB_TOKEN="$GH_TOKEN" GITHUB_OWNER="$GH_OWNER" GITHUB_REPO=infraguard \
SLACK_WEBHOOK_URL="$SLACK_WEBHOOK" SLACK_CHANNEL="$SLACK_CHAN" \
PAGERDUTY_ROUTING_KEY="$PD_KEY" \
nohup /tmp/remediation-test > /tmp/remediation.log 2>&1 &
cd ~/infraguard
sleep 3
curl -s http://localhost:8082/healthz && echo " Remediation engine (host) OK"

echo "[13/13] Starting React Dashboard..."
fuser -k 3000/tcp 2>/dev/null || true
sleep 1
cd dashboard
if [ ! -f package.json ] || ! grep -q '"start"' package.json 2>/dev/null; then
  echo "  Dashboard package.json missing/broken — skipping."
else
  nohup npm start > /tmp/dashboard.log 2>&1 &
  cd ~/infraguard
  for i in $(seq 1 30); do
    curl -s http://localhost:3000 2>/dev/null | grep -qi "<title>" && echo "  Dashboard OK" && break
    sleep 2
  done
fi
cd ~/infraguard

# Firewall — idempotent
sudo ufw allow 3000/tcp 2>/dev/null || true
sudo ufw allow 8080/tcp 2>/dev/null || true
sudo ufw allow 8081/tcp 2>/dev/null || true
sudo ufw allow 8082/tcp 2>/dev/null || true
sudo ufw allow 9000/tcp 2>/dev/null || true

VM_IP=$(hostname -I | awk '{print $1}')
echo ""
echo "=== Startup complete ==="
echo "Drift Dashboard:  http://${VM_IP}:3000"
echo "ArgoCD UI:        https://${VM_IP}:8081  (admin / password below)"
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' 2>/dev/null | base64 -d
echo ""
echo "ML server:        http://localhost:8001/docs"
echo "MLflow:            http://localhost:5000"
echo "Grafana:           http://${VM_IP}:3001  (admin/admin)"
echo "Prometheus:        http://localhost:9090"
echo "SonarQube:         http://${VM_IP}:9000  (admin / see: vault kv get -field=admin_password infraguard/sonarqube)"
echo "Local Registry:    localhost:5001"
echo "Helm release:"
helm list
cd ~/infraguard
