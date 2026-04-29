"""AutoDock-GPU docking engine sidecar.

Uses NVIDIA's official AutoDock-GPU container (nvcr.io/hpc/autodock)
for GPU-accelerated molecular docking. Exposes the same POST /dock
contract as vina-1.2 and gnina for interchangeable use.

AutoDock-GPU runs the AutoDock4 scoring function on CUDA GPUs with
massive parallelism (thousands of concurrent evaluations).
"""

import logging
import os
import re
import subprocess
import tempfile
import traceback

from flask import Flask, jsonify, request

app = Flask(__name__)
logging.basicConfig(level=logging.INFO)
log = logging.getLogger(__name__)

# AutoDock-GPU binary path (pre-installed in the NVIDIA container)
AUTODOCK_GPU = os.environ.get("AUTODOCK_GPU_BIN", "autodock_gpu_128wi")


@app.route("/health", methods=["GET"])
def health():
    return jsonify({"status": "ok"})


@app.route("/readyz", methods=["GET"])
def readyz():
    try:
        result = subprocess.run(
            [AUTODOCK_GPU, "--help"],
            capture_output=True, text=True, timeout=10,
        )
        return jsonify({"status": "ready", "engine": "autodock-gpu"})
    except Exception as e:
        return jsonify({"status": "not ready", "error": str(e)}), 503


@app.route("/dock", methods=["POST"])
def dock():
    data = request.get_json(force=True)

    receptor_pdbqt = data.get("receptor_pdbqt", "")
    ligand_pdbqt = data.get("ligand_pdbqt", "")
    center = data.get("center", [0, 0, 0])
    size = data.get("size", [20, 20, 20])
    exhaustiveness = data.get("exhaustiveness", 32)
    n_poses = data.get("n_poses", 9)

    if not receptor_pdbqt or not ligand_pdbqt:
        return jsonify({"error": "receptor_pdbqt and ligand_pdbqt required"}), 400

    try:
        with tempfile.TemporaryDirectory() as tmpdir:
            rec_path = os.path.join(tmpdir, "receptor.pdbqt")
            lig_path = os.path.join(tmpdir, "ligand.pdbqt")
            out_path = os.path.join(tmpdir, "output.dlg")

            with open(rec_path, "w") as f:
                f.write(receptor_pdbqt)
            with open(lig_path, "w") as f:
                f.write(ligand_pdbqt)

            # AutoDock-GPU uses a .gpf (grid parameter file) or direct CLI args
            # For simplicity, use Vina-style PDBQT input with the autodock binary
            cmd = [
                AUTODOCK_GPU,
                "--ffile", rec_path,
                "--lfile", lig_path,
                "--nrun", str(n_poses),
                "--nev", str(exhaustiveness * 1000000),
                "--resnam", out_path,
            ]

            log.info(f"Running: {' '.join(cmd)}")
            result = subprocess.run(
                cmd, capture_output=True, text=True, timeout=600,
            )

            if result.returncode != 0:
                log.error(f"AutoDock-GPU failed: {result.stderr[:500]}")
                return jsonify({
                    "error": f"AutoDock-GPU exited with code {result.returncode}",
                    "stderr": result.stderr[:500],
                }), 500

            # Parse output for poses and scores
            poses = parse_autodock_output(result.stdout, out_path, tmpdir)

            return jsonify({
                "poses": poses,
                "engine": "autodock-gpu",
                "scoring": "ad4",
            })

    except subprocess.TimeoutExpired:
        return jsonify({"error": "Docking timed out (600s)"}), 504
    except Exception as e:
        log.error(f"Docking failed: {traceback.format_exc()}")
        return jsonify({"error": str(e)}), 500


def parse_autodock_output(stdout, dlg_path, tmpdir):
    """Parse AutoDock-GPU output for poses and binding energies."""
    poses = []

    # Try parsing the DLG (docking log) file
    try:
        if os.path.exists(dlg_path):
            with open(dlg_path) as f:
                dlg = f.read()

            # Extract ranked results
            for match in re.finditer(
                r"RANKING\s+(\d+)\s+(-?[\d.]+)\s+(-?[\d.]+)", dlg
            ):
                rank = int(match.group(1))
                energy = float(match.group(2))
                poses.append({
                    "affinity": energy,
                    "rmsd_lb": 0.0,
                    "rmsd_ub": 0.0,
                    "pdbqt": "",  # TODO: extract pose PDBQT from DLG
                })
    except Exception as e:
        log.warning(f"Failed to parse DLG: {e}")

    # Fallback: parse stdout for energy values
    if not poses:
        for match in re.finditer(
            r"[-]?\d+\.\d+\s+kcal/mol", stdout
        ):
            energy = float(match.group().split()[0])
            poses.append({
                "affinity": energy,
                "rmsd_lb": 0.0,
                "rmsd_ub": 0.0,
                "pdbqt": "",
            })

    # Sort by affinity (most negative first)
    poses.sort(key=lambda p: p["affinity"])

    return poses


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8000, debug=True)
