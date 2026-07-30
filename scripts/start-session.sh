#!/usr/bin/env bash
set -e
echo "=== InfraGuard Session Startup ==="
cd ~/infraguard

echo "[1/10] Starting Docker services..."
docker compose up -d
for i in $(seq 1 20); do
  curl -s http://localhost:4566/_localstack/health 2>/dev/null | grep -q '"ec2"' && break
  sleep 5
done
sleep 5

echo "[2/10] Restoring Vault secrets..."
export VAULT_ADDR=http://localhost:8200
export VAULT_TOKEN=root
bash scripts/vault-init.sh

echo "[3/10] Restoring IaC baseline..."
cd infra
TF_VAR_db_password=infraguard_dev tofu apply -auto-approve
cd ~/infraguard

echo "[4/10] Rebuilding K8s namespaces/RBAC if needed..."
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

echo "[5/10] Rebuilding ArgoCD if needed, then logging in..."
if ! kubectl get namespace argocd &>/dev/null; then
  kubectl create namespace argocd
  kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml --server-side --force-conflicts
  kubectl wait --namespace argocd --for=condition=ready pod \
    --selector=app.kubernetes.io/name=argocd-server --timeout=180s || true
fi

# Port-forward ArgoCD (idempotent — kill old one first)
pkill -f "port-forward svc/argocd-server" 2>/dev/null || true
sleep 1
nohup kubectl port-forward svc/argocd-server -n argocd 8081:443 --address 0.0.0.0 > /tmp/argocd-portforward.log 2>&1 &
sleep 5

# Auto-login to ArgoCD CLI
ARGOCD_PWD=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" 2>/dev/null | base64 -d)
if [ -n "$ARGOCD_PWD" ]; then
  argocd login localhost:8081 --username admin --password "$ARGOCD_PWD" --insecure 2>&1 | grep -v "^$" || true
fi

# Recreate the ArgoCD application if it's missing (e.g. cluster was recreated)
if ! argocd app get infraguard-platform &>/dev/null; then
  echo "  Recreating ArgoCD application..."
  argocd app create infraguard-platform \
    --repo https://github.com/Haseef-Ahamed/infraguard.git \
    --path charts/infraguard \
    --dest-server https://kubernetes.default.svc \
    --dest-namespace default \
    --sync-policy automated \
    --self-heal 2>&1 || echo "  (app creation skipped — may already exist or repo unreachable)"
fi

echo "[6/10] Redeploying drift-engine DaemonSet..."
HOST_IP=$(docker inspect infraguard-worker --format '{{range .NetworkSettings.Networks}}{{.Gateway}}{{end}}' | head -1)
kubectl apply -f charts/infraguard/templates/drift-engine-rbac.yaml
kubectl apply -f charts/infraguard/templates/drift-engine-config.yaml
kind load docker-image infraguard/drift-engine:latest --name infraguard 2>/dev/null || true
kubectl apply -f charts/infraguard/templates/drift-engine-daemonset.yaml
kubectl rollout status daemonset/drift-engine -n drift-engine --timeout=60s || true

echo "[7/10] Regenerating ML pipeline..."
cd services/ml-predictor
source venv/bin/activate
python3 src/generate_data.py
python3 src/preprocess.py
python3 src/train.py
python3 src/promote_model.py
deactivate
cd ~/infraguard

echo "[8/10] Starting ML inference server..."
fuser -k 8001/tcp 2>/dev/null || true
sleep 1
cd services/ml-predictor
source venv/bin/activate
nohup python3 -m uvicorn main:app --app-dir src --host 0.0.0.0 --port 8001 > /tmp/ml-server.log 2>&1 &
deactivate
cd ~/infraguard
sleep 5
curl -s http://localhost:8001/healthz && echo " ML server OK"

echo "[9/10] Starting Remediation Engine (with Slack/PagerDuty if configured)..."
fuser -k 8082/tcp 2>/dev/null || true
sleep 1
GH_TOKEN=$(vault kv get -field=token infraguard/github 2>/dev/null || echo "")
GH_OWNER=$(vault kv get -field=owner infraguard/github 2>/dev/null || echo "")
SLACK_WEBHOOK=$(vault kv get -field=webhook_url infraguard/slack 2>/dev/null || echo "")
SLACK_CHAN=$(vault kv get -field=channel infraguard/slack 2>/dev/null || echo "#ops-alerts")
PD_KEY=$(vault kv get -field=routing_key infraguard/pagerduty 2>/dev/null || echo "")

cd services/remediation
go build -o /tmp/remediation-test ./cmd/server/
NATS_URL=nats://localhost:4222 \
GITHUB_TOKEN="$GH_TOKEN" \
GITHUB_OWNER="$GH_OWNER" \
GITHUB_REPO=infraguard \
SLACK_WEBHOOK_URL="$SLACK_WEBHOOK" \
SLACK_CHANNEL="$SLACK_CHAN" \
PAGERDUTY_ROUTING_KEY="$PD_KEY" \
nohup /tmp/remediation-test > /tmp/remediation.log 2>&1 &
cd ~/infraguard
sleep 3
curl -s http://localhost:8082/healthz && echo " Remediation engine OK"

echo "[10/10] Starting React Dashboard..."
fuser -k 3000/tcp 2>/dev/null || true
sleep 1
cd dashboard
if [ ! -f package.json ] || ! grep -q '"start"' package.json 2>/dev/null; then
  echo "  Dashboard package.json missing/broken — skipping start. Run create-react-app manually."
else
  nohup npm start > /tmp/dashboard.log 2>&1 &
  cd ~/infraguard
  echo "  Waiting for dashboard to compile..."
  for i in $(seq 1 30); do
    if curl -s http://localhost:3000 2>/dev/null | grep -qi "<title>"; then
      echo "  Dashboard OK"
      break
    fi
    sleep 2
  done
fi
cd ~/infraguard

# Open firewall for browser access from Windows host (idempotent)
sudo ufw allow 3000/tcp 2>/dev/null || true
sudo ufw allow 8081/tcp 2>/dev/null || true
sudo ufw allow 8082/tcp 2>/dev/null || true

VM_IP=$(hostname -I | awk '{print $1}')
echo ""
echo "=== Startup complete ==="
echo "Drift Dashboard:  http://${VM_IP}:3000"
echo "ArgoCD UI:        https://${VM_IP}:8081  (admin / password below)"
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' 2>/dev/null | base64 -d
echo ""
echo "ML server:        http://localhost:8001/docs"
echo "MLflow:            http://localhost:5000"
echo "Grafana:           http://localhost:3001"
cd ~/infraguard
