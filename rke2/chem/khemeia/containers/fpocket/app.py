"""fpocket pocket detection service for WP-1 binding-site definition.

Accepts a cleaned PDB structure, runs fpocket, and returns detected
pockets with druggability scores, centers, sizes, and residue lists.
"""

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


# ---------------------------------------------------------------------------
# Health endpoints
# ---------------------------------------------------------------------------

@app.route("/health", methods=["GET"])
def health():
    return jsonify({"status": "ok"})


@app.route("/readyz", methods=["GET"])
def readyz():
    """Readiness check -- verify fpocket binary is available."""
    if shutil.which("fpocket") is None:
        return jsonify({"status": "not ready", "error": "fpocket not found"}), 503
    return jsonify({"status": "ready"})


# ---------------------------------------------------------------------------
# Main endpoint
# ---------------------------------------------------------------------------

@app.route("/detect", methods=["POST"])
def detect():
    """Run fpocket on a PDB structure and return detected pockets.

    Request body:
        {"pdb_data": "<PDB file contents as string>"}

    Response:
        {"pockets": [{"rank": 1, "druggability_score": 0.87, ...}, ...]}
    """
    data = request.get_json(force=True)
    pdb_data = data.get("pdb_data")

    if not pdb_data:
        return jsonify({"error": "pdb_data is required"}), 400

    try:
        pockets = _run_fpocket(pdb_data)
        return jsonify({"pockets": pockets})
    except subprocess.CalledProcessError as exc:
        logger.exception("fpocket execution failed")
        return jsonify({
            "error": "fpocket execution failed",
            "detail": exc.stderr or str(exc),
        }), 500
    except Exception as exc:
        logger.exception("Pocket detection failed")
        return jsonify({"error": str(exc)}), 500


# ---------------------------------------------------------------------------
# fpocket execution and output parsing
# ---------------------------------------------------------------------------

def _run_fpocket(pdb_data: str) -> list[dict]:
    """Write PDB to tempdir, run fpocket, parse results."""
    work_dir = tempfile.mkdtemp(prefix="fpocket_")
    pdb_path = os.path.join(work_dir, "input.pdb")

    try:
        with open(pdb_path, "w") as f:
            f.write(pdb_data)

        # Run fpocket -- outputs go to input_out/ subdirectory
        result = subprocess.run(
            ["fpocket", "-f", pdb_path],
            capture_output=True,
            text=True,
            timeout=120,
            check=True,
        )
        logger.info("fpocket stdout: %s", result.stdout[:500] if result.stdout else "")

        output_dir = os.path.join(work_dir, "input_out")
        if not os.path.isdir(output_dir):
            raise FileNotFoundError(
                f"fpocket output directory not found at {output_dir}. "
                f"Contents of work_dir: {os.listdir(work_dir)}"
            )

        return _parse_fpocket_output(output_dir)
    finally:
        shutil.rmtree(work_dir, ignore_errors=True)


def _parse_fpocket_output(output_dir: str) -> list[dict]:
    """Parse fpocket output directory into structured pocket data.

    fpocket creates:
    - pockets/ directory with pocket PDB files (pocket0_atm.pdb, etc.)
    - input_info.txt with summary scores per pocket
    - input_pockets.pqr with all pockets
    """
    pockets = []

    # Parse the info file for druggability scores and metadata
    info_scores = _parse_info_file(output_dir)

    # Parse individual pocket PDB files for coordinates and residues
    pockets_dir = os.path.join(output_dir, "pockets")
    if not os.path.isdir(pockets_dir):
        logger.warning("No pockets directory found in fpocket output")
        return pockets

    pocket_files = sorted(
        f for f in os.listdir(pockets_dir) if f.endswith("_atm.pdb")
    )

    for i, pocket_file in enumerate(pocket_files):
        pocket_path = os.path.join(pockets_dir, pocket_file)
        coords, residues = _parse_pocket_pdb(pocket_path)

        if len(coords) == 0:
            continue

        coords_arr = np.array(coords)
        center = coords_arr.mean(axis=0).tolist()
        mins = coords_arr.min(axis=0)
        maxs = coords_arr.max(axis=0)
        size = (maxs - mins).tolist()
        volume = float(np.prod(maxs - mins))

        # Pocket numbering in fpocket is 1-based
        pocket_num = i + 1
        scores = info_scores.get(pocket_num, {})

        pockets.append({
            "rank": pocket_num,
            "druggability_score": scores.get("druggability_score", 0.0),
            "center": [round(c, 3) for c in center],
            "size": [round(s, 3) for s in size],
            "volume": round(volume, 2),
            "residues": sorted(set(residues)),
        })

    # Sort by druggability score descending, re-assign ranks
    pockets.sort(key=lambda p: p["druggability_score"], reverse=True)
    for rank, pocket in enumerate(pockets, 1):
        pocket["rank"] = rank

    return pockets


def _parse_info_file(output_dir: str) -> dict[int, dict]:
    """Parse fpocket *_info.txt for per-pocket druggability scores.

    The info file contains blocks like:
        Pocket 1 :
            Score :          0.4532
            Druggability Score :          0.872
            Number of Alpha Spheres :         42
            ...
    """
    scores: dict[int, dict] = {}

    # Find the info file -- named input_info.txt
    info_file = None
    for f in os.listdir(output_dir):
        if f.endswith("_info.txt"):
            info_file = os.path.join(output_dir, f)
            break

    if info_file is None:
        logger.warning("No info file found in fpocket output")
        return scores

    with open(info_file) as fh:
        content = fh.read()

    # Split into pocket blocks
    pocket_blocks = re.split(r"Pocket\s+(\d+)\s*:", content)
    # pocket_blocks alternates: [preamble, "1", block1, "2", block2, ...]
    for i in range(1, len(pocket_blocks) - 1, 2):
        pocket_num = int(pocket_blocks[i])
        block = pocket_blocks[i + 1]

        pocket_scores: dict[str, float] = {}

        # Extract druggability score
        drug_match = re.search(
            r"Druggability\s+Score\s*:\s*([\d.]+)", block
        )
        if drug_match:
            pocket_scores["druggability_score"] = float(drug_match.group(1))

        # Extract fpocket raw score
        score_match = re.search(r"^\s*Score\s*:\s*([\d.]+)", block, re.MULTILINE)
        if score_match:
            pocket_scores["score"] = float(score_match.group(1))

        # Extract volume if present
        vol_match = re.search(
            r"Real\s+volume\s*\(.*?\)\s*:\s*([\d.]+)", block
        )
        if vol_match:
            pocket_scores["real_volume"] = float(vol_match.group(1))

        scores[pocket_num] = pocket_scores

    return scores


def _parse_pocket_pdb(pocket_path: str) -> tuple[list[list[float]], list[str]]:
    """Extract atom coordinates and residue identifiers from a pocket PDB file.

    Returns:
        (coords, residues) where coords is a list of [x, y, z] and residues
        is a list of residue identifiers like "ALA123.A".
    """
    coords: list[list[float]] = []
    residues: list[str] = []

    with open(pocket_path) as fh:
        for line in fh:
            if not line.startswith(("ATOM", "HETATM")):
                continue

            try:
                x = float(line[30:38])
                y = float(line[38:46])
                z = float(line[46:54])
                coords.append([x, y, z])

                # Extract residue info: name (cols 17-20), seq (cols 22-26),
                # chain (col 21)
                res_name = line[17:20].strip()
                res_seq = line[22:26].strip()
                chain_id = line[21].strip() or "A"
                residue_id = f"{res_name}{res_seq}.{chain_id}"
                residues.append(residue_id)
            except (ValueError, IndexError):
                continue

    return coords, residues


# ---------------------------------------------------------------------------
# Dev-mode entrypoint
# ---------------------------------------------------------------------------
if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8000, debug=True)
