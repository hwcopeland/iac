"""Smina docking engine sidecar.

Exposes a Flask API with a POST /dock endpoint that runs Smina (Vina fork
with custom scoring terms and flexible residue support).  The request/response
contract is identical across all WP-3 docking engines so the Go API can call
any engine interchangeably.

Smina-specific: supports custom scoring_terms parameter.
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

SMINA_BIN = "/usr/local/bin/smina"

# Matches result lines in Smina output:
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
    """Readiness probe -- verify smina binary is available."""
    if os.path.isfile(SMINA_BIN) and os.access(SMINA_BIN, os.X_OK):
        return jsonify({"status": "ready"})
    return jsonify({"status": "not_ready", "error": "smina binary not found"}), 503


# ---------------------------------------------------------------------------
# POST /dock
# ---------------------------------------------------------------------------

@app.route("/dock", methods=["POST"])
def dock():
    """Run Smina docking via CLI.

    Request JSON:
        receptor_pdbqt (str):       Receptor in PDBQT format.
        ligand_pdbqt (str):         Ligand in PDBQT format.
        center (list[float]):       Binding-site center [x, y, z].
        size (list[float]):         Box dimensions [sx, sy, sz].
        exhaustiveness (int):       Search exhaustiveness (default 32).
        scoring (str):              Scoring function (default "vina"; Smina also
                                    supports "vinardo").
        scoring_terms (list[str]):  Smina-specific custom scoring terms.
        n_poses (int):              Number of poses to return (default 9).

    Response JSON:
        poses (list[dict]):   Each pose has: pdbqt, affinity, rmsd_lb, rmsd_ub.
        engine (str):         "smina"
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
    scoring_terms = data.get("scoring_terms", [])
    n_poses = int(data.get("n_poses", 9))

    try:
        poses = _run_smina(
            receptor_pdbqt, ligand_pdbqt, center, size,
            exhaustiveness, scoring, scoring_terms, n_poses,
        )
        return jsonify({
            "poses": poses,
            "engine": "smina",
            "scoring": scoring,
        })
    except Exception as exc:
        logger.error("Docking failed: %s\n%s", exc, traceback.format_exc())
        return jsonify({"error": str(exc)}), 500


# ---------------------------------------------------------------------------
# Smina execution
# ---------------------------------------------------------------------------

def _run_smina(receptor_pdbqt, ligand_pdbqt, center, size,
               exhaustiveness, scoring, scoring_terms, n_poses):
    """Execute Smina docking and return parsed pose list."""
    tmpdir = tempfile.mkdtemp(prefix="smina_")
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
            SMINA_BIN,
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

        # Smina scoring function override
        if scoring and scoring != "vina":
            cmd.extend(["--scoring", scoring])

        # Smina-specific custom scoring terms
        for term in scoring_terms:
            cmd.extend(["--custom_scoring", term])

        result = subprocess.run(
            cmd, capture_output=True, text=True, timeout=300,
        )
        if result.returncode != 0:
            raise RuntimeError(
                f"Smina exited with code {result.returncode}: {result.stderr}"
            )

        # Parse results
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
    """Parse Smina log file for pose energies.

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
