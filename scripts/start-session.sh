#!/usr/bin/env bash
set -e
echo "=== InfraGuard Session Startup ==="
cd ~/infraguard

# 1. Start Docker services
echo "[1/6] Starting Docker services..."
docker compose up -d
echo "Waiting for services..."
for i in $(seq 1 20); do
  if curl -s http://localhost:4566/_localstack/health 2>/dev/null | grep -q '"ec2"'; then
    echo "LocalStack ready"; break
  fi
  sleep 5
done
sleep 5

# 2. Restore Vault secrets
echo "[2/6] Restoring Vault secrets..."
export VAULT_ADDR=http://localhost:8200
export VAULT_TOKEN=root
~/infraguard/scripts/vault-init.sh

# 3. Restore IaC baseline
echo "[3/6] Restoring IaC baseline..."
cd ~/infraguard/infra
TF_VAR_db_password=infraguard_dev tofu apply -auto-approve
cd ~/infraguard

# 4. Regenerate ML training data (lost on reboot)
echo "[4/6] Regenerating ML data..."
cd ~/infraguard/services/ml-predictor
source venv/bin/activate
python3 src/generate_data.py
python3 src/preprocess.py

# 5. Retrain model (lost on reboot)
echo "[5/6] Retraining model..."
python3 src/train.py
python3 src/promote_model.py

# 6. Start ML inference server
echo "[6/6] Starting ML inference server..."
fuser -k 8001/tcp 2>/dev/null || true
nohup python3 -m uvicorn main:app \
  --app-dir src \
  --host 0.0.0.0 \
  --port 8001 \
  --log-level info > /tmp/ml-server.log 2>&1 &
echo $! > /tmp/ml-server.pid
sleep 3
curl -s http://localhost:8001/healthz && echo " ML server OK" || echo " ML server failed - check /tmp/ml-server.log"

echo ""
echo "=== Startup complete ==="
echo "Services:  docker compose ps"
echo "ML server: http://localhost:8001/docs"
echo "MLflow:    http://localhost:5000"
echo "Grafana:   http://localhost:3001"
cd ~/infraguard

# Rebuild K8s namespaces/RBAC if cluster was recreated
if ! kubectl get namespace drift-engine &>/dev/null; then
  echo "Rebuilding Kubernetes namespaces and RBAC..."
  kubectl create namespace drift-engine
  kubectl create namespace compliance
  kubectl create namespace remediation
  kubectl create namespace dashboard
  kubectl create namespace observability
  kubectl create namespace argocd
  kubectl create serviceaccount vault-reviewer -n default
  kubectl create clusterrolebinding vault-reviewer \
    --clusterrole=system:auth-delegator \
    --serviceaccount=default:vault-reviewer
  for ns in drift-engine compliance remediation dashboard; do
    kubectl create serviceaccount infraguard-sa -n $ns
  done
  kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
fi

HOST_IP=$(docker inspect infraguard-worker --format '{{range .NetworkSettings.Networks}}{{.Gateway}}{{end}}' | head -1)
kubectl apply -f ~/infraguard/charts/infraguard/templates/drift-engine-rbac.yaml
kubectl apply -f ~/infraguard/charts/infraguard/templates/drift-engine-config.yaml
kind load docker-image infraguard/drift-engine:latest --name infraguard 2>/dev/null || true
kubectl apply -f ~/infraguard/charts/infraguard/templates/drift-engine-daemonset.yaml
kubectl apply -f ~/infraguard/charts/infraguard/templates/drift-engine-service.yaml
kubectl rollout status daemonset/drift-engine -n drift-engine --timeout=60s || true
