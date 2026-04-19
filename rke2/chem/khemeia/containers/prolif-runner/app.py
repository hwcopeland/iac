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

        # Generate network plot as HTML (proper ProLIF ligand network)
        html = _generate_network_html(fp, lig_mol, compound_id, dark_theme)

        return jsonify({
            "html": html,
            "svg": "",  # deprecated, use html
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


def _generate_network_html(
    fp: plf.Fingerprint,
    ligand_mol,
    compound_id: str,
    dark_theme: bool,
) -> str:
    """Render the ProLIF ligand interaction network as HTML.

    Uses fp.plot_lignetwork which generates the proper 2D ligand structure
    with atom-level interaction lines to protein residues.
    """
    try:
        net = fp.plot_lignetwork(
            ligand_mol,
            kind="frame",
            frame=0,
            display_all=False,
            use_coordinates=True,
        )

        buf = io.StringIO()
        net.save(buf)
        html = buf.getvalue()

        if dark_theme:
            # Inject dark theme CSS + residue click handler + zoom fix
            dark_inject = """
<style>
  body { background-color: #0d1117 !important; margin: 0; }
  .vis-network { background-color: #0d1117 !important; }
  canvas { background-color: #0d1117 !important; }
</style>
<script>
  document.addEventListener('DOMContentLoaded', function() {
    setTimeout(function() {
      if (typeof network !== 'undefined') {
        // Reduce zoom speed significantly
        network.setOptions({
          interaction: {
            zoomSpeed: 0.15,
            zoomView: true
          }
        });

        // Click residue → post to parent for 3D viewer focus
        network.on('click', function(params) {
          if (params.nodes.length > 0) {
            var nodeId = params.nodes[0];
            var node = network.body.data.nodes.get(nodeId);
            if (node) {
              var label = node.label || node.title || String(nodeId);
              // Only post for protein residue nodes (have format like ASP107.A)
              if (label.match(/[A-Z]{3}\\d+/)) {
                window.parent.postMessage({
                  type: 'prolif-residue-click',
                  residue: label
                }, '*');
              }
            }
          }
        });
      }
    }, 3000);
    // Also try immediately in case it's already loaded
    try {
      if (typeof network !== 'undefined') {
        network.setOptions({ interaction: { zoomSpeed: 0.15, zoomView: true } });
      }
    } catch(e) {}
  });
</script>
"""
            html = html.replace("</head>", dark_inject + "</head>", 1)

            # Fix all colors for dark theme — SVG attributes AND JSON strings
            # White fills → dark bg
            html = html.replace('fill="white"', 'fill="#0d1117"')
            html = html.replace("fill='white'", "fill='#0d1117'")
            html = html.replace('fill="#FFFFFF"', 'fill="#0d1117"')
            # Black → light grey (SVG attributes)
            html = html.replace('fill="black"', 'fill="#c9d1d9"')
            html = html.replace('stroke="black"', 'stroke="#c9d1d9"')
            # Black → light grey (JSON strings in vis-network config)
            html = html.replace('"color": "black"', '"color": "#c9d1d9"')
            html = html.replace('"fontcolor": "black"', '"fontcolor": "#c9d1d9"')
            html = html.replace('"color": "#000000"', '"color": "#c9d1d9"')
            html = html.replace('"color":"black"', '"color":"#c9d1d9"')
            # White → dark bg (JSON strings)
            html = html.replace('"color": "white"', '"color": "#0d1117"')
            html = html.replace('"color":"white"', '"color":"#0d1117"')

        return html
    except Exception as exc:
        app.logger.warning(
            "plot_lignetwork failed for %s: %s", compound_id, exc,
        )
        return ""


# ---------------------------------------------------------------------------
# Dev-mode entrypoint
# ---------------------------------------------------------------------------
if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8000, debug=True)
