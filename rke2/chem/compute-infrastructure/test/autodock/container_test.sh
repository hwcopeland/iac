#!/bin/bash
# Runs inside the test-runner container:
# 1. Install deps
# 2. Generate 1000 mock Vina log files
# 3. Export to MySQL
# 4. Verify 1000 rows landed
set -euo pipefail

echo "Installing mysql-connector-python..."
pip install mysql-connector-python --quiet

echo ""
echo "=== Step 1: Generating 1000 mock docking results ==="
python3 /scripts/generate_mock_data.py \
    --output-dir /tmp/testdata \
    --count 1000 \
    --batches 5 \
    --db-label ligands

echo ""
echo "=== Step 2: Exporting to MySQL ==="
python3 /scripts/export_energies_mysql.py \
    --workflow test-run-001 \
    --pdbid 7jrn \
    --db-label ligands \
    --base-dir /tmp/testdata

echo ""
echo "=== Step 3: Verifying results ==="
python3 /scripts/verify_mysql.py \
    --expected 1000 \
    --workflow test-run-001
