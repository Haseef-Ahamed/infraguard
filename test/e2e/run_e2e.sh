#!/usr/bin/env bash
set -e
echo "════════════════════════════════════════════"
echo "  InfraGuard — Full E2E Test Suite"
echo "════════════════════════════════════════════"

cd ~/infraguard/test/e2e

echo ""
echo "[1/3] Infrastructure baseline tests..."
go test -v -run TestInfraBaseline -timeout 60s ./...

echo ""
echo "[2/3] Compliance classification tests..."
go test -v -run TestCompliance -timeout 30s ./...

echo ""
echo "[3/3] Drift detection E2E test (takes ~2 minutes)..."
go test -v -run TestEndToEnd -timeout 150s ./...

echo ""
echo "════════════════════════════════════════════"
echo "  ALL E2E TESTS PASSED ✓"
echo "════════════════════════════════════════════"
