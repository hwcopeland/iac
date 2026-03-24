#!/usr/bin/env python3
"""Export AutoDock Vina docking results to MySQL.

Reads all .log files from {base_dir}/{db_label}_batch*/docked/*.log,
parses mode-1 affinity scores, and bulk-inserts them into MySQL.

MySQL connection is configured via environment variables:
  MYSQL_HOST     (default: localhost)
  MYSQL_PORT     (default: 3306)
  MYSQL_USER     (default: root)
  MYSQL_PASSWORD (required)
  MYSQL_DATABASE (default: docking)
"""

import os
import re
import glob
import argparse
import sys


def parse_args():
    parser = argparse.ArgumentParser(description="Export docking results to MySQL.")
    parser.add_argument("--workflow", required=True, help="Workflow/run name identifier.")
    parser.add_argument("--pdbid", required=True, help="PDB ID of the receptor protein.")
    parser.add_argument("--db-label", required=True,
                        help="Ligand database label used for batch directory naming.")
    parser.add_argument("--base-dir", default=".",
                        help="Base directory containing batch result folders (default: .).")
    return parser.parse_args()


def get_mysql_connection():
    try:
        import mysql.connector
    except ImportError:
        print("Error: mysql-connector-python not installed. Run: pip install mysql-connector-python")
        sys.exit(1)

    conn = mysql.connector.connect(
        host=os.environ.get("MYSQL_HOST", "localhost"),
        port=int(os.environ.get("MYSQL_PORT", "3306")),
        user=os.environ.get("MYSQL_USER", "root"),
        password=os.environ.get("MYSQL_PASSWORD", ""),
        database=os.environ.get("MYSQL_DATABASE", "docking"),
    )
    return conn


def ensure_table(cursor):
    cursor.execute("""
        CREATE TABLE IF NOT EXISTS docking_results (
            id                INT AUTO_INCREMENT PRIMARY KEY,
            workflow_name     VARCHAR(255) NOT NULL,
            pdb_id            VARCHAR(10)  NOT NULL,
            batch_label       VARCHAR(255) NOT NULL,
            ligand_name       VARCHAR(255) NOT NULL,
            affinity_kcal_mol FLOAT        NOT NULL,
            created_at        TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
            INDEX idx_workflow (workflow_name),
            INDEX idx_pdbid    (pdb_id),
            INDEX idx_affinity (affinity_kcal_mol)
        )
    """)


# Matches the mode-1 result line in AutoDock Vina output:
#   "   1       -7.1      0.000      0.000"
_MODE1_RE = re.compile(r'^\s+1\s+([-\d.]+)\s+')


def parse_log_file(log_path):
    """Return the mode-1 affinity (kcal/mol) from a Vina log file, or None."""
    try:
        with open(log_path, 'r') as f:
            for line in f:
                m = _MODE1_RE.match(line)
                if m:
                    return float(m.group(1))
    except (OSError, ValueError) as exc:
        print(f"Warning: could not parse {log_path}: {exc}")
    return None


def collect_results(base_dir, db_label, workflow, pdbid):
    pattern = os.path.join(base_dir, f"{db_label}_batch*", "docked", "*.log")
    log_files = sorted(glob.glob(pattern))

    if not log_files:
        print(f"No log files found matching: {pattern}")
        return []

    results = []
    skipped = 0
    for log_path in log_files:
        parts = log_path.replace("\\", "/").split("/")
        batch_label = parts[-3]                          # e.g. "ligands_batch0"
        ligand_name = os.path.splitext(parts[-1])[0]    # e.g. "ligand_1"

        affinity = parse_log_file(log_path)
        if affinity is not None:
            results.append((workflow, pdbid, batch_label, ligand_name, affinity))
        else:
            skipped += 1

    if skipped:
        print(f"Skipped {skipped} log file(s) with no parseable mode-1 result.")
    return results


def export_to_mysql(results):
    if not results:
        print("No results to export.")
        return 0

    conn = get_mysql_connection()
    cursor = conn.cursor()
    ensure_table(cursor)

    cursor.executemany(
        """INSERT INTO docking_results
               (workflow_name, pdb_id, batch_label, ligand_name, affinity_kcal_mol)
           VALUES (%s, %s, %s, %s, %s)""",
        results,
    )
    conn.commit()
    count = cursor.rowcount
    cursor.close()
    conn.close()
    return count


def main():
    args = parse_args()
    print(f"workflow={args.workflow}  pdbid={args.pdbid}  "
          f"db_label={args.db_label}  base_dir={args.base_dir}")

    results = collect_results(args.base_dir, args.db_label, args.workflow, args.pdbid)
    print(f"Parsed {len(results)} docking result(s).")

    if not results:
        print("ERROR: No docking results found. Check that docking jobs completed successfully.")
        sys.exit(1)

    exported = export_to_mysql(results)
    print(f"Exported {exported} row(s) to MySQL table 'docking_results'.")


if __name__ == "__main__":
    main()
