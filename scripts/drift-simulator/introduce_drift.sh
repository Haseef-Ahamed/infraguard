#!/usr/bin/env bash
# InfraGuard Drift Simulator
# Introduces controlled drift into LocalStack for testing
# Usage: ./introduce_drift.sh <drift-type>
# Types: sg-port | sg-ssh | s3-public | revert-all | status

set -euo pipefail

ENDPOINT="http://localhost:4566"
PROFILE="localstack"
AWS="aws --endpoint-url=${ENDPOINT} --profile=${PROFILE}"
BOLD="\033[1m"
RED="\033[0;31m"
GREEN="\033[0;32m"
YELLOW="\033[1;33m"
RESET="\033[0m"

get_sg_id() {
  ${AWS} ec2 describe-security-groups \
    --filters "Name=group-name,Values=infraguard-app-sg" \
    --query "SecurityGroups[0].GroupId" \
    --output text 2>/dev/null || echo ""
}

case "${1:-help}" in

  sg-port)
    SG_ID=$(get_sg_id)
    if [ -z "$SG_ID" ] || [ "$SG_ID" = "None" ]; then
      echo -e "${RED}[ERROR]${RESET} Security group 'infraguard-app-sg' not found."
      echo "Run: cd ~/infraguard/infra && TF_VAR_db_password=infraguard_dev tofu apply -auto-approve"
      exit 1
    fi
    echo -e "${YELLOW}[DRIFT]${RESET} Opening port 5432 (PostgreSQL) to 0.0.0.0/0 on ${SG_ID}"
    ${AWS} ec2 authorize-security-group-ingress \
      --group-id "${SG_ID}" \
      --protocol tcp \
      --port 5432 \
      --cidr 0.0.0.0/0
    echo -e "${RED}[DRIFT INTRODUCED]${RESET} Port 5432 now open."
    echo -e "Expected violations: ${BOLD}CIS_5.4${RESET} + ${BOLD}SOC2_CC6.1${RESET} + ${BOLD}HIPAA_164.312e2ii${RESET}"
    echo "Detection expected within 60 seconds."
    ;;

  sg-ssh)
    SG_ID=$(get_sg_id)
    echo -e "${YELLOW}[DRIFT]${RESET} Opening port 22 (SSH) to 0.0.0.0/0 on ${SG_ID}"
    ${AWS} ec2 authorize-security-group-ingress \
      --group-id "${SG_ID}" \
      --protocol tcp \
      --port 22 \
      --cidr 0.0.0.0/0
    echo -e "${RED}[DRIFT INTRODUCED]${RESET} Port 22 (SSH) now open."
    echo "Expected violation: CIS_5.2"
    ;;

  s3-public)
    echo -e "${YELLOW}[DRIFT]${RESET} Removing S3 public access block from infraguard-artifacts-dev"
    ${AWS} s3api delete-public-access-block \
      --bucket infraguard-artifacts-dev
    echo -e "${RED}[DRIFT INTRODUCED]${RESET} S3 public access block removed."
    echo "Expected violations: CIS_2.1.1 + CIS_2.1.2"
    ;;

  status)
    SG_ID=$(get_sg_id)
    echo -e "${BOLD}=== Current Security Group State ===${RESET}"
    echo "SG ID: ${SG_ID}"
    ${AWS} ec2 describe-security-groups \
      --group-ids "${SG_ID}" \
      --query "SecurityGroups[0].IpPermissions" \
      --output table 2>/dev/null || echo "Could not fetch SG state"
    ;;

  revert-all)
    SG_ID=$(get_sg_id)
    echo -e "${GREEN}[REVERT]${RESET} Reverting all drift..."

    # Remove port 5432
    ${AWS} ec2 revoke-security-group-ingress \
      --group-id "${SG_ID}" \
      --protocol tcp --port 5432 --cidr 0.0.0.0/0 2>/dev/null \
      && echo "  Removed port 5432" || echo "  Port 5432 was not open"

    # Remove port 22
    ${AWS} ec2 revoke-security-group-ingress \
      --group-id "${SG_ID}" \
      --protocol tcp --port 22 --cidr 0.0.0.0/0 2>/dev/null \
      && echo "  Removed port 22" || echo "  Port 22 was not open"

    echo -e "${GREEN}[REVERTED]${RESET} All drift removed."
    ;;

  help|*)
    echo "InfraGuard Drift Simulator"
    echo ""
    echo "Usage: $0 <command>"
    echo ""
    echo "Commands:"
    echo "  sg-port     Open port 5432 (PostgreSQL) to internet — triggers CIS_5.4"
    echo "  sg-ssh      Open port 22 (SSH) to internet — triggers CIS_5.2"
    echo "  s3-public   Remove S3 public access block — triggers CIS_2.1.1"
    echo "  status      Show current security group ingress rules"
    echo "  revert-all  Revert all introduced drift"
    ;;
esac
