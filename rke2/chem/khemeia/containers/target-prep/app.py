"""Target preparation sidecar for the Khemeia SBDD pipeline.

Cleans receptor PDB structures via PDBFixer (remove waters, add missing
heavy atoms, optionally add hydrogens at specified pH) and defines binding-site
boxes using native-ligand extraction or user-specified coordinates.

Replaces the embedded receptor prep in containers/autodock-vina/scripts/proteinprepv2.py
with a standalone, reusable service.
"""

import io
import logging
import os
import tempfile
from typing import Any

import numpy as np
import requests as http_requests
from Bio.PDB import PDBParser
from flask import Flask, jsonify, request
from pdbfixer import PDBFixer

# OpenMM >= 7.6 uses the top-level 'openmm' package; older versions use
# 'simtk.openmm'.  PDBFixer pulls in the correct one, so we try both.
try:
    from openmm.app import PDBFile
except ImportError:
    from simtk.openmm.app import PDBFile

app = Flask(__name__)
logging.basicConfig(level=logging.INFO)

# Default co-factors to retain during heteroatom stripping.
DEFAULT_KEEP_COFACTORS = frozenset({"ZN", "MG", "CA", "FE"})

# RCSB PDB download URL template.
RCSB_URL = "https://files.rcsb.org/download/{pdb_id}.pdb"


# ---------------------------------------------------------------------------
# PDB fetch
# ---------------------------------------------------------------------------

def fetch_pdb(pdb_id: str) -> str:
    """Download a PDB file from RCSB by ID.

    Returns the raw PDB text.  Raises ValueError on HTTP errors.
    """
    url = RCSB_URL.format(pdb_id=pdb_id.upper())
    resp = http_requests.get(url, timeout=30)
    if resp.status_code != 200:
        raise ValueError(
            f"Failed to fetch PDB {pdb_id} from RCSB "
            f"(HTTP {resp.status_code})"
        )
    return resp.text


# ---------------------------------------------------------------------------
# PDBFixer-based receptor cleaning
# ---------------------------------------------------------------------------

def clean_receptor(
    pdb_text: str,
    ph: float = 7.4,
    keep_cofactors: frozenset[str] | None = None,
    add_hydrogens: bool = False,
    quick_clean: bool = True,
) -> tuple[str, dict[str, int]]:
    """Clean a receptor structure using PDBFixer.

    Steps:
      1. Remove water molecules.
      2. Remove heteroatoms except those in *keep_cofactors*.
      3. Find and add missing residues / heavy atoms.
      4. (Optional) Add hydrogens at the specified pH.

    Hydrogen addition is off by default because docking engines (Vina, Smina)
    add their own protons, and OpenMM's addMissingHydrogens can take >5 min
    on large proteins (e.g. 7JRN), causing worker timeouts.

    Returns:
        (cleaned_pdb_text, stats_dict) where stats_dict contains counts of
        atoms removed, hydrogens added, and missing atoms fixed.
    """
    if keep_cofactors is None:
        keep_cofactors = DEFAULT_KEEP_COFACTORS

    # 1. Remove water molecules by stripping HOH HETATM/ATOM lines.
    # PDBFixer does not expose a "remove waters only" method, so we filter
    # the raw PDB text before loading into PDBFixer.
    lines_no_water = [
        line for line in pdb_text.splitlines()
        if not (
            line.startswith(("HETATM", "ATOM"))
            and len(line) >= 20
            and line[17:20].strip() == "HOH"
        )
    ]
    waters_removed = _count_atoms_in_lines(pdb_text, "HOH")

    # 2. Remove non-cofactor heteroatoms.
    # Build a cleaned PDB that keeps only ATOM lines and HETATM lines for
    # residues in the keep list.
    cleaned_lines = []
    het_atoms_removed = 0
    for line in lines_no_water:
        if line.startswith("HETATM"):
            resname = line[17:20].strip()
            if resname in keep_cofactors:
                cleaned_lines.append(line)
            else:
                het_atoms_removed += 1
        else:
            cleaned_lines.append(line)

    cleaned_pdb_text = "\n".join(cleaned_lines) + "\n"

    # Re-load the cleaned structure into PDBFixer for heavy-atom and H repair.
    fixer = PDBFixer(pdbfile=io.StringIO(cleaned_pdb_text))

    # 3. Find and add missing heavy atoms (and optionally missing residues).
    # quick_clean=True (default) skips this — PDBFixer's missing-atom detection
    # is very slow on large proteins (>2000 residues) because it iterates all
    # residue templates. For well-resolved X-ray structures, this step is
    # usually a no-op anyway. Set quick_clean=False for NMR or low-resolution
    # structures that may have missing atoms.
    missing_atoms_count = 0
    if not quick_clean:
        fixer.findMissingResidues()
        fixer.findMissingAtoms()
        missing_atoms_count = sum(
            len(atoms) for atoms in fixer.missingAtoms.values()
        )
        fixer.addMissingAtoms()

    # 4. Optionally add hydrogens at the target pH.
    hydrogens_added = 0
    if add_hydrogens:
        atoms_before_h = sum(1 for _ in fixer.topology.atoms())
        fixer.addMissingHydrogens(ph)
        atoms_after_h = sum(1 for _ in fixer.topology.atoms())
        hydrogens_added = atoms_after_h - atoms_before_h

    # Serialise the cleaned structure to PDB text.
    out = io.StringIO()
    PDBFile.writeFile(fixer.topology, fixer.positions, out)
    receptor_pdb = out.getvalue()

    stats = {
        "atoms_removed": waters_removed + het_atoms_removed,
        "waters_removed": waters_removed,
        "het_atoms_removed": het_atoms_removed,
        "hydrogens_added": hydrogens_added,
        "missing_atoms_fixed": missing_atoms_count,
    }
    return receptor_pdb, stats


def _count_atoms_in_lines(pdb_text: str, resname: str) -> int:
    """Count ATOM/HETATM lines matching a given residue name."""
    count = 0
    for line in pdb_text.splitlines():
        if line.startswith(("ATOM", "HETATM")) and len(line) >= 20:
            if line[17:20].strip() == resname:
                count += 1
    return count


# ---------------------------------------------------------------------------
# Binding-site extraction
# ---------------------------------------------------------------------------

def binding_site_from_native_ligand(
    pdb_text: str,
    ligand_id: str,
    padding: float = 10.0,
) -> dict[str, list[float]]:
    """Compute binding-site box from the centroid of a native ligand.

    Uses BioPython to parse the *original* (pre-cleaning) PDB so the ligand
    is still present.  Returns {"center": [x,y,z], "size": [sx,sy,sz]}.
    """
    parser = PDBParser(QUIET=True)
    with tempfile.NamedTemporaryFile(
        suffix=".pdb", mode="w", delete=False,
    ) as tmp:
        tmp.write(pdb_text)
        tmp_path = tmp.name

    try:
        structure = parser.get_structure("receptor", tmp_path)

        residues: list[tuple[tuple[int, str, tuple[Any, int, str]], np.ndarray]] = []
        for model in structure:
            for chain in model:
                for residue in chain:
                    if residue.get_resname().strip() == ligand_id:
                        coords = np.array(
                            [atom.get_vector().get_array() for atom in residue.get_atoms()],
                        )
                        if len(coords) == 0:
                            continue
                        residue_key = (model.id, chain.id, residue.id)
                        residues.append((residue_key, coords))

        if not residues:
            raise ValueError(
                f"Ligand '{ligand_id}' not found in PDB structure. "
                f"Check the residue name (case-sensitive, 3-letter code)."
            )

        # Multiple instances of the same ligand residue can exist in symmetric
        # assemblies. Native-ligand mode should choose one binding site, not the
        # union of every copy across the structure.
        _, coords_arr = residues[0]
        centroid = coords_arr.mean(axis=0).tolist()

        # Box size: range of ligand coordinates + padding on each side.
        extent = coords_arr.max(axis=0) - coords_arr.min(axis=0)
        size = (extent + 2 * padding).tolist()

        return {
            "center": [round(c, 3) for c in centroid],
            "size": [round(s, 3) for s in size],
        }
    finally:
        os.unlink(tmp_path)


def binding_site_from_custom_box(
    center: list[float],
    size: list[float],
    receptor_pdb: str,
) -> dict[str, list[float]]:
    """Validate a user-provided custom box against the receptor.

    Verifies the box intersects the receptor bounding box.  Returns the box
    specification unchanged if valid, raises ValueError otherwise.
    """
    parser = PDBParser(QUIET=True)
    with tempfile.NamedTemporaryFile(
        suffix=".pdb", mode="w", delete=False,
    ) as tmp:
        tmp.write(receptor_pdb)
        tmp_path = tmp.name

    try:
        structure = parser.get_structure("receptor", tmp_path)

        rec_coords: list[np.ndarray] = []
        for model in structure:
            for chain in model:
                for residue in chain:
                    for atom in residue.get_atoms():
                        rec_coords.append(atom.get_vector().get_array())

        if not rec_coords:
            raise ValueError("Receptor structure contains no atoms")

        rec_arr = np.array(rec_coords)
        rec_min = rec_arr.min(axis=0)
        rec_max = rec_arr.max(axis=0)

        box_center = np.array(center)
        box_half = np.array(size) / 2.0
        box_min = box_center - box_half
        box_max = box_center + box_half

        # Check axis-aligned bounding box intersection.
        if np.any(box_max < rec_min) or np.any(box_min > rec_max):
            raise ValueError(
                "Custom box does not intersect the receptor bounding box. "
                f"Receptor spans [{rec_min.tolist()}, {rec_max.tolist()}]; "
                f"box spans [{box_min.tolist()}, {box_max.tolist()}]."
            )

        return {
            "center": [round(c, 3) for c in center],
            "size": [round(s, 3) for s in size],
        }
    finally:
        os.unlink(tmp_path)


# ---------------------------------------------------------------------------
# Provenance
# ---------------------------------------------------------------------------

def build_provenance(source_pdb: str) -> dict[str, str]:
    """Build provenance metadata for the preparation run."""
    try:
        import pdbfixer
        pdbfixer_version = getattr(pdbfixer, "__version__", "unknown")
    except Exception:
        pdbfixer_version = "unknown"

    try:
        import openmm
        openmm_version = getattr(openmm, "__version__", "unknown")
    except Exception:
        openmm_version = "unknown"

    return {
        "source_pdb": source_pdb,
        "pdbfixer_version": pdbfixer_version,
        "openmm_version": openmm_version,
    }


# ---------------------------------------------------------------------------
# Flask endpoints
# ---------------------------------------------------------------------------

@app.route("/health", methods=["GET"])
def health():
    """Health check endpoint."""
    return jsonify({"status": "ok"})


@app.route("/readyz", methods=["GET"])
def readyz():
    """Readiness check endpoint."""
    return jsonify({"status": "ready"})


@app.route("/prepare", methods=["POST"])
def prepare():
    """Prepare a receptor for docking.

    Accepts JSON with:
      - pdb_id (str):            PDB ID to fetch from RCSB, OR
      - pdb_data (str):          Raw PDB text for user uploads.
      - mode (str):              "native-ligand", "custom-box", or "pocket-detection".
      - native_ligand_id (str):  Required for native-ligand mode.
      - padding (float):         Box padding in Angstroms (default 10).
      - pH (float):              Protonation pH (default 7.4).
      - add_hydrogens (bool):    Add hydrogens at pH (default false). Off by default
                                 because docking engines add their own protons.
      - keep_cofactors (list):   Residue names to retain (default ["ZN","MG","CA","FE"]).
      - center (list[float]):    Required for custom-box mode.
      - size (list[float]):      Required for custom-box mode.

    Returns JSON:
      - receptor_pdb (str):      Cleaned PDB text.
      - binding_site (dict):     {"center": [x,y,z], "size": [sx,sy,sz]}.
      - stats (dict):            Cleaning statistics.
      - provenance (dict):       Tool versions and source info.
    """
    data: dict[str, Any] = request.get_json(force=True)

    # --- Resolve input PDB ---
    pdb_id = data.get("pdb_id")
    pdb_data = data.get("pdb_data")

    if not pdb_id and not pdb_data:
        return jsonify({"error": "Either 'pdb_id' or 'pdb_data' is required"}), 400

    source_label = pdb_id or "user-upload"

    try:
        if pdb_data:
            raw_pdb = pdb_data
        else:
            raw_pdb = fetch_pdb(pdb_id)
    except ValueError as exc:
        return jsonify({"error": str(exc)}), 502

    # --- Parameters ---
    mode = data.get("mode", "native-ligand")
    ph = float(data.get("pH", 7.4))
    add_hydrogens = bool(data.get("add_hydrogens", False))
    padding = float(data.get("padding", 10.0))
    keep_list = data.get("keep_cofactors", list(DEFAULT_KEEP_COFACTORS))
    keep_set = frozenset(keep_list)

    # --- Binding-site definition (computed BEFORE cleaning so the ligand is present) ---
    try:
        if mode == "native-ligand":
            ligand_id = data.get("native_ligand_id")
            if not ligand_id:
                return jsonify({
                    "error": "native_ligand_id is required for native-ligand mode",
                }), 400
            binding_site = binding_site_from_native_ligand(
                raw_pdb, ligand_id, padding,
            )

        elif mode == "custom-box":
            center = data.get("center")
            size = data.get("size")
            if not center or not size:
                return jsonify({
                    "error": "center and size are required for custom-box mode",
                }), 400
            if len(center) != 3 or len(size) != 3:
                return jsonify({
                    "error": "center and size must each be arrays of 3 floats",
                }), 400
            # Validate after cleaning so we check against the actual receptor.
            # We defer the call below.
            binding_site = None  # computed after cleaning

        elif mode == "pocket-detection":
            return jsonify({
                "error": (
                    "pocket-detection mode is not handled by this container. "
                    "Use the fpocket and p2rank containers instead."
                ),
            }), 400

        else:
            return jsonify({"error": f"Unknown mode: {mode}"}), 400

    except ValueError as exc:
        return jsonify({"error": str(exc)}), 422

    # --- Receptor cleaning ---
    try:
        receptor_pdb, stats = clean_receptor(
            raw_pdb, ph=ph, keep_cofactors=keep_set, add_hydrogens=add_hydrogens,
        )
    except Exception as exc:
        app.logger.exception("Receptor cleaning failed for %s", source_label)
        return jsonify({
            "error": f"Receptor cleaning failed: {exc}",
        }), 500

    # --- Deferred custom-box validation (against cleaned receptor) ---
    if mode == "custom-box" and binding_site is None:
        try:
            binding_site = binding_site_from_custom_box(
                data["center"], data["size"], receptor_pdb,
            )
        except ValueError as exc:
            return jsonify({"error": str(exc)}), 422

    # --- Provenance ---
    provenance = build_provenance(source_label)

    return jsonify({
        "receptor_pdb": receptor_pdb,
        "binding_site": binding_site,
        "stats": stats,
        "provenance": provenance,
    })


# ---------------------------------------------------------------------------
# Dev-mode entrypoint
# ---------------------------------------------------------------------------
if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8000, debug=True)
