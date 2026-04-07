#!/bin/bash
# API Integration Tests for Khemeia
# Runs against the live cluster API
# Usage: ./api.test.sh [API_URL]
#
# Includes:
#   - Health / readiness checks
#   - Plugin registry
#   - QE (Quantum ESPRESSO) job lifecycle
#   - Docking benchmark: HIV-1 protease (1HSG) with FDA-approved inhibitors

set -euo pipefail

API="${1:-https://khemeia.hwcopeland.net}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
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

assert_lt() {
  local name="$1"
  local threshold="$2"
  local actual="$3"
  TOTAL=$((TOTAL + 1))
  if [ "$(echo "$actual < $threshold" | bc -l 2>/dev/null || echo 0)" = "1" ]; then
    echo -e "  ${GREEN}✓${NC} $name ($actual < $threshold)"
    PASS=$((PASS + 1))
  else
    echo -e "  ${RED}✗${NC} $name ($actual not < $threshold)"
    FAIL=$((FAIL + 1))
  fi
}

assert_between() {
  local name="$1"
  local lo="$2"
  local hi="$3"
  local actual="$4"
  TOTAL=$((TOTAL + 1))
  local in_range
  in_range=$(echo "$actual >= $lo && $actual <= $hi" | bc -l 2>/dev/null || echo 0)
  if [ "$in_range" = "1" ]; then
    echo -e "  ${GREEN}✓${NC} $name ($actual in [$lo, $hi])"
    PASS=$((PASS + 1))
  else
    echo -e "  ${RED}✗${NC} $name ($actual not in [$lo, $hi])"
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
    echo -e "  ${YELLOW}! No auth token available -- some tests may fail${NC}"
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
echo -e "${YELLOW}=== Khemeia API Integration Tests ===${NC}"
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

# Check docking plugin exists
DOCK_SLUG=$(echo "$RESP" | jq -r '.plugins[] | select(.slug == "docking") | .slug' 2>/dev/null)
assert "Docking plugin registered" "docking" "$DOCK_SLUG"

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

# ─── Docking: HIV-1 Protease (1HSG) Benchmark ───
echo ""
echo -e "${YELLOW}Docking: HIV-1 Protease (1HSG) Benchmark${NC}"

# Seed benchmark ligands via the API (idempotent — uses UPSERT).
# These are 8 FDA-approved HIV protease inhibitors with known binding affinity.
BENCHMARK_SOURCE_DB="hiv-benchmark"
BENCHMARK_PDBID="1HSG"
BENCHMARK_NATIVE_LIGAND="MK1"

SEED_PAYLOAD=$(cat <<'SEED'
[
  {"compound_id":"DB00224-indinavir","smiles":"CC(C)(C)NC(=O)[C@@H]1CN(CCN1C[C@@H](O)C[C@@H](CC2=CC=CC=C2)C(=O)N[C@H]3[C@H](O)CC4=CC=CC=C34)CC5=CC=CN=C5","source_db":"hiv-benchmark"},
  {"compound_id":"DB00503-ritonavir","smiles":"CC(C)[C@H](NC(=O)N(C)CC1=CSC(=N1)C(C)C)C(=O)N[C@@H](C[C@@H](O)[C@H](CC2=CC=CC=C2)NC(=O)OCC3=CN=CS3)CC4=CC=CC=C4","source_db":"hiv-benchmark"},
  {"compound_id":"DB01232-saquinavir","smiles":"CC(C)(C)NC(=O)[C@@H]1CC2=CC=CC=C2C[C@@H]1NC(=O)[C@H](CC3=CC=CC=C3)NC(=O)C4=NC5=CC=CC=C5C=C4","source_db":"hiv-benchmark"},
  {"compound_id":"DB00220-nelfinavir","smiles":"OC1CC2=CC=CC=C2[C@@H]1NC(=O)[C@H](CC3=CC=CC=C3)CNC(=O)C4=CC5=C(OC(C)(C)C5)C=C4SC","source_db":"hiv-benchmark"},
  {"compound_id":"DB00701-amprenavir","smiles":"CC(C)CN(CC(O)[C@H](CC1=CC=CC=C1)NC(=O)OC2COC3CCCC23)S(=O)(=O)C4=CC=C(N)C=C4","source_db":"hiv-benchmark"},
  {"compound_id":"DB01601-lopinavir","smiles":"CC(C)[C@H](NC(=O)[C@H](CC1=CC=CC=C1)CC(=O)N[C@@H](C[C@@H](O)[C@H](CC2=CC=CC=C2)NC(=O)COC3=CC=CC=C3)CC4=CC=CC=C4)C(C)C","source_db":"hiv-benchmark"},
  {"compound_id":"DB01072-atazanavir","smiles":"COC(=O)N[C@H](C(=O)N[C@@H](CC1=CC=CC=C1)[C@@H](O)CN(CC2=CC=C(C=C2)C3=CC=CC=N3)NC(=O)[C@H](NC(=O)OC)C(C)(C)C)C(C)(C)C","source_db":"hiv-benchmark"},
  {"compound_id":"DB00932-tipranavir","smiles":"CCC(CC)OC1=CC(=CC(=C1)NS(=O)(=O)C2=CC=C(C=C2)C3=CC(=NN3CC4CC4)C(F)(F)F)[C@H](CC)O","source_db":"hiv-benchmark"}
]
SEED
)

# Import ligands via API
IMPORT_RESP=$(curl -sf -X POST "$API/api/v1/ligands" \
  -H "$(auth_header)" -H "Content-Type: application/json" \
  -d "$SEED_PAYLOAD" 2>/dev/null || echo '{}')

IMPORTED=$(echo "$IMPORT_RESP" | jq -r '.imported // 0' 2>/dev/null)
IMPORT_TOTAL=$(echo "$IMPORT_RESP" | jq -r '.total // 0' 2>/dev/null)
assert "Imported benchmark ligands" "8" "$IMPORT_TOTAL"
assert_gt "At least 1 ligand imported or updated" "0" "$IMPORTED"

echo ""
echo -e "${YELLOW}Docking: Submit 1HSG Job${NC}"

# Submit a docking job for HIV-1 protease with HIV benchmark ligands.
# The plugin slug is "docking", so the endpoint is /api/v1/docking/submit.
DOCK_SUBMIT_RESP=$(curl -sf -X POST "$API/api/v1/docking/submit" \
  -H "$(auth_header)" -H "Content-Type: application/json" \
  -d "$(jq -n \
    --arg pdbid "$BENCHMARK_PDBID" \
    --arg ligand_db "$BENCHMARK_SOURCE_DB" \
    --arg native_ligand "$BENCHMARK_NATIVE_LIGAND" \
    --argjson chunk_size 10000 \
    '{pdbid: $pdbid, ligand_db: $ligand_db, native_ligand: $native_ligand, chunk_size: $chunk_size}')" \
  2>/dev/null || echo '{}')

DOCK_JOB_NAME=$(echo "$DOCK_SUBMIT_RESP" | jq -r '.name' 2>/dev/null)
assert_contains "Docking submit returns job name" "docking-" "$DOCK_JOB_NAME"

if [ "$DOCK_JOB_NAME" != "null" ] && [ -n "$DOCK_JOB_NAME" ]; then
  # Poll until complete (max 15 min — docking involves protein prep, ligand prep, and Vina)
  echo "  Polling $DOCK_JOB_NAME (1HSG + 8 HIV inhibitors, timeout 15m)..."
  DOCK_STATUS="Pending"
  DOCK_POLL_MAX=90    # 90 x 10s = 15 minutes
  for i in $(seq 1 $DOCK_POLL_MAX); do
    DOCK_RESP=$(curl -sf -H "$(auth_header)" "$API/api/v1/docking/jobs/$DOCK_JOB_NAME" 2>/dev/null || echo '{}')
    DOCK_STATUS=$(echo "$DOCK_RESP" | jq -r '.status' 2>/dev/null || echo "?")

    case "$DOCK_STATUS" in
      Completed|Failed) break ;;
    esac

    # Progress indicator every 30s
    if [ $((i % 3)) -eq 0 ]; then
      echo "    ... status=$DOCK_STATUS (${i}0s elapsed)"
    fi

    sleep 10
  done

  assert "Docking job completes" "Completed" "$DOCK_STATUS"

  # Validate output contains docking results
  DOCK_OUTPUT=$(echo "$DOCK_RESP" | jq -r '.output_data' 2>/dev/null || echo '{}')

  # Check best affinity from Vina output (parsed by plugin output regex)
  BEST_AFFINITY=$(echo "$DOCK_OUTPUT" | jq -r '.best_affinity // empty' 2>/dev/null || echo "")
  if [ -n "$BEST_AFFINITY" ] && [ "$BEST_AFFINITY" != "null" ]; then
    echo -e "  ${GREEN}✓${NC} Best Vina affinity: $BEST_AFFINITY kcal/mol"
    PASS=$((PASS + 1))

    # HIV protease inhibitors should have Vina scores between -7 and -12 kcal/mol.
    # Vina reports negative values (more negative = stronger binding).
    assert_lt "Best affinity below -7 kcal/mol" "-7" "$BEST_AFFINITY"
    assert_gt "Best affinity above -12 kcal/mol" "-12" "$BEST_AFFINITY"
  else
    echo -e "  ${RED}✗${NC} No best_affinity in docking output"
    FAIL=$((FAIL + 1))
  fi
  TOTAL=$((TOTAL + 1))

  # Check result count (should have processed some ligands)
  RESULT_COUNT=$(echo "$DOCK_OUTPUT" | jq -r '.result_count // 0' 2>/dev/null || echo "0")
  assert_gt "Docking processed at least 1 ligand" "0" "$RESULT_COUNT"

  # List docking jobs — should include our job
  DOCK_LIST=$(curl -sf -H "$(auth_header)" "$API/api/v1/docking/jobs" 2>/dev/null || echo '{}')
  assert_contains "Docking job list includes our job" "$DOCK_JOB_NAME" "$DOCK_LIST"

  # Delete docking job
  DOCK_DEL_STATUS=$(curl -sf -o /dev/null -w "%{http_code}" -X DELETE \
    -H "$(auth_header)" "$API/api/v1/docking/jobs/$DOCK_JOB_NAME" 2>/dev/null)
  assert "Docking job delete returns 204" "204" "$DOCK_DEL_STATUS"
fi

# ─── Summary ───
echo ""
echo -e "${YELLOW}=== Results ===${NC}"
echo -e "Total: $TOTAL  ${GREEN}Passed: $PASS${NC}  ${RED}Failed: $FAIL${NC}"
echo ""

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
