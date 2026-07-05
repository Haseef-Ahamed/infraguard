#!/usr/bin/env bash
set -e
export VAULT_ADDR=http://localhost:8200
export VAULT_TOKEN=root

# Detect Docker host IP for pod-to-host connectivity
HOST_IP=$(docker inspect infraguard-worker \
  --format '{{range .NetworkSettings.Networks}}{{.Gateway}}{{end}}' 2>/dev/null \
  | head -1 || echo "172.19.0.1")

echo "Initialising Vault secrets (host IP: ${HOST_IP})..."

vault secrets enable -path=infraguard kv-v2 2>/dev/null || echo "KV already enabled"

vault kv put infraguard/aws \
  access_key=test secret_key=test \
  region=us-east-1 endpoint=http://${HOST_IP}:4566

vault kv put infraguard/postgres \
  host=${HOST_IP} port=5432 \
  database=infraguard username=infraguard password=infraguard_dev

vault kv put infraguard/timescaledb \
  host=${HOST_IP} port=5433 \
  database=infraguard_ts username=infraguard password=infraguard_dev

vault kv put infraguard/nats \
  url=nats://${HOST_IP}:4222

vault kv put infraguard/slack \
  webhook_url=https://hooks.slack.com/services/PLACEHOLDER \
  channel='#ops-alerts'

echo "Setting up Kubernetes auth..."
vault auth enable kubernetes 2>/dev/null || echo "K8s auth already enabled"

K8S_HOST=$(kubectl config view --raw \
  -o jsonpath='{.clusters[?(@.name=="kind-infraguard")].cluster.server}')
K8S_CA=$(kubectl config view --raw \
  -o jsonpath='{.clusters[?(@.name=="kind-infraguard")].cluster.certificate-authority-data}' \
  | base64 -d)
SA_TOKEN=$(kubectl create token vault-reviewer -n default --duration=8760h 2>/dev/null || \
  kubectl get secret vault-reviewer-token -n default \
  -o jsonpath='{.data.token}' | base64 -d)

vault write auth/kubernetes/config \
  kubernetes_host="${K8S_HOST}" \
  kubernetes_ca_cert="${K8S_CA}" \
  token_reviewer_jwt="${SA_TOKEN}"

vault policy write infraguard-read - << 'EOF'
path "infraguard/data/*" { capabilities = ["read","list"] }
path "infraguard/metadata/*" { capabilities = ["read","list"] }
EOF

vault write auth/kubernetes/role/infraguard \
  bound_service_account_names=infraguard-sa \
  bound_service_account_namespaces=drift-engine,compliance,remediation,dashboard \
  policies=infraguard-read \
  ttl=1h

# Add GitHub secret if token exists
if [ -f ~/infraguard/.github-token ]; then
  GH_TOKEN=$(cat ~/infraguard/.github-token)
  vault kv put infraguard/github \
    token="${GH_TOKEN}" \
    owner=Haseef-Ahamed \
    repo=infraguard
fi

echo "Vault init complete."
