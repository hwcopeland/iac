"""Library preparation sidecar for the Khemeia SBDD pipeline.

Compound standardization, filtering, and 3D conformer generation.
Accepts SMILES lists or SDF data, standardizes molecules via RDKit
MolStandardize and (optionally) ChEMBL Structure Pipeline, computes
drug-likeness descriptors, applies configurable filters (Lipinski,
Veber, PAINS, Brenk, REOS), generates 3D conformers via ETKDG, and
converts to PDBQT via Meeko when available.
"""

import io
import logging
import traceback
from typing import Any

from flask import Flask, jsonify, request
from rdkit import Chem, RDLogger
from rdkit.Chem import AllChem, Descriptors, FilterCatalog, QED, rdMolDescriptors
from rdkit.Chem.MolStandardize import rdMolStandardize

# Suppress RDKit warnings that would flood gunicorn logs during batch
# processing.  Actual molecule-level errors are caught and logged.
RDLogger.logger().setLevel(RDLogger.ERROR)

app = Flask(__name__)
logging.basicConfig(level=logging.INFO)

# ---------------------------------------------------------------------------
# Optional: ChEMBL Structure Pipeline
# ---------------------------------------------------------------------------

_HAS_CHEMBL_STANDARDIZER = False
try:
    from chembl_structure_pipeline import standardize_mol as _chembl_standardize_mol
    _HAS_CHEMBL_STANDARDIZER = True
except ImportError:
    app.logger.info(
        "chembl_structure_pipeline not available; "
        "skipping ChEMBL standardization step"
    )

# ---------------------------------------------------------------------------
# Optional: Meeko for PDBQT output
# ---------------------------------------------------------------------------

_HAS_MEEKO = False
try:
    from meeko import MoleculePreparation  # type: ignore[import-untyped]
    _HAS_MEEKO = True
except ImportError:
    app.logger.info("meeko not available; PDBQT output disabled")

# ---------------------------------------------------------------------------
# Filter catalogs (built once at import time)
# ---------------------------------------------------------------------------

_pains_params = FilterCatalog.FilterCatalogParams()
_pains_params.AddCatalog(FilterCatalog.FilterCatalogParams.FilterCatalogs.PAINS)
_PAINS_CATALOG = FilterCatalog.FilterCatalog(_pains_params)

_brenk_params = FilterCatalog.FilterCatalogParams()
_brenk_params.AddCatalog(FilterCatalog.FilterCatalogParams.FilterCatalogs.BRENK)
_BRENK_CATALOG = FilterCatalog.FilterCatalog(_brenk_params)


# ---------------------------------------------------------------------------
# Standardization
# ---------------------------------------------------------------------------

def standardize_molecule(mol: Chem.Mol) -> Chem.Mol | None:
    """Sanitize, normalize, uncharge, and select canonical tautomer.

    Returns the standardized mol, or None on failure.
    """
    try:
        Chem.SanitizeMol(mol)
        mol = rdMolStandardize.Normalize(mol)
        uncharger = rdMolStandardize.Uncharger()
        mol = uncharger.uncharge(mol)

        # Canonical tautomer
        enumerator = rdMolStandardize.TautomerEnumerator()
        mol = enumerator.Canonicalize(mol)

        # Optional ChEMBL pipeline (extra salt stripping, standardization)
        if _HAS_CHEMBL_STANDARDIZER:
            try:
                mol = _chembl_standardize_mol(mol)
            except Exception:
                # ChEMBL pipeline can fail on unusual molecules; fall back
                # to the RDKit-only result rather than rejecting the molecule.
                app.logger.debug(
                    "ChEMBL standardizer failed, using RDKit-only result"
                )

        return mol
    except Exception:
        return None


# ---------------------------------------------------------------------------
# Descriptor computation
# ---------------------------------------------------------------------------

def compute_descriptors(mol: Chem.Mol) -> dict[str, Any]:
    """Compute drug-likeness descriptors for a standardized molecule."""
    return {
        "mw": round(Descriptors.MolWt(mol), 2),
        "logp": round(Descriptors.MolLogP(mol), 2),
        "hba": rdMolDescriptors.CalcNumHBA(mol),
        "hbd": rdMolDescriptors.CalcNumHBD(mol),
        "psa": round(Descriptors.TPSA(mol), 2),
        "rotatable_bonds": rdMolDescriptors.CalcNumRotatableBonds(mol),
        "heavy_atom_count": mol.GetNumHeavyAtoms(),
        "formal_charge": Chem.GetFormalCharge(mol),
        "qed": round(QED.qed(mol), 4),
    }


# ---------------------------------------------------------------------------
# Filters
# ---------------------------------------------------------------------------

def _lipinski(desc: dict) -> bool:
    """Lipinski Rule of Five -- pass if <= 1 violation."""
    violations = 0
    if desc["mw"] > 500:
        violations += 1
    if desc["logp"] > 5:
        violations += 1
    if desc["hba"] > 10:
        violations += 1
    if desc["hbd"] > 5:
        violations += 1
    return violations <= 1


def _veber(desc: dict) -> bool:
    """Veber oral bioavailability: PSA <= 140, rotatable bonds <= 10."""
    return desc["psa"] <= 140 and desc["rotatable_bonds"] <= 10


def _pains(mol: Chem.Mol) -> bool:
    """PAINS substructure filter -- True means PASS (no PAINS match)."""
    return not _PAINS_CATALOG.HasMatch(mol)


def _brenk(mol: Chem.Mol) -> bool:
    """Brenk unwanted substructure filter -- True means PASS."""
    return not _BRENK_CATALOG.HasMatch(mol)


def _reos(desc: dict) -> bool:
    """Rapid Elimination Of Swill (REOS)."""
    return (
        200 <= desc["mw"] <= 500
        and -5 <= desc["logp"] <= 5
        and 0 <= desc["hbd"] <= 5
        and 0 <= desc["hba"] <= 10
        and -2 <= desc["formal_charge"] <= 2
        and 0 <= desc["rotatable_bonds"] <= 8
        and 15 <= desc["heavy_atom_count"] <= 50
    )


# Map of filter name -> (function, requires_mol).
# Descriptor-only filters receive desc dict; mol-based filters receive mol.
FILTER_REGISTRY: dict[str, tuple[Any, bool]] = {
    "lipinski": (_lipinski, False),
    "veber": (_veber, False),
    "pains": (_pains, True),
    "brenk": (_brenk, True),
    "reos": (_reos, False),
}


def apply_filters(
    mol: Chem.Mol,
    desc: dict,
    enabled_filters: dict[str, bool],
) -> dict[str, bool]:
    """Run enabled filters and return per-filter pass/fail results.

    Only filters whose key is present and True in *enabled_filters* are run.
    Returns a dict mapping filter name -> pass (True) or fail (False).
    """
    results: dict[str, bool] = {}
    for name, enabled in enabled_filters.items():
        if not enabled:
            continue
        entry = FILTER_REGISTRY.get(name)
        if entry is None:
            continue
        fn, needs_mol = entry
        results[name] = fn(mol) if needs_mol else fn(desc)
    return results


# ---------------------------------------------------------------------------
# 3D Conformer generation
# ---------------------------------------------------------------------------

def generate_conformer(mol: Chem.Mol) -> Chem.Mol | None:
    """Generate a single low-energy 3D conformer via ETKDG + MMFF94.

    Returns mol with 3D coordinates, or None on embedding failure.
    """
    mol = Chem.AddHs(mol)

    params = AllChem.ETKDGv3()
    params.randomSeed = 42

    result = AllChem.EmbedMolecule(mol, params)
    if result == -1:
        # Fallback: relax chirality + use random initial coords
        params.useRandomCoords = True
        params.enforceChirality = False
        result = AllChem.EmbedMolecule(mol, params)
        if result == -1:
            return None

    # MMFF94 energy minimization (max 200 iterations per spec)
    try:
        AllChem.MMFFOptimizeMolecule(mol, maxIters=200)
    except Exception:
        # Optimization failure is non-fatal; the ETKDG coords are usable
        app.logger.debug("MMFF optimization failed, using ETKDG coords")

    return mol


# ---------------------------------------------------------------------------
# PDBQT conversion via Meeko
# ---------------------------------------------------------------------------

def mol_to_pdbqt(mol: Chem.Mol) -> str | None:
    """Convert an RDKit mol with 3D coords to PDBQT via Meeko.

    Returns PDBQT string, or None if Meeko is unavailable or conversion fails.
    Handles both the newer Meeko API (prepare returns list of MoleculeSetup)
    and the older API (prepare populates setup attribute).
    """
    if not _HAS_MEEKO:
        return None
    try:
        from meeko import PDBQTWriterLegacy
        preparator = MoleculePreparation()
        mol_setup_list = preparator.prepare(mol)
        if not mol_setup_list:
            return None
        # Meeko >= 0.5: PDBQTWriterLegacy.write_string takes the MoleculeSetup object
        pdbqt_string, is_ok, _ = PDBQTWriterLegacy.write_string(mol_setup_list[0])
        return pdbqt_string if is_ok and pdbqt_string else None
    except Exception:
        return None


# ---------------------------------------------------------------------------
# SDF output helper
# ---------------------------------------------------------------------------

def mol_to_sdf(mol: Chem.Mol) -> str:
    """Write a single mol to SDF text."""
    writer = Chem.SDWriter(sio := io.StringIO())
    writer.write(mol)
    writer.close()
    return sio.getvalue()


# ---------------------------------------------------------------------------
# Core processing pipeline
# ---------------------------------------------------------------------------

def process_molecule(
    mol: Chem.Mol,
    source_smiles: str,
    enabled_filters: dict[str, bool],
    generate_3d: bool = True,
) -> dict[str, Any]:
    """Run the full standardization + filter + conformer pipeline on one mol.

    Returns an annotation dict with all computed data.
    """
    record: dict[str, Any] = {
        "input_smiles": source_smiles,
        "canonical_smiles": None,
        "inchikey": None,
        "stable_id": None,
        "descriptors": None,
        "filters": None,
        "filtered": False,
        "conformer_failed": False,
        "sdf_data": None,
        "pdbqt_data": None,
        "error": None,
    }

    # 1. Standardize
    std_mol = standardize_molecule(mol)
    if std_mol is None:
        record["error"] = "Standardization failed"
        return record

    # 2. Canonical SMILES + InChIKey
    canonical = Chem.MolToSmiles(std_mol)
    inchikey = None
    try:
        from rdkit.Chem.inchi import MolToInchi, InchiToInchiKey
        inchi = MolToInchi(std_mol)
        if inchi:
            inchikey = InchiToInchiKey(inchi)
    except Exception:
        pass

    record["canonical_smiles"] = canonical
    record["inchikey"] = inchikey
    if inchikey:
        record["stable_id"] = f"KHM-{inchikey[:14]}"

    # 3. Descriptors
    desc = compute_descriptors(std_mol)
    record["descriptors"] = desc

    # 4. Filters
    filter_results = apply_filters(std_mol, desc, enabled_filters)
    record["filters"] = filter_results
    # A compound is marked filtered if ANY enabled filter fails
    if filter_results and not all(filter_results.values()):
        record["filtered"] = True

    # 5. 3D conformer
    if generate_3d:
        conf_mol = generate_conformer(std_mol)
        if conf_mol is None:
            record["conformer_failed"] = True
        else:
            record["sdf_data"] = mol_to_sdf(conf_mol)
            pdbqt = mol_to_pdbqt(conf_mol)
            if pdbqt:
                record["pdbqt_data"] = pdbqt

    return record


# ---------------------------------------------------------------------------
# Flask endpoints
# ---------------------------------------------------------------------------

@app.route("/health", methods=["GET"])
def health():
    """Health check endpoint."""
    return jsonify({
        "status": "ok",
        "chembl_standardizer": _HAS_CHEMBL_STANDARDIZER,
        "meeko": _HAS_MEEKO,
    })


@app.route("/readyz", methods=["GET"])
def readyz():
    """Readiness check endpoint."""
    return jsonify({"status": "ready"})


@app.route("/standardize", methods=["POST"])
def standardize():
    """Standardize, annotate, filter, and generate 3D conformers.

    Accepts JSON with:
      - smiles_list (list[str]): SMILES strings to process, OR
      - sdf_data (str): raw SDF text containing one or more molecules.
      - filters (dict, optional): per-filter enable flags.
        Default: {"lipinski": true, "veber": true, "pains": true,
                  "brenk": false, "reos": false}
      - generate_3d (bool, optional): generate conformers (default true).

    Returns JSON:
      - compounds (list[dict]): annotated compound records.
      - summary (dict): counts of processed, passed, filtered, failed.
    """
    data: dict[str, Any] = request.get_json(force=True)

    smiles_list = data.get("smiles_list")
    sdf_data = data.get("sdf_data")

    if not smiles_list and not sdf_data:
        return jsonify({
            "error": "Either 'smiles_list' or 'sdf_data' is required",
        }), 400

    # Default filter configuration per WP-2 spec
    enabled_filters = data.get("filters", {
        "lipinski": True,
        "veber": True,
        "pains": True,
        "brenk": False,
        "reos": False,
    })
    generate_3d = data.get("generate_3d", True)

    # Collect (mol, source_smiles) pairs
    mol_inputs: list[tuple[Chem.Mol, str]] = []

    if smiles_list:
        for smi in smiles_list:
            mol = Chem.MolFromSmiles(smi)
            if mol is not None:
                mol_inputs.append((mol, smi))
            else:
                # Record parse failures directly
                mol_inputs.append((None, smi))  # type: ignore[arg-type]

    if sdf_data:
        supplier = Chem.SDMolSupplier()
        supplier.SetData(sdf_data)
        for mol in supplier:
            if mol is not None:
                smi = Chem.MolToSmiles(mol)
                mol_inputs.append((mol, smi))
            else:
                mol_inputs.append((None, ""))  # type: ignore[arg-type]

    # Process each molecule
    compounds: list[dict[str, Any]] = []
    n_processed = 0
    n_passed = 0
    n_filtered = 0
    n_failed = 0

    for mol, source_smiles in mol_inputs:
        n_processed += 1
        if mol is None:
            compounds.append({
                "input_smiles": source_smiles,
                "canonical_smiles": None,
                "inchikey": None,
                "stable_id": None,
                "descriptors": None,
                "filters": None,
                "filtered": False,
                "conformer_failed": False,
                "sdf_data": None,
                "pdbqt_data": None,
                "error": "Failed to parse SMILES",
            })
            n_failed += 1
            continue

        try:
            record = process_molecule(
                mol, source_smiles, enabled_filters, generate_3d,
            )
        except Exception as exc:
            app.logger.warning(
                "Unexpected error processing %s: %s",
                source_smiles, traceback.format_exc(),
            )
            record = {
                "input_smiles": source_smiles,
                "canonical_smiles": None,
                "inchikey": None,
                "stable_id": None,
                "descriptors": None,
                "filters": None,
                "filtered": False,
                "conformer_failed": False,
                "sdf_data": None,
                "pdbqt_data": None,
                "error": str(exc),
            }
            n_failed += 1
            compounds.append(record)
            continue

        if record.get("error"):
            n_failed += 1
        elif record.get("filtered"):
            n_filtered += 1
        else:
            n_passed += 1

        compounds.append(record)

    return jsonify({
        "compounds": compounds,
        "summary": {
            "total": n_processed,
            "passed": n_passed,
            "filtered": n_filtered,
            "failed": n_failed,
            "conformer_failed": sum(
                1 for c in compounds if c.get("conformer_failed")
            ),
        },
    })


@app.route("/filter", methods=["POST"])
def filter_compounds():
    """Apply filters to pre-computed compound records.

    Accepts JSON with:
      - compounds (list[dict]): compound records with at least
        'canonical_smiles' and 'descriptors' populated.
      - filters (dict): per-filter enable flags.

    Returns JSON:
      - compounds (list[dict]): re-annotated compound records.
      - summary (dict): counts.
    """
    data: dict[str, Any] = request.get_json(force=True)

    compounds_in = data.get("compounds")
    if not compounds_in:
        return jsonify({"error": "'compounds' is required"}), 400

    enabled_filters = data.get("filters", {
        "lipinski": True,
        "veber": True,
        "pains": True,
        "brenk": False,
        "reos": False,
    })

    results: list[dict[str, Any]] = []
    n_passed = 0
    n_filtered = 0
    n_failed = 0

    for compound in compounds_in:
        smiles = compound.get("canonical_smiles")
        if not smiles:
            compound["error"] = "Missing canonical_smiles"
            n_failed += 1
            results.append(compound)
            continue

        mol = Chem.MolFromSmiles(smiles)
        if mol is None:
            compound["error"] = "Failed to parse canonical_smiles"
            n_failed += 1
            results.append(compound)
            continue

        # Use existing descriptors if present, otherwise recompute
        desc = compound.get("descriptors")
        if not desc:
            desc = compute_descriptors(mol)
            compound["descriptors"] = desc

        filter_results = apply_filters(mol, desc, enabled_filters)
        compound["filters"] = filter_results
        compound["filtered"] = bool(
            filter_results and not all(filter_results.values())
        )

        if compound["filtered"]:
            n_filtered += 1
        else:
            n_passed += 1

        results.append(compound)

    return jsonify({
        "compounds": results,
        "summary": {
            "total": len(results),
            "passed": n_passed,
            "filtered": n_filtered,
            "failed": n_failed,
        },
    })


# ---------------------------------------------------------------------------
# Dev-mode entrypoint
# ---------------------------------------------------------------------------
if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8000, debug=True)
