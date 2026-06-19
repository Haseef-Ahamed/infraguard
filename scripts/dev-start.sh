#!/usr/bin/env bash
set -e
echo "Starting InfraGuard dev environment..."

cd ~/infraguard

# Start Docker services
docker compose up -d

# Wait for LocalStack to be fully ready
echo "Waiting for LocalStack to be healthy..."
until curl -s http://localhost:4566/_localstack/health | grep -qE '"ec2": "(running|available)"'; do
  echo "  LocalStack not ready yet, waiting 5s..."
  sleep 5
done
echo "LocalStack ready"

# Wait for Vault
echo "Waiting for Vault..."
until curl -s http://localhost:8200/v1/sys/health | grep -qE '"initialized":true'; do
  sleep 3
done
echo "Vault ready"

# Restore Vault secrets
export VAULT_ADDR=http://localhost:8200
export VAULT_TOKEN=root
~/infraguard/scripts/vault-init.sh

# Restore LocalStack IaC baseline
cd ~/infraguard/infra
TF_VAR_db_password=infraguard_dev tofu apply -auto-approve
cd ~/infraguard

echo "Dev environment ready."
