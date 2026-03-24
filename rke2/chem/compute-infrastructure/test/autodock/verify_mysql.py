#!/usr/bin/env python3
"""Verify that MySQL contains the expected number of docking results."""

import os
import sys
import argparse


def get_conn():
    import mysql.connector
    return mysql.connector.connect(
        host=os.environ.get("MYSQL_HOST", "localhost"),
        port=int(os.environ.get("MYSQL_PORT", "3306")),
        user=os.environ.get("MYSQL_USER", "root"),
        password=os.environ.get("MYSQL_PASSWORD", ""),
        database=os.environ.get("MYSQL_DATABASE", "docking"),
    )


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--expected", type=int, required=True,
                        help="Expected row count in docking_results.")
    parser.add_argument("--workflow", default="test-run-001",
                        help="Workflow name filter.")
    args = parser.parse_args()

    conn = get_conn()
    cursor = conn.cursor()

    cursor.execute(
        "SELECT COUNT(*) FROM docking_results WHERE workflow_name = %s",
        (args.workflow,),
    )
    count = cursor.fetchone()[0]
    print(f"Rows in MySQL for workflow '{args.workflow}': {count}")

    if count != args.expected:
        print(f"FAIL: expected {args.expected}, got {count}")
        cursor.close()
        conn.close()
        sys.exit(1)

    print(f"PASS: {count} == {args.expected}")

    # Print summary statistics
    cursor.execute(
        """SELECT
               MIN(affinity_kcal_mol) AS best,
               MAX(affinity_kcal_mol) AS worst,
               AVG(affinity_kcal_mol) AS avg_aff
           FROM docking_results
           WHERE workflow_name = %s""",
        (args.workflow,),
    )
    row = cursor.fetchone()
    print(f"Affinity (kcal/mol): best={row[0]:.2f}  worst={row[1]:.2f}  avg={row[2]:.2f}")

    # Print top 5 ligands
    cursor.execute(
        """SELECT ligand_name, batch_label, affinity_kcal_mol
           FROM docking_results
           WHERE workflow_name = %s
           ORDER BY affinity_kcal_mol
           LIMIT 5""",
        (args.workflow,),
    )
    rows = cursor.fetchall()
    print("\nTop 5 ligands by binding affinity:")
    print(f"  {'Ligand':<20} {'Batch':<30} {'Affinity':>10}")
    print("  " + "-" * 62)
    for r in rows:
        print(f"  {r[0]:<20} {r[1]:<30} {r[2]:>10.1f}")

    cursor.close()
    conn.close()


if __name__ == "__main__":
    main()
