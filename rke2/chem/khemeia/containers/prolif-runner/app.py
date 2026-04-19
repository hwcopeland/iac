"""ProLIF interaction fingerprint analysis sidecar.

Accepts receptor + ligand PDBQT strings, computes protein-ligand
interaction fingerprints via ProLIF, and returns an SVG interaction
network plus a structured interaction list.
"""

import io
import os
import re
import tempfile
from typing import Any

import prolif as plf
from flask import Flask, jsonify, request
from rdkit import Chem

app = Flask(__name__)

# ---------------------------------------------------------------------------
# Dark-theme colour palette
# ---------------------------------------------------------------------------
DARK_THEME_COLORS = {
    "HBDonor": "#58a6ff",
    "HBAcceptor": "#58a6ff",
    "Hydrophobic": "#8b949e",
    "Ionic": "#d29922",
    "Anionic": "#d29922",
    "Cationic": "#d29922",
    "VanDerWaals": "#555555",
    "PiStacking": "#bb33bb",
    "PiCation": "#bb33bb",
    "EdgeToFace": "#bb33bb",
    "FaceToFace": "#bb33bb",
    "MetalAcceptor": "#d29922",
}
DARK_BG = "#0d1117"
DARK_TEXT = "#c9d1d9"


# ---------------------------------------------------------------------------
# PDBQT -> PDB conversion helpers
# ---------------------------------------------------------------------------

def _pdbqt_to_pdb_receptor(pdbqt_text: str) -> str:
    """Convert receptor PDBQT to PDB.

    Keeps only ATOM lines and truncates columns beyond 66 (strips the
    Vina partial-charge and atom-type columns).
    """
    lines = []
    for line in pdbqt_text.splitlines():
        if line.startswith("ATOM"):
            lines.append(line[:66].rstrip())
    lines.append("END")
    return "\n".join(lines) + "\n"


def _pdbqt_to_pdb_ligand(pdbqt_text: str) -> str:
    """Convert ligand PDBQT to PDB (MODEL 1 only).

    Keeps HETATM (and ATOM as fallback) lines; strips Vina columns.
    Only processes through MODEL 1 -- stops at the first ENDMDL.
    If the input has no MODEL records, all HETATM/ATOM lines are used.
    """
    has_models = any(
        l.strip().startswith("MODEL") for l in pdbqt_text.splitlines()
    )
    lines = []
    in_model = not has_models  # if no MODEL records, accept all lines
    for line in pdbqt_text.splitlines():
        stripped = line.strip()
        if stripped.startswith("MODEL"):
            parts = stripped.split()
            if len(parts) >= 2 and parts[1] != "1":
                break
            in_model = True
            continue
        if stripped.startswith("ENDMDL"):
            break
        if in_model and stripped.startswith(("HETATM", "ATOM")):
            lines.append(line[:66].rstrip())
    lines.append("END")
    return "\n".join(lines) + "\n"


# ---------------------------------------------------------------------------
# Dark-theme SVG post-processing
# ---------------------------------------------------------------------------

def _apply_dark_theme(svg: str) -> str:
    """Rewrite SVG colours for a dark background."""
    # Set background
    svg = svg.replace(
        "<svg ",
        f'<svg style="background-color:{DARK_BG}" ',
        1,
    )
    # Lighten all text to the dark-theme foreground
    svg = re.sub(
        r'fill="(?:black|#000000|#000)"',
        f'fill="{DARK_TEXT}"',
        svg,
    )
    svg = re.sub(
        r'stroke="(?:black|#000000|#000)"',
        f'stroke="{DARK_TEXT}"',
        svg,
    )
    return svg


# ---------------------------------------------------------------------------
# Main endpoint
# ---------------------------------------------------------------------------

@app.route("/health", methods=["GET"])
def health():
    return jsonify({"status": "ok"})


@app.route("/readyz", methods=["GET"])
def readyz():
    return jsonify({"status": "ready"})


@app.route("/interaction-map", methods=["POST"])
def interaction_map():
    data = request.get_json(force=True)

    receptor_pdbqt = data.get("receptor_pdbqt")
    ligand_pdbqt = data.get("ligand_pdbqt")
    compound_id = data.get("compound_id", "unknown")
    dark_theme = data.get("dark_theme", False)

    if not receptor_pdbqt or not ligand_pdbqt:
        return jsonify({"error": "receptor_pdbqt and ligand_pdbqt are required"}), 400

    try:
        return _compute_interactions(
            receptor_pdbqt, ligand_pdbqt, compound_id, dark_theme,
        )
    except Exception as exc:  # noqa: BLE001
        app.logger.exception("Interaction map failed for %s", compound_id)
        return jsonify({"error": str(exc)}), 500


def _compute_interactions(
    receptor_pdbqt: str,
    ligand_pdbqt: str,
    compound_id: str,
    dark_theme: bool,
) -> Any:
    """Run ProLIF fingerprint and build the response."""
    receptor_pdb = _pdbqt_to_pdb_receptor(receptor_pdbqt)
    ligand_pdb = _pdbqt_to_pdb_ligand(ligand_pdbqt)

    # Write temporary PDB files -- RDKit's MolFromPDBFile needs filesystem paths
    with tempfile.NamedTemporaryFile(
        suffix=".pdb", mode="w", delete=False,
    ) as rec_f:
        rec_f.write(receptor_pdb)
        rec_path = rec_f.name

    with tempfile.NamedTemporaryFile(
        suffix=".pdb", mode="w", delete=False,
    ) as lig_f:
        lig_f.write(ligand_pdb)
        lig_path = lig_f.name

    try:
        rec_mol = Chem.MolFromPDBFile(rec_path, removeHs=False, sanitize=False)
        lig_mol = Chem.MolFromPDBFile(lig_path, removeHs=False, sanitize=False)

        if rec_mol is None:
            return jsonify({"error": "Failed to parse receptor PDB"}), 422
        if lig_mol is None:
            return jsonify({"error": "Failed to parse ligand PDB"}), 422

        # Convert to ProLIF Molecule objects
        protein = plf.Molecule(rec_mol)
        ligand = plf.Molecule(lig_mol)

        # Run fingerprint analysis
        fp = plf.Fingerprint()
        fp.run_from_iterable([ligand], protein)

        # Extract interaction data
        interaction_list = _extract_interactions(fp)

        # Generate network plot as SVG
        svg = _generate_network_svg(fp, compound_id, dark_theme)

        return jsonify({
            "svg": svg,
            "interactions": interaction_list,
            "compound_id": compound_id,
        })
    finally:
        os.unlink(rec_path)
        os.unlink(lig_path)


def _extract_interactions(fp: plf.Fingerprint) -> list[dict]:
    """Pull per-residue interaction details from the fingerprint result."""
    interactions = []
    try:
        df = fp.to_dataframe()
        if df.empty:
            return interactions

        # DataFrame columns are multi-indexed: (ligand_residue, protein_residue, interaction_type)
        for col in df.columns:
            if df[col].iloc[0]:
                parts = col
                if len(parts) >= 3:
                    residue = str(parts[1])
                    interaction_type = str(parts[2])
                    interactions.append({
                        "residue": residue,
                        "type": interaction_type,
                    })
    except Exception:  # noqa: BLE001
        app.logger.warning("Could not extract interaction details from dataframe")

    # Ensure every interaction has a distance key (ProLIF bitvectors do not
    # expose per-interaction distances, so we set None as a placeholder).
    for interaction in interactions:
        interaction.setdefault("distance", None)

    return interactions


def _generate_network_svg(
    fp: plf.Fingerprint,
    compound_id: str,
    dark_theme: bool,
) -> str:
    """Render the ligand interaction diagram as SVG using matplotlib."""
    import matplotlib
    matplotlib.use("Agg")
    import matplotlib.pyplot as plt
    import numpy as np

    interactions = _extract_interactions(fp)
    if not interactions:
        return _empty_svg(compound_id, dark_theme)

    # Group by residue
    residue_interactions: dict[str, list[str]] = {}
    for ix in interactions:
        res = ix["residue"]
        if res not in residue_interactions:
            residue_interactions[res] = []
        residue_interactions[res].append(ix["type"])

    residues = list(residue_interactions.keys())
    n = len(residues)

    # Color palette
    colors = {
        "HBDonor": "#58a6ff", "HBAcceptor": "#58a6ff",
        "Hydrophobic": "#8b949e", "VdWContact": "#555555",
        "VanDerWaals": "#555555",
        "Ionic": "#d29922", "Anionic": "#d29922", "Cationic": "#d29922",
        "PiStacking": "#bb33bb", "PiCation": "#bb33bb",
        "EdgeToFace": "#bb33bb", "FaceToFace": "#bb33bb",
    }

    bg = DARK_BG if dark_theme else "white"
    text_color = DARK_TEXT if dark_theme else "black"

    fig, ax = plt.subplots(figsize=(8, 8), facecolor=bg)
    ax.set_facecolor(bg)
    ax.set_xlim(-2, 2)
    ax.set_ylim(-2, 2)
    ax.set_aspect("equal")
    ax.axis("off")

    # Ligand in center
    ax.text(0, 0, compound_id, ha="center", va="center",
            fontsize=11, fontweight="bold", color="#58a6ff",
            bbox=dict(boxstyle="round,pad=0.4", facecolor=bg,
                      edgecolor="#58a6ff", linewidth=1.5))

    # Residues in circle
    for i, res in enumerate(residues):
        angle = 2 * np.pi * i / max(n, 1) - np.pi / 2
        x = 1.5 * np.cos(angle)
        y = 1.5 * np.sin(angle)

        ix_types = residue_interactions[res]
        primary = ix_types[0]
        color = colors.get(primary, "#555555")

        # Dashed line from center to residue
        ax.plot([0, x], [0, y], color=color, linewidth=1.5,
                linestyle="--", alpha=0.6, zorder=1)

        # Residue circle + label
        circle = plt.Circle((x, y), 0.22, facecolor=bg,
                             edgecolor=color, linewidth=2, zorder=2)
        ax.add_patch(circle)
        ax.text(x, y + 0.04, res, ha="center", va="center",
                fontsize=7, color=text_color, fontweight="bold", zorder=3)

    # Legend
    seen = set()
    legend_y = 1.8
    for ix in interactions:
        t = ix["type"]
        if t in seen:
            continue
        seen.add(t)
        c = colors.get(t, "#555")
        ax.plot([- 1.9, -1.7], [legend_y, legend_y], color=c,
                linewidth=2, linestyle="--")
        ax.text(-1.65, legend_y, t, fontsize=8, color=c, va="center")
        legend_y -= 0.15

    ax.set_title(f"Interaction Map: {compound_id}", fontsize=12,
                 color=text_color, pad=10)

    buf = io.BytesIO()
    fig.savefig(buf, format="svg", bbox_inches="tight",
                facecolor=bg, edgecolor="none")
    plt.close(fig)
    return buf.getvalue().decode("utf-8")


def _empty_svg(compound_id: str, dark_theme: bool) -> str:
    bg = DARK_BG if dark_theme else "white"
    text = DARK_TEXT if dark_theme else "black"
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" width="400" height="100"'
        f' style="background-color:{bg}">'
        f'<text x="200" y="50" text-anchor="middle" fill="{text}" '
        f'font-size="14">No interactions detected for {compound_id}</text></svg>'
    )


# ---------------------------------------------------------------------------
# Dev-mode entrypoint
# ---------------------------------------------------------------------------
if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8000, debug=True)
