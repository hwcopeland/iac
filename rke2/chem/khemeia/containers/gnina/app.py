"""Gnina CNN-scored docking engine sidecar.

Exposes a Flask API with a POST /dock endpoint that runs Gnina (CNN-scored
docking, GPU-accelerated).  The request/response contract is identical across
all WP-3 docking engines so the Go API can call any engine interchangeably.

Gnina-specific: returns additional cnn_score and cnn_affinity fields per pose.
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

GNINA_BIN = "/usr/local/bin/gnina"

# Gnina SDF output properties that contain scores
_CNN_SCORE_PROP = "CNNscore"
_CNN_AFFINITY_PROP = "CNNaffinity"
_VINA_AFFINITY_PROP = "minimizedAffinity"
_RMSD_LB_PROP = "minimizedRMSD"

# Matches result lines in Gnina log output (Vina-like format):
#   "   1       -7.1      0.000      0.000   0.543   -6.23"
_MODE_RE = re.compile(
    r"^\s+(\d+)\s+([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)"
)
# Fallback for lines without CNN scores (plain Vina output):
_MODE_VINA_RE = re.compile(
    r"^\s+(\d+)\s+([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)"
)


# ---------------------------------------------------------------------------
# Health endpoints
# ---------------------------------------------------------------------------

@app.route("/health", methods=["GET"])
def health():
    return jsonify({"status": "ok"})


@app.route("/readyz", methods=["GET"])
def readyz():
    """Readiness probe -- verify gnina binary is available and GPU is accessible."""
    if not (os.path.isfile(GNINA_BIN) and os.access(GNINA_BIN, os.X_OK)):
        return jsonify({"status": "not_ready", "error": "gnina binary not found"}), 503

    # Quick GPU check via gnina --version (it logs CUDA availability)
    try:
        result = subprocess.run(
            [GNINA_BIN, "--version"],
            capture_output=True, text=True, timeout=10,
        )
        version_info = result.stdout + result.stderr
        return jsonify({"status": "ready", "version": version_info.strip()})
    except Exception as exc:
        return jsonify({"status": "not_ready", "error": str(exc)}), 503


# ---------------------------------------------------------------------------
# POST /dock
# ---------------------------------------------------------------------------

@app.route("/dock", methods=["POST"])
def dock():
    """Run Gnina docking via CLI.

    Request JSON:
        receptor_pdbqt (str):   Receptor in PDBQT format.
        ligand_pdbqt (str):     Ligand in PDBQT format.
        center (list[float]):   Binding-site center [x, y, z].
        size (list[float]):     Box dimensions [sx, sy, sz].
        exhaustiveness (int):   Search exhaustiveness (default 32).
        scoring (str):          Scoring function (default "default"; Gnina uses
                                its own CNN+Vina hybrid by default).
        cnn_model (str):        CNN model to use (default "default").
        n_poses (int):          Number of poses to return (default 9).

    Response JSON:
        poses (list[dict]):   Each pose has: pdbqt, affinity, rmsd_lb, rmsd_ub,
                              cnn_score, cnn_affinity.
        engine (str):         "gnina"
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
    scoring = data.get("scoring", "default")
    cnn_model = data.get("cnn_model", "default")
    n_poses = int(data.get("n_poses", 9))

    try:
        poses = _run_gnina(
            receptor_pdbqt, ligand_pdbqt, center, size,
            exhaustiveness, scoring, cnn_model, n_poses,
        )
        return jsonify({
            "poses": poses,
            "engine": "gnina",
            "scoring": scoring,
        })
    except Exception as exc:
        logger.error("Docking failed: %s\n%s", exc, traceback.format_exc())
        return jsonify({"error": str(exc)}), 500


# ---------------------------------------------------------------------------
# Gnina execution
# ---------------------------------------------------------------------------

def _run_gnina(receptor_pdbqt, ligand_pdbqt, center, size,
               exhaustiveness, scoring, cnn_model, n_poses):
    """Execute Gnina docking and return parsed pose list."""
    tmpdir = tempfile.mkdtemp(prefix="gnina_")
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
            GNINA_BIN,
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
            "--out", out_path,
            "--log", log_path,
        ]

        # CNN model override
        if cnn_model and cnn_model != "default":
            cmd.extend(["--cnn", cnn_model])

        result = subprocess.run(
            cmd, capture_output=True, text=True, timeout=600,
        )
        if result.returncode != 0:
            raise RuntimeError(
                f"Gnina exited with code {result.returncode}: {result.stderr}"
            )

        # Parse results from log (includes CNN scores)
        energies = _parse_log(log_path)

        poses_pdbqt = ""
        if os.path.exists(out_path):
            with open(out_path) as f:
                poses_pdbqt = f.read()

        pose_blocks = _split_pdbqt_models(poses_pdbqt)

        poses = []
        for i, block in enumerate(pose_blocks):
            energy_row = energies[i] if i < len(energies) else {}
            poses.append({
                "pdbqt": block,
                "affinity": energy_row.get("affinity", 0.0),
                "rmsd_lb": energy_row.get("rmsd_lb", 0.0),
                "rmsd_ub": energy_row.get("rmsd_ub", 0.0),
                "cnn_score": energy_row.get("cnn_score", 0.0),
                "cnn_affinity": energy_row.get("cnn_affinity", 0.0),
            })

        logger.info(
            "Docking complete: %d poses, best affinity %.2f kcal/mol, "
            "best CNN score %.3f",
            len(poses),
            poses[0]["affinity"] if poses else 0.0,
            poses[0]["cnn_score"] if poses else 0.0,
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
    """Parse Gnina log file for pose energies and CNN scores.

    Gnina log format (after the header):
        mode | affinity | rmsd_lb | rmsd_ub | CNN_score | CNN_affinity
          1     -7.1       0.000    0.000      0.543      -6.23

    Returns list of dicts with keys: affinity, rmsd_lb, rmsd_ub,
    cnn_score, cnn_affinity.
    """
    energies = []
    if not os.path.exists(log_path):
        return energies

    with open(log_path) as f:
        for line in f:
            # Try full Gnina format first (with CNN scores)
            m = _MODE_RE.match(line)
            if m:
                energies.append({
                    "affinity": float(m.group(2)),
                    "rmsd_lb": float(m.group(3)),
                    "rmsd_ub": float(m.group(4)),
                    "cnn_score": float(m.group(5)),
                    "cnn_affinity": float(m.group(6)),
                })
                continue

            # Fallback: Vina-like format without CNN scores
            m = _MODE_VINA_RE.match(line)
            if m:
                energies.append({
                    "affinity": float(m.group(2)),
                    "rmsd_lb": float(m.group(3)),
                    "rmsd_ub": float(m.group(4)),
                    "cnn_score": 0.0,
                    "cnn_affinity": 0.0,
                })

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
