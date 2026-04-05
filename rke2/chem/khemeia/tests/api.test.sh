#!/bin/bash
# API Integration Tests for Khemeia
# Runs against the live cluster API
# Usage: ./api.test.sh [API_URL]

set -euo pipefail

API="${1:-https://khemeia.hwcopeland.net}"
PASS=0
FAIL=0
TOTAL=0

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

assert() {
  local name="$1"
  local expected="$2"
  local actual="$3"
  TOTAL=$((TOTAL + 1))
  if [ "$actual" = "$expected" ]; then
    echo -e "  ${GREEN}✓${NC} $name"
    PASS=$((PASS + 1))
  else
    echo -e "  ${RED}✗${NC} $name (expected: $expected, got: $actual)"
    FAIL=$((FAIL + 1))
  fi
}

assert_contains() {
  local name="$1"
  local needle="$2"
  local haystack="$3"
  TOTAL=$((TOTAL + 1))
  if echo "$haystack" | grep -q "$needle"; then
    echo -e "  ${GREEN}✓${NC} $name"
    PASS=$((PASS + 1))
  else
    echo -e "  ${RED}✗${NC} $name (expected to contain: $needle)"
    FAIL=$((FAIL + 1))
  fi
}

assert_gt() {
  local name="$1"
  local threshold="$2"
  local actual="$3"
  TOTAL=$((TOTAL + 1))
  if [ "$(echo "$actual > $threshold" | bc -l 2>/dev/null || echo 0)" = "1" ]; then
    echo -e "  ${GREEN}✓${NC} $name ($actual > $threshold)"
    PASS=$((PASS + 1))
  else
    echo -e "  ${RED}✗${NC} $name ($actual not > $threshold)"
    FAIL=$((FAIL + 1))
  fi
}

# Get token — use KHEMEIA_TOKEN env var, or try kubectl, or try client credentials
TOKEN="${KHEMEIA_TOKEN:-}"
get_token() {
  if [ -n "$TOKEN" ]; then return; fi
  # Try kubectl for secret
  local secret
  secret=$(kubectl get secret docking-oidc-secret -n chem -o jsonpath='{.data.client-secret}' 2>/dev/null | base64 -d 2>/dev/null || echo "")
  if [ -n "$secret" ]; then
    TOKEN=$(curl -sf -X POST https://auth.hwcopeland.net/application/o/token/ \
      -d "grant_type=client_credentials" \
      -d "client_id=docking-controller" \
      -d "client_secret=$secret" \
      -d "scope=openid" 2>/dev/null | jq -r '.access_token' 2>/dev/null || echo "")
  fi
  if [ -z "$TOKEN" ]; then
    echo -e "  ${YELLOW}⚠ No auth token available — some tests may fail${NC}"
  fi
}

auth_header() {
  if [ -n "$TOKEN" ]; then
    echo "Authorization: Bearer $TOKEN"
  else
    echo "X-No-Auth: true"
  fi
}

echo ""
echo -e "${YELLOW}═══ Khemeia API Integration Tests ═══${NC}"
echo "Target: $API"
echo ""

# ─── Health ───
echo -e "${YELLOW}Health${NC}"
RESP=$(curl -sf "$API/health" 2>/dev/null || echo '{}')
assert "GET /health returns 200" "healthy" "$(echo "$RESP" | jq -r '.status' 2>/dev/null)"

RESP=$(curl -sf "$API/readyz" 2>/dev/null || echo '{}')
assert "GET /readyz returns ready" "ready" "$(echo "$RESP" | jq -r '.status' 2>/dev/null)"

# ─── Plugins ───
echo ""
echo -e "${YELLOW}Plugin System${NC}"
get_token
RESP=$(curl -sf -H "$(auth_header)" "$API/api/v1/plugins" 2>/dev/null || echo '{}')
assert_contains "GET /api/v1/plugins returns plugin list" "plugins" "$RESP"

PLUGIN_COUNT=$(echo "$RESP" | jq '.plugins | length' 2>/dev/null || echo 0)
assert_gt "At least 1 plugin registered" "0" "$PLUGIN_COUNT"

# Check QE plugin exists
QE_SLUG=$(echo "$RESP" | jq -r '.plugins[] | select(.slug == "qe") | .slug' 2>/dev/null)
assert "QE plugin registered" "qe" "$QE_SLUG"

# Check plugin has input schema
QE_INPUTS=$(echo "$RESP" | jq '.plugins[] | select(.slug == "qe") | .input | length' 2>/dev/null || echo 0)
assert_gt "QE plugin has input fields" "0" "$QE_INPUTS"

# ─── QE Job Lifecycle ───
echo ""
echo -e "${YELLOW}QE Job Lifecycle${NC}"

# Submit a Silicon SCF job
QE_INPUT=$(cat <<'QEINPUT'
&CONTROL
  calculation = 'scf'
  prefix = 'si_test'
  outdir = './tmp'
  pseudo_dir = './'
/
&SYSTEM
  ibrav = 2, celldm(1) = 10.2
  nat = 2, ntyp = 1, ecutwfc = 30.0
/
&ELECTRONS
  mixing_beta = 0.7, conv_thr = 1.0d-8
/
ATOMIC_SPECIES
  Si 28.086 Si.pbe-n-rrkjus_psl.1.0.0.UPF
ATOMIC_POSITIONS {alat}
  Si 0.00 0.00 0.00
  Si 0.25 0.25 0.25
K_POINTS {automatic}
  4 4 4 1 1 1
QEINPUT
)

SUBMIT_RESP=$(curl -sf -X POST "$API/api/v1/qe/submit" \
  -H "$(auth_header)" -H "Content-Type: application/json" \
  -d "$(jq -n --arg input "$QE_INPUT" '{input_file: $input, executable: "pw.x", num_cpus: 1, memory_mb: 1024}')" \
  2>/dev/null || echo '{}')

JOB_NAME=$(echo "$SUBMIT_RESP" | jq -r '.name' 2>/dev/null)
assert_contains "QE submit returns job name" "qe-" "$JOB_NAME"

if [ "$JOB_NAME" != "null" ] && [ -n "$JOB_NAME" ]; then
  # Poll until complete (max 5 min)
  echo "  Polling $JOB_NAME..."
  STATUS="Pending"
  for i in $(seq 1 30); do
    RESP=$(curl -sf -H "$(auth_header)" "$API/api/v1/qe/jobs/$JOB_NAME" 2>/dev/null || echo '{}')
    STATUS=$(echo "$RESP" | jq -r '.status' 2>/dev/null || echo "?")
    [ "$STATUS" = "Completed" ] || [ "$STATUS" = "Failed" ] && break
    sleep 10
  done

  assert "QE job completes" "Completed" "$STATUS"

  # Check output
  OUTPUT=$(echo "$RESP" | jq -r '.output_data' 2>/dev/null || echo '{}')
  ENERGY=$(echo "$OUTPUT" | jq -r '.total_energy' 2>/dev/null || echo "null")
  if [ "$ENERGY" != "null" ] && [ "$ENERGY" != "" ]; then
    echo -e "  ${GREEN}✓${NC} QE job has total_energy: $ENERGY Ry"
    PASS=$((PASS + 1))
  else
    echo -e "  ${RED}✗${NC} QE job missing total_energy"
    FAIL=$((FAIL + 1))
  fi
  TOTAL=$((TOTAL + 1))

  # List jobs — should include our job
  LIST_RESP=$(curl -sf -H "$(auth_header)" "$API/api/v1/qe/jobs" 2>/dev/null || echo '{}')
  assert_contains "QE job list includes our job" "$JOB_NAME" "$LIST_RESP"

  # Delete job
  DEL_STATUS=$(curl -sf -o /dev/null -w "%{http_code}" -X DELETE \
    -H "$(auth_header)" "$API/api/v1/qe/jobs/$JOB_NAME" 2>/dev/null)
  assert "QE job delete returns 204" "204" "$DEL_STATUS"
fi

# ─── Summary ───
echo ""
echo -e "${YELLOW}═══ Results ═══${NC}"
echo -e "Total: $TOTAL  ${GREEN}Passed: $PASS${NC}  ${RED}Failed: $FAIL${NC}"
echo ""

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
