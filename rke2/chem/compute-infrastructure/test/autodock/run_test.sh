#!/usr/bin/env bash
# run_test.sh — end-to-end test: generate 1000 mock vina results, export to MySQL, verify.
#
# Requires: Docker + Docker Compose v2 (or docker-compose v1).
#
# Usage:
#   ./run_test.sh          # run test, clean up on success
#   ./run_test.sh --keep   # keep containers running after test (for inspection)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

KEEP=false
[[ "${1:-}" == "--keep" ]] && KEEP=true

# Support both 'docker compose' (v2) and 'docker-compose' (v1)
if docker compose version &>/dev/null 2>&1; then
    DC="docker compose"
else
    DC="docker-compose"
fi

cleanup() {
    if [[ "$KEEP" == "false" ]]; then
        echo ""
        echo "Cleaning up containers and volumes..."
        $DC down --remove-orphans --volumes 2>/dev/null || true
    else
        echo ""
        echo "Containers kept running (--keep). Run '$DC down' to clean up."
    fi
}
trap cleanup EXIT

echo "========================================"
echo " AutoDock Vina MySQL Export Test"
echo "========================================"
echo ""

# Tear down any leftover state from a previous run
$DC down --remove-orphans --volumes 2>/dev/null || true

echo "Starting MySQL and test runner..."
echo ""

if $DC up \
    --build \
    --exit-code-from test-runner \
    --abort-on-container-exit \
    2>&1; then
    echo ""
    echo "========================================"
    echo " TEST PASSED: 1000 ligand energies"
    echo " exported to MySQL successfully."
    echo "========================================"
    exit 0
else
    EXIT_CODE=$?
    echo ""
    echo "========================================"
    echo " TEST FAILED (exit code: $EXIT_CODE)"
    echo "========================================"
    echo ""
    echo "--- test-runner logs ---"
    $DC logs test-runner 2>/dev/null || true
    exit $EXIT_CODE
fi
