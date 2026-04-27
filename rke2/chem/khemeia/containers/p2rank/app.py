"""P2Rank pocket prediction service for WP-1 binding-site definition.

Accepts a cleaned PDB structure, runs P2Rank predict, and returns
predicted pockets with probabilities, centers, sizes, and residue lists.
"""

import csv
import logging
import os
import re
import shutil
import subprocess
import tempfile

import numpy as np
from flask import Flask, jsonify, request

app = Flask(__name__)
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

P2RANK_HOME = os.environ.get("P2RANK_HOME", "/opt/p2rank")
PRANK_BIN = os.path.join(P2RANK_HOME, "prank")


# ---------------------------------------------------------------------------
# Health endpoints
# ---------------------------------------------------------------------------

@app.route("/health", methods=["GET"])
def health():
    return jsonify({"status": "ok"})


@app.route("/readyz", methods=["GET"])
def readyz():
    """Readiness check -- verify P2Rank binary is available."""
    if not os.path.isfile(PRANK_BIN):
        return jsonify({"status": "not ready", "error": "prank not found"}), 503
    return jsonify({"status": "ready"})


# ---------------------------------------------------------------------------
# Main endpoint
# ---------------------------------------------------------------------------

@app.route("/predict", methods=["POST"])
def predict():
    """Run P2Rank on a PDB structure and return predicted pockets.

    Request body:
        {"pdb_data": "<PDB file contents as string>"}

    Response:
        {"pockets": [{"rank": 1, "probability": 0.92, ...}, ...]}
    """
    data = request.get_json(force=True)
    pdb_data = data.get("pdb_data")

    if not pdb_data:
        return jsonify({"error": "pdb_data is required"}), 400

    try:
        pockets = _run_p2rank(pdb_data)
        return jsonify({"pockets": pockets})
    except subprocess.CalledProcessError as exc:
        logger.exception("P2Rank execution failed")
        return jsonify({
            "error": "P2Rank execution failed",
            "detail": exc.stderr or str(exc),
        }), 500
    except Exception as exc:
        logger.exception("Pocket prediction failed")
        return jsonify({"error": str(exc)}), 500


# ---------------------------------------------------------------------------
# P2Rank execution and output parsing
# ---------------------------------------------------------------------------

def _run_p2rank(pdb_data: str) -> list[dict]:
    """Write PDB to tempdir, run P2Rank predict, parse results."""
    work_dir = tempfile.mkdtemp(prefix="p2rank_")
    pdb_path = os.path.join(work_dir, "input.pdb")
    out_dir = os.path.join(work_dir, "output")

    try:
        with open(pdb_path, "w") as f:
            f.write(pdb_data)

        # Run P2Rank predict
        result = subprocess.run(
            [
                PRANK_BIN, "predict",
                "-f", pdb_path,
                "-o", out_dir,
                "-threads", "2",
            ],
            capture_output=True,
            text=True,
            timeout=300,
            check=True,
        )
        logger.info("P2Rank stdout: %s", result.stdout[:500] if result.stdout else "")

        if not os.path.isdir(out_dir):
            raise FileNotFoundError(
                f"P2Rank output directory not found at {out_dir}. "
                f"Contents of work_dir: {os.listdir(work_dir)}"
            )

        return _parse_p2rank_output(out_dir, pdb_path)
    finally:
        shutil.rmtree(work_dir, ignore_errors=True)


def _parse_p2rank_output(out_dir: str, pdb_path: str) -> list[dict]:
    """Parse P2Rank output CSV into structured pocket data.

    P2Rank creates a predictions CSV file with columns:
        name, rank, score, probability,
        center_x, center_y, center_z,
        residue_ids, surf_atom_ids, ...

    It also creates per-pocket residue files for detailed residue lists.
    """
    # Find the predictions file
    predictions_file = _find_predictions_file(out_dir)
    if predictions_file is None:
        logger.warning("No predictions file found in P2Rank output")
        return []

    pockets = []

    with open(predictions_file, newline="") as csvfile:
        reader = csv.DictReader(csvfile)
        # P2Rank CSV headers have leading spaces -- strip them
        if reader.fieldnames:
            reader.fieldnames = [f.strip() for f in reader.fieldnames]

        for row in reader:
            # Strip whitespace from all values
            row = {k.strip(): v.strip() for k, v in row.items()}

            try:
                rank = int(row.get("rank", 0))
                score = float(row.get("score", 0.0))
                probability = float(row.get("probability", 0.0))
                center_x = float(row.get("center_x", 0.0))
                center_y = float(row.get("center_y", 0.0))
                center_z = float(row.get("center_z", 0.0))
            except (ValueError, TypeError) as exc:
                logger.warning("Failed to parse P2Rank row: %s (%s)", row, exc)
                continue

            # Parse residue IDs -- P2Rank format: "ALA_123_A SER_45_B ..."
            raw_residues = row.get("residue_ids", "")
            residues = _parse_residue_ids(raw_residues)

            # Estimate pocket size from residue spread
            # P2Rank does not directly output pocket dimensions, so we
            # compute a bounding box from the pocket residue coordinates
            # if a residue-level file is available; otherwise use a
            # reasonable default based on the number of residues.
            size = _estimate_pocket_size(
                out_dir, rank, len(residues), pdb_path,
            )

            pockets.append({
                "rank": rank,
                "score": round(score, 4),
                "probability": round(probability, 4),
                "center": [
                    round(center_x, 3),
                    round(center_y, 3),
                    round(center_z, 3),
                ],
                "size": size,
                "residues": residues,
            })

    return pockets


def _find_predictions_file(out_dir: str) -> str | None:
    """Locate the P2Rank predictions CSV in the output directory.

    P2Rank names it: input.pdb_predictions.csv
    """
    for root, _dirs, files in os.walk(out_dir):
        for f in files:
            if f.endswith("_predictions.csv"):
                return os.path.join(root, f)
    return None


def _parse_residue_ids(raw: str) -> list[str]:
    """Convert P2Rank residue ID format to our standard format.

    P2Rank uses: "ALA_123_A SER_45_B" (underscore-separated)
    We output:   "ALA123.A" (compact with dot chain separator)
    """
    if not raw:
        return []

    residues = []
    for token in raw.split():
        token = token.strip()
        if not token:
            continue
        # Expected: NAME_SEQ_CHAIN
        parts = token.split("_")
        if len(parts) >= 3:
            res_name = parts[0]
            res_seq = parts[1]
            chain_id = parts[2]
            residues.append(f"{res_name}{res_seq}.{chain_id}")
        elif len(parts) == 2:
            # Fallback: NAME_SEQ (no chain)
            residues.append(f"{parts[0]}{parts[1]}.A")
        else:
            residues.append(token)

    return sorted(set(residues))


def _estimate_pocket_size(
    out_dir: str,
    rank: int,
    residue_count: int,
    pdb_path: str,
) -> list[float]:
    """Estimate pocket dimensions.

    Tries to read the per-pocket residue file for coordinate-based sizing.
    Falls back to a heuristic based on residue count.
    """
    # Try to find per-pocket residue PDB/visualization file
    for root, _dirs, files in os.walk(out_dir):
        for f in files:
            if f"pocket{rank}" in f and f.endswith(".pdb"):
                pocket_path = os.path.join(root, f)
                coords = _extract_coords_from_pdb(pocket_path)
                if len(coords) > 0:
                    arr = np.array(coords)
                    dims = (arr.max(axis=0) - arr.min(axis=0)).tolist()
                    return [round(d, 3) for d in dims]

    # Heuristic fallback: typical pocket size scales with residue count
    # Average residue contributes ~3-4 A of pocket extent
    estimate = max(8.0, residue_count * 0.8)
    return [round(estimate, 3)] * 3


def _extract_coords_from_pdb(pdb_path: str) -> list[list[float]]:
    """Extract atom coordinates from a PDB file."""
    coords: list[list[float]] = []
    try:
        with open(pdb_path) as fh:
            for line in fh:
                if not line.startswith(("ATOM", "HETATM")):
                    continue
                try:
                    x = float(line[30:38])
                    y = float(line[38:46])
                    z = float(line[46:54])
                    coords.append([x, y, z])
                except (ValueError, IndexError):
                    continue
    except OSError:
        pass
    return coords


# ---------------------------------------------------------------------------
# Dev-mode entrypoint
# ---------------------------------------------------------------------------
if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8000, debug=True)
