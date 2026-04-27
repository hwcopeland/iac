"""AutoDock Vina 1.2 docking engine sidecar.

Exposes a Flask API with a POST /dock endpoint that runs Vina 1.2 via
its Python bindings.  The request/response contract is identical across
all WP-3 docking engines so the Go API can call any engine interchangeably.

Scoring functions: vina (default), vinardo, ad4.
"""

import logging
import os
import tempfile
import traceback

from flask import Flask, jsonify, request

app = Flask(__name__)
logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
logger = logging.getLogger(__name__)

# Valid scoring functions for Vina 1.2
_VALID_SCORING = {"vina", "vinardo", "ad4"}


# ---------------------------------------------------------------------------
# Health endpoints
# ---------------------------------------------------------------------------

@app.route("/health", methods=["GET"])
def health():
    return jsonify({"status": "ok"})


@app.route("/readyz", methods=["GET"])
def readyz():
    """Readiness probe -- verify Vina Python bindings are importable."""
    try:
        from vina import Vina  # noqa: F401
        return jsonify({"status": "ready"})
    except ImportError as exc:
        return jsonify({"status": "not_ready", "error": str(exc)}), 503


# ---------------------------------------------------------------------------
# POST /dock
# ---------------------------------------------------------------------------

@app.route("/dock", methods=["POST"])
def dock():
    """Run Vina 1.2 docking via Python bindings.

    Request JSON:
        receptor_pdbqt (str): Receptor in PDBQT format.
        ligand_pdbqt (str):   Ligand in PDBQT format.
        center (list[float]): Binding-site center [x, y, z].
        size (list[float]):   Box dimensions [sx, sy, sz].
        exhaustiveness (int): Search exhaustiveness (default 32).
        scoring (str):        Scoring function: vina, vinardo, ad4 (default vina).
        n_poses (int):        Number of poses to return (default 9).

    Response JSON:
        poses (list[dict]):   Each pose has: pdbqt, affinity, rmsd_lb, rmsd_ub.
        engine (str):         "vina-1.2"
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

    if scoring not in _VALID_SCORING:
        return jsonify({
            "error": f"invalid scoring function '{scoring}', must be one of: {sorted(_VALID_SCORING)}",
        }), 400

    try:
        poses = _run_vina(
            receptor_pdbqt, ligand_pdbqt, center, size,
            exhaustiveness, scoring, n_poses,
        )
        return jsonify({
            "poses": poses,
            "engine": "vina-1.2",
            "scoring": scoring,
        })
    except Exception as exc:
        logger.error("Docking failed: %s\n%s", exc, traceback.format_exc())
        return jsonify({"error": str(exc)}), 500


# ---------------------------------------------------------------------------
# Vina execution
# ---------------------------------------------------------------------------

def _run_vina(receptor_pdbqt, ligand_pdbqt, center, size,
              exhaustiveness, scoring, n_poses):
    """Execute Vina 1.2 docking and return parsed pose list."""
    from vina import Vina

    v = Vina(sf_name=scoring)

    # Vina needs receptor as a file path
    with tempfile.NamedTemporaryFile(
        suffix=".pdbqt", mode="w", delete=False,
    ) as rec_f:
        rec_f.write(receptor_pdbqt)
        rec_path = rec_f.name

    try:
        v.set_receptor(rec_path)
    finally:
        os.unlink(rec_path)

    # Ligand can be set from string
    v.set_ligand_from_string(ligand_pdbqt)

    # Compute affinity maps
    v.compute_vina_maps(
        center=[float(c) for c in center],
        box_size=[float(s) for s in size],
    )

    # Run docking
    v.dock(exhaustiveness=exhaustiveness, n_poses=n_poses)

    # Extract results -- energies() returns a 2D array:
    #   [[affinity, rmsd_lb, rmsd_ub], ...]
    energies = v.energies()
    poses_pdbqt = v.poses()

    # Split multi-model PDBQT into individual poses
    pose_blocks = _split_pdbqt_models(poses_pdbqt)

    poses = []
    for i, block in enumerate(pose_blocks):
        energy_row = energies[i] if i < len(energies) else [0.0, 0.0, 0.0]
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
