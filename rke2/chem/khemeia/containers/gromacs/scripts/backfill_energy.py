#!/usr/bin/env python3
"""
One-off backfill: generate energy.json for completed MD results that lack it.

Runs inside a GROMACS container (needs gmx binary + boto3 + pymysql).
Downloads md.edr from S3, runs gmx energy, uploads energy.json, updates DB.
"""
import json
import os
import shutil
import subprocess
import tempfile
from pathlib import Path

import boto3
import mysql.connector

# NGC GROMACS image: binary is at this path (not on subprocess PATH by default)
_GMX_CANDIDATES = [
    '/usr/local/gromacs/bin/gmx',
    '/usr/local/gromacs/avx2_256/bin/gmx',
    '/usr/local/gromacs/avx_512/bin/gmx',
    '/usr/local/gromacs/avx_256/bin/gmx',
    '/usr/local/gromacs/sse4.1/bin/gmx',
]
GMX = shutil.which('gmx') or next((p for p in _GMX_CANDIDATES if Path(p).exists()), 'gmx')


def s3_client():
    return boto3.client(
        "s3",
        endpoint_url=os.environ["GARAGE_ENDPOINT"],
        aws_access_key_id=os.environ["GARAGE_ACCESS_KEY"],
        aws_secret_access_key=os.environ["GARAGE_SECRET_KEY"],
        region_name=os.environ.get("GARAGE_REGION", "garage"),
    )


def parse_xvg(path):
    time, potential, temperature = [], [], []
    with open(path) as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith(("#", "@")):
                continue
            parts = line.split()
            if len(parts) >= 3:
                try:
                    time.append(float(parts[0]))
                    potential.append(float(parts[1]))
                    temperature.append(float(parts[2]))
                except ValueError:
                    pass
            elif len(parts) == 2:
                try:
                    time.append(float(parts[0]))
                    potential.append(float(parts[1]))
                except ValueError:
                    pass
    return {"time": time, "potential": potential, "temperature": temperature}


def main():
    s3 = s3_client()
    bucket = "khemeia-trajectories"

    conn = mysql.connector.connect(
        host=os.environ.get("MYSQL_HOST", "docking-mysql.chem.svc.cluster.local"),
        port=int(os.environ.get("MYSQL_PORT", "3306")),
        user=os.environ.get("MYSQL_USER", "root"),
        password=os.environ.get("MYSQL_PASSWORD", ""),
        database=os.environ.get("MYSQL_DB", "docking"),
    )
    cursor = conn.cursor()

    cursor.execute(
        """SELECT job_name, compound_id, energy_s3_key
           FROM md_results
           WHERE energy_s3_key IS NOT NULL
             AND energy_s3_key != ''
             AND (energy_json_s3_key IS NULL OR energy_json_s3_key = '')"""
    )
    rows = cursor.fetchall()
    print(f"Found {len(rows)} compounds to backfill", flush=True)

    for job_name, compound_id, edr_key in rows:
        print(f"Processing {compound_id} ...", flush=True)
        with tempfile.TemporaryDirectory() as tmp:
            wd = Path(tmp)
            edr_path = wd / "md.edr"
            xvg_path = wd / "energy.xvg"
            json_path = wd / "energy.json"

            # Download EDR
            try:
                s3.download_file(bucket, edr_key, str(edr_path))
            except Exception as e:
                print(f"  SKIP: download failed: {e}", flush=True)
                continue

            # Extract energy
            result = subprocess.run(
                [GMX, "-quiet", "energy", "-f", str(edr_path), "-o", str(xvg_path)],
                input="Potential\nTemperature\n0\n",
                capture_output=True, text=True, cwd=str(wd),
            )
            if result.returncode != 0 or not xvg_path.exists():
                print(f"  SKIP: gmx energy failed: {result.stderr[-200:]}", flush=True)
                continue

            data = parse_xvg(xvg_path)
            json_path.write_text(json.dumps(data))

            energy_json_key = f"md/{job_name}/{compound_id}/energy.json"
            try:
                s3.upload_file(str(json_path), bucket, energy_json_key)
            except Exception as e:
                print(f"  SKIP: upload failed: {e}", flush=True)
                continue

            cursor.execute(
                "UPDATE md_results SET energy_json_s3_key = %s WHERE job_name = %s AND compound_id = %s",
                (energy_json_key, job_name, compound_id),
            )
            conn.commit()
            print(f"  OK: {energy_json_key} ({len(data['time'])} points)", flush=True)

    cursor.close()
    conn.close()
    print("Backfill complete.", flush=True)


if __name__ == "__main__":
    main()
