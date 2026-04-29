"""Vina-GPU 2.1 docking engine sidecar.

Exposes a Flask API with a POST /dock endpoint that runs AutoDock Vina-GPU 2.1
(GPU-accelerated via OpenCL).  The request/response contract is identical across
all WP-3 docking engines so the Go API can call any engine interchangeably.

Vina-GPU 2.1 specific:
  - Uses OpenCL for massively parallel docking on NVIDIA GPUs.
  - Supports --thread (GPU parallelism lanes) and --search_depth parameters.
  - Kernel binaries are compiled on first invocation (BUILD_KERNEL_FROM_SOURCE).
"""

import logging
import os
import re
import subprocess
import tempfile
import traceback

from flask import Flask, jsonify, request

app = Flask(__name__)
logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
logger = logging.getLogger(__name__)

VINA_GPU_BIN = "/usr/local/bin/vina-gpu"
OPENCL_BINARY_PATH = "/opt/vina-gpu"

# Matches result lines in Vina log output:
#   "   1       -7.1      0.000      0.000"
_MODE_RE = re.compile(r"^\s+(\d+)\s+([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)")


# ---------------------------------------------------------------------------
# Health endpoints
# ---------------------------------------------------------------------------

@app.route("/health", methods=["GET"])
def health():
    return jsonify({"status": "ok"})


@app.route("/readyz", methods=["GET"])
def readyz():
    """Readiness probe -- verify vina-gpu binary is available."""
    if not (os.path.isfile(VINA_GPU_BIN) and os.access(VINA_GPU_BIN, os.X_OK)):
        return jsonify({
            "status": "not_ready",
            "error": "vina-gpu binary not found",
        }), 503

    # Quick check that the binary can at least print version info
    try:
        result = subprocess.run(
            [VINA_GPU_BIN, "--version"],
            capture_output=True, text=True, timeout=10,
        )
        version_info = (result.stdout + result.stderr).strip()
        return jsonify({"status": "ready", "version": version_info})
    except Exception as exc:
        return jsonify({"status": "not_ready", "error": str(exc)}), 503


# ---------------------------------------------------------------------------
# POST /dock
# ---------------------------------------------------------------------------

@app.route("/dock", methods=["POST"])
def dock():
    """Run Vina-GPU 2.1 docking via CLI.

    Request JSON:
        receptor_pdbqt (str):   Receptor in PDBQT format.
        ligand_pdbqt (str):     Ligand in PDBQT format.
        center (list[float]):   Binding-site center [x, y, z].
        size (list[float]):     Box dimensions [sx, sy, sz].
        exhaustiveness (int):   Search exhaustiveness (default 32).
        scoring (str):          Scoring function (default "vina"; ignored by
                                Vina-GPU but accepted for API compatibility).
        n_poses (int):          Number of poses to return (default 9).
        thread (int):           GPU parallelism lanes (default 8000).
        search_depth (int):     Monte Carlo search depth (default 0 = heuristic).

    Response JSON:
        poses (list[dict]):   Each pose has: pdbqt, affinity, rmsd_lb, rmsd_ub.
        engine (str):         "vina-gpu-2.1"
        scoring (str):        Scoring function used.
    """
    data = request.get_json(force=True)

    # --- Validate required fields ----------------------------------------
    receptor_pdbqt = data.get("receptor_pdbqt")
    ligand_pdbqt = data.get("ligand_pdbqt")
    center = data.get("center")
    size = data.get("size")

    if not receptor_pdbqt:
        return jsonify({"error": "receptor_pdbqt is required"}), 400
    if not ligand_pdbqt:
        return jsonify({"error": "ligand_pdbqt is required"}), 400
    if not center or len(center) != 3:
        return jsonify({"error": "center must be [x, y, z]"}), 400
    if not size or len(size) != 3:
        return jsonify({"error": "size must be [sx, sy, sz]"}), 400

    exhaustiveness = int(data.get("exhaustiveness", 32))
    scoring = data.get("scoring", "vina")
    n_poses = int(data.get("n_poses", 9))
    thread = int(data.get("thread", 8000))
    search_depth = int(data.get("search_depth", 0))

    try:
        poses = _run_vina_gpu(
            receptor_pdbqt, ligand_pdbqt, center, size,
            exhaustiveness, n_poses, thread, search_depth,
        )
        return jsonify({
            "poses": poses,
            "engine": "vina-gpu-2.1",
            "scoring": scoring,
        })
    except Exception as exc:
        logger.error("Docking failed: %s\n%s", exc, traceback.format_exc())
        return jsonify({"error": str(exc)}), 500


# ---------------------------------------------------------------------------
# Vina-GPU execution
# ---------------------------------------------------------------------------

def _run_vina_gpu(receptor_pdbqt, ligand_pdbqt, center, size,
                  exhaustiveness, n_poses, thread, search_depth):
    """Execute Vina-GPU 2.1 docking and return parsed pose list."""
    tmpdir = tempfile.mkdtemp(prefix="vina_gpu_")
    rec_path = os.path.join(tmpdir, "receptor.pdbqt")
    lig_path = os.path.join(tmpdir, "ligand.pdbqt")
    out_path = os.path.join(tmpdir, "docked.pdbqt")
    log_path = os.path.join(tmpdir, "docked.log")

    try:
        with open(rec_path, "w") as f:
            f.write(receptor_pdbqt)
        with open(lig_path, "w") as f:
            f.write(ligand_pdbqt)

        cmd = [
            VINA_GPU_BIN,
            "--receptor", rec_path,
            "--ligand", lig_path,
            "--center_x", str(float(center[0])),
            "--center_y", str(float(center[1])),
            "--center_z", str(float(center[2])),
            "--size_x", str(float(size[0])),
            "--size_y", str(float(size[1])),
            "--size_z", str(float(size[2])),
            "--exhaustiveness", str(exhaustiveness),
            "--num_modes", str(n_poses),
            "--thread", str(thread),
            "--opencl_binary_path", OPENCL_BINARY_PATH,
            "--out", out_path,
            "--log", log_path,
        ]

        # Optional search_depth (0 = heuristic, Vina-GPU decides automatically)
        if search_depth > 0:
            cmd.extend(["--search_depth", str(search_depth)])

        logger.info(
            "Running Vina-GPU: center=[%.1f,%.1f,%.1f] size=[%.1f,%.1f,%.1f] "
            "exhaustiveness=%d thread=%d",
            float(center[0]), float(center[1]), float(center[2]),
            float(size[0]), float(size[1]), float(size[2]),
            exhaustiveness, thread,
        )

        result = subprocess.run(
            cmd, capture_output=True, text=True, timeout=600,
        )
        if result.returncode != 0:
            raise RuntimeError(
                f"Vina-GPU exited with code {result.returncode}: "
                f"{result.stderr}"
            )

        # Parse results from log
        energies = _parse_log(log_path)

        poses_pdbqt = ""
        if os.path.exists(out_path):
            with open(out_path) as f:
                poses_pdbqt = f.read()

        pose_blocks = _split_pdbqt_models(poses_pdbqt)

        poses = []
        for i, block in enumerate(pose_blocks):
            energy_row = energies[i] if i < len(energies) else (0.0, 0.0, 0.0)
            poses.append({
                "pdbqt": block,
                "affinity": float(energy_row[0]),
                "rmsd_lb": float(energy_row[1]),
                "rmsd_ub": float(energy_row[2]),
            })

        logger.info(
            "Docking complete: %d poses, best affinity %.2f kcal/mol",
            len(poses), poses[0]["affinity"] if poses else 0.0,
        )
        return poses

    finally:
        # Clean up temp files
        for f in [rec_path, lig_path, out_path, log_path]:
            if os.path.exists(f):
                os.unlink(f)
        if os.path.isdir(tmpdir):
            os.rmdir(tmpdir)


def _parse_log(log_path):
    """Parse Vina-GPU log file for pose energies.

    Vina-GPU log format (after the header):
        mode | affinity | dist from best mode
                         rmsd l.b.| rmsd u.b.
          1       -7.1      0.000      0.000
          2       -6.8      1.234      2.345

    Returns list of (affinity, rmsd_lb, rmsd_ub) tuples.
    """
    energies = []
    if not os.path.exists(log_path):
        return energies

    with open(log_path) as f:
        for line in f:
            m = _MODE_RE.match(line)
            if m:
                energies.append((
                    float(m.group(2)),
                    float(m.group(3)),
                    float(m.group(4)),
                ))

    return energies


def _split_pdbqt_models(pdbqt_text):
    """Split a multi-model PDBQT string into individual model blocks."""
    models = []
    current = []
    for line in pdbqt_text.splitlines():
        if line.startswith("MODEL"):
            current = [line]
        elif line.startswith("ENDMDL"):
            current.append(line)
            models.append("\n".join(current))
            current = []
        else:
            current.append(line)

    # If no MODEL records, treat the entire text as a single pose
    if not models and current:
        models.append("\n".join(current))

    return models


# ---------------------------------------------------------------------------
# Dev-mode entrypoint
# ---------------------------------------------------------------------------
if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8000, debug=True)
