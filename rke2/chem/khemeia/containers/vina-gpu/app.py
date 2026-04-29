"""Vina-GPU 2.1 docking sidecar."""

import logging
import os
import re
import subprocess
import tempfile
import traceback
from pathlib import Path

from flask import Flask, jsonify, request

app = Flask(__name__)
logging.basicConfig(level=logging.INFO)
log = logging.getLogger(__name__)

VINA_GPU_BIN = os.environ.get("VINA_GPU_BIN", "/usr/local/bin/vina-gpu")
OPENCL_BINARY_PATH = os.environ.get("VINA_GPU_OPENCL_BINARY_PATH", "/opt/vina-gpu/bin")
_RESULT_RE = re.compile(r"^REMARK VINA RESULT:\s+(-?\d+(?:\.\d+)?)", re.MULTILINE)


@app.route("/health", methods=["GET"])
def health():
    return jsonify({"status": "ok"})


@app.route("/readyz", methods=["GET"])
def readyz():
    try:
        result = subprocess.run([VINA_GPU_BIN, "--help"], capture_output=True, text=True, timeout=10)
        if result.returncode != 0:
            return jsonify({"status": "not ready", "stderr": result.stderr[:500]}), 503
        return jsonify({"status": "ready", "engine": "vina-gpu"})
    except Exception as exc:
        return jsonify({"status": "not ready", "error": str(exc)}), 503


def parse_pose(path: Path):
    contents = path.read_text()
    match = _RESULT_RE.search(contents)
    if not match:
        return None
    return {
        "affinity": float(match.group(1)),
        "rmsd_lb": 0.0,
        "rmsd_ub": 0.0,
        "pdbqt": contents,
    }


@app.route("/dock", methods=["POST"])
def dock():
    data = request.get_json(force=True)
    receptor_pdbqt = data.get("receptor_pdbqt", "")
    ligand_pdbqt = data.get("ligand_pdbqt", "")
    center = data.get("center", [0, 0, 0])
    size = data.get("size", [20, 20, 20])
    exhaustiveness = int(data.get("exhaustiveness", 32))
    threads = int(data.get("threads", 8000))

    if not receptor_pdbqt or not ligand_pdbqt:
        return jsonify({"error": "receptor_pdbqt and ligand_pdbqt required"}), 400

    try:
        with tempfile.TemporaryDirectory(prefix="vina_gpu_api_") as tmpdir:
            root = Path(tmpdir)
            ligand_dir = root / "ligands"
            output_dir = root / "out"
            ligand_dir.mkdir()
            output_dir.mkdir()

            receptor_path = root / "receptor.pdbqt"
            receptor_path.write_text(receptor_pdbqt)
            ligand_path = ligand_dir / "ligand.pdbqt"
            ligand_path.write_text(ligand_pdbqt)

            cmd = [
                VINA_GPU_BIN,
                "--receptor", str(receptor_path),
                "--ligand_directory", str(ligand_dir),
                "--output_directory", str(output_dir),
                "--opencl_binary_path", OPENCL_BINARY_PATH,
                "--center_x", str(float(center[0])),
                "--center_y", str(float(center[1])),
                "--center_z", str(float(center[2])),
                "--size_x", str(float(size[0])),
                "--size_y", str(float(size[1])),
                "--size_z", str(float(size[2])),
                "--thread", str(threads),
                "--search_depth", str(max(1, exhaustiveness)),
            ]
            log.info("Running: %s", " ".join(cmd))
            result = subprocess.run(cmd, capture_output=True, text=True, timeout=600, cwd=tmpdir)
            if result.returncode != 0:
                return jsonify({
                    "error": f"Vina-GPU exited with code {result.returncode}",
                    "stdout": result.stdout[-1000:],
                    "stderr": result.stderr[-1000:],
                }), 500

            pose = parse_pose(output_dir / "ligand_out.pdbqt")
            if pose is None:
                return jsonify({"poses": [], "engine": "vina-gpu", "scoring": "vina"})
            return jsonify({"poses": [pose], "engine": "vina-gpu", "scoring": "vina"})
    except subprocess.TimeoutExpired:
        return jsonify({"error": "Docking timed out (600s)"}), 504
    except Exception as exc:
        log.error("Docking failed: %s", traceback.format_exc())
        return jsonify({"error": str(exc)}), 500


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8000, debug=True)
