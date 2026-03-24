#!/usr/bin/env python3
"""CI test: generate 1000 mock Vina results, export to MySQL, assert count.

Designed to run as a one-shot kubectl pod inside the cluster so it can
reach docking-mysql.chem.svc.cluster.local directly.

Required env vars: MYSQL_HOST, MYSQL_PORT, MYSQL_USER, MYSQL_PASSWORD,
                   MYSQL_DATABASE, WORKFLOW_NAME
"""

import os
import random
import re
import sys
import tempfile

import mysql.connector

WORKFLOW  = os.environ["WORKFLOW_NAME"]
COUNT     = 1000
BATCHES   = 5

# ── 1. Generate mock Vina log files ──────────────────────────────────────────
LOG_TEMPLATE = """\
mode |   affinity | dist from best mode
     | (kcal/mol) | rmsd l.b.| rmsd u.b.
-----+------------+----------+----------
   1      {aff:>6.1f}      0.000      0.000
"""

base = tempfile.mkdtemp()
per_batch, rem = divmod(COUNT, BATCHES)
idx = 1
for b in range(BATCHES):
    size = per_batch + (1 if b < rem else 0)
    docked = os.path.join(base, f"ligands_batch{b}", "docked")
    os.makedirs(docked)
    for _ in range(size):
        aff = round(random.uniform(-12.0, -3.0), 1)
        with open(os.path.join(docked, f"ligand_{idx}.log"), "w") as f:
            f.write(LOG_TEMPLATE.format(aff=aff))
        idx += 1
print(f"Generated {idx - 1} mock log files in {base}")

# ── 2. Parse all log files ────────────────────────────────────────────────────
mode1 = re.compile(r'^\s+1\s+([-\d.]+)\s+')
rows = []
for path in sorted(__import__("glob").glob(f"{base}/ligands_batch*/docked/*.log")):
    parts = path.split(os.sep)
    batch = parts[-3]
    ligand = os.path.splitext(parts[-1])[0]
    for line in open(path):
        m = mode1.match(line)
        if m:
            rows.append((WORKFLOW, "7jrn", batch, ligand, float(m.group(1))))
            break
print(f"Parsed {len(rows)} results")

# ── 3. Export to MySQL ────────────────────────────────────────────────────────
conn = mysql.connector.connect(
    host=os.environ["MYSQL_HOST"],
    port=int(os.environ.get("MYSQL_PORT", 3306)),
    user=os.environ["MYSQL_USER"],
    password=os.environ["MYSQL_PASSWORD"],
    database=os.environ["MYSQL_DATABASE"],
)
cur = conn.cursor()
cur.execute("""CREATE TABLE IF NOT EXISTS docking_results (
    id INT AUTO_INCREMENT PRIMARY KEY,
    workflow_name VARCHAR(255) NOT NULL,
    pdb_id VARCHAR(10) NOT NULL,
    batch_label VARCHAR(255) NOT NULL,
    ligand_name VARCHAR(255) NOT NULL,
    affinity_kcal_mol FLOAT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_workflow (workflow_name)
)""")
cur.executemany(
    "INSERT INTO docking_results (workflow_name,pdb_id,batch_label,ligand_name,affinity_kcal_mol)"
    " VALUES (%s,%s,%s,%s,%s)", rows)
conn.commit()
print(f"Inserted {cur.rowcount} rows")

# ── 4. Verify ─────────────────────────────────────────────────────────────────
cur.execute("SELECT COUNT(*) FROM docking_results WHERE workflow_name=%s", (WORKFLOW,))
count = cur.fetchone()[0]
cur.close(); conn.close()
print(f"Rows in MySQL for workflow '{WORKFLOW}': {count}")
if count != COUNT:
    print(f"FAIL: expected {COUNT}, got {count}")
    sys.exit(1)
print(f"PASS: {count} == {COUNT}")
