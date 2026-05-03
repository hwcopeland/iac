"""ADMET prediction sidecar for the Khemeia SBDD pipeline.

Serves ADMET property predictions using the Stanford admet_ai package
(Chemprop-D models). Falls back to RDKit descriptor-based rules when
the ML models are unavailable.

Endpoints:
    POST /predict   - batch ADMET prediction for a list of SMILES
    POST /mpo       - multi-parameter optimization scoring
    GET  /health    - liveness probe
    GET  /readyz    - readiness probe (True once models are loaded)
"""

import logging
import math
import time
import traceback
from typing import Any

from flask import Flask, jsonify, request
from rdkit import Chem, RDLogger
from rdkit.Chem import Descriptors, rdMolDescriptors

# Suppress noisy RDKit warnings during batch processing.
RDLogger.logger().setLevel(RDLogger.ERROR)

app = Flask(__name__)
logging.basicConfig(level=logging.INFO)

# ---------------------------------------------------------------------------
# Model loading — attempt admet_ai, fall back to rule-based
# ---------------------------------------------------------------------------

_ADMET_AI_AVAILABLE = False
_admet_predictor = None
_model_load_time = None

try:
    from admet_ai import ADMETModel
    _admet_predictor = ADMETModel()
    _ADMET_AI_AVAILABLE = True
    _model_load_time = time.time()
    app.logger.info("admet_ai models loaded successfully")
except Exception as exc:
    app.logger.warning(
        "admet_ai not available (%s); using rule-based fallback", exc
    )

# ---------------------------------------------------------------------------
# Endpoint name mapping: canonical name -> admet_ai column name
# admet_ai returns predictions keyed by dataset/task names.  We map them
# to our canonical endpoint names from the WP-4 spec.
# ---------------------------------------------------------------------------

# These mappings will be populated dynamically from admet_ai output columns.
# We define the canonical set of endpoints we want to expose.
CANONICAL_ENDPOINTS = {
    # Absorption
    "solubility": {
        "category": "absorption",
        "unit": "log S",
        "continuous": True,
        "description": "Aqueous solubility",
    },
    "caco2": {
        "category": "absorption",
        "unit": "log Papp (cm/s)",
        "continuous": True,
        "description": "Caco-2 permeability",
    },
    "hia": {
        "category": "absorption",
        "unit": "probability",
        "continuous": False,
        "description": "Human intestinal absorption",
    },
    "pgp_substrate": {
        "category": "absorption",
        "unit": "probability",
        "continuous": False,
        "description": "P-glycoprotein substrate",
    },
    "pgp_inhibitor": {
        "category": "absorption",
        "unit": "probability",
        "continuous": False,
        "description": "P-glycoprotein inhibitor",
    },
    "bioavailability": {
        "category": "absorption",
        "unit": "probability",
        "continuous": False,
        "description": "Oral bioavailability (%F >= 20%)",
    },
    # Distribution
    "ppb": {
        "category": "distribution",
        "unit": "% bound",
        "continuous": True,
        "description": "Plasma protein binding",
    },
    "vdss": {
        "category": "distribution",
        "unit": "L/kg",
        "continuous": True,
        "description": "Volume of distribution at steady state",
    },
    "bbb": {
        "category": "distribution",
        "unit": "probability",
        "continuous": False,
        "description": "Blood-brain barrier penetration",
    },
    # Metabolism — CYP substrates
    "cyp1a2_substrate": {
        "category": "metabolism",
        "unit": "probability",
        "continuous": False,
        "description": "CYP1A2 substrate",
    },
    "cyp1a2_inhibitor": {
        "category": "metabolism",
        "unit": "probability",
        "continuous": False,
        "description": "CYP1A2 inhibitor",
    },
    "cyp2c9_substrate": {
        "category": "metabolism",
        "unit": "probability",
        "continuous": False,
        "description": "CYP2C9 substrate",
    },
    "cyp2c9_inhibitor": {
        "category": "metabolism",
        "unit": "probability",
        "continuous": False,
        "description": "CYP2C9 inhibitor",
    },
    "cyp2c19_substrate": {
        "category": "metabolism",
        "unit": "probability",
        "continuous": False,
        "description": "CYP2C19 substrate",
    },
    "cyp2c19_inhibitor": {
        "category": "metabolism",
        "unit": "probability",
        "continuous": False,
        "description": "CYP2C19 inhibitor",
    },
    "cyp2d6_substrate": {
        "category": "metabolism",
        "unit": "probability",
        "continuous": False,
        "description": "CYP2D6 substrate",
    },
    "cyp2d6_inhibitor": {
        "category": "metabolism",
        "unit": "probability",
        "continuous": False,
        "description": "CYP2D6 inhibitor",
    },
    "cyp3a4_substrate": {
        "category": "metabolism",
        "unit": "probability",
        "continuous": False,
        "description": "CYP3A4 substrate",
    },
    "cyp3a4_inhibitor": {
        "category": "metabolism",
        "unit": "probability",
        "continuous": False,
        "description": "CYP3A4 inhibitor",
    },
    # Excretion
    "clearance": {
        "category": "excretion",
        "unit": "mL/min/kg",
        "continuous": True,
        "description": "Total body clearance",
    },
    "half_life": {
        "category": "excretion",
        "unit": "hours",
        "continuous": True,
        "description": "Elimination half-life",
    },
    # Toxicity
    "herg": {
        "category": "toxicity",
        "unit": "probability",
        "continuous": False,
        "description": "hERG inhibition",
    },
    "ames": {
        "category": "toxicity",
        "unit": "probability",
        "continuous": False,
        "description": "AMES mutagenicity",
    },
    "dili": {
        "category": "toxicity",
        "unit": "probability",
        "continuous": False,
        "description": "Drug-induced liver injury",
    },
    "hepatotoxicity": {
        "category": "toxicity",
        "unit": "probability",
        "continuous": False,
        "description": "Hepatotoxicity",
    },
    "skin_sensitization": {
        "category": "toxicity",
        "unit": "probability",
        "continuous": False,
        "description": "Skin sensitization",
    },
    "ld50": {
        "category": "toxicity",
        "unit": "mg/kg (log)",
        "continuous": True,
        "description": "Acute oral toxicity LD50",
    },
    "carcinogenicity": {
        "category": "toxicity",
        "unit": "probability",
        "continuous": False,
        "description": "Carcinogenicity",
    },
}

# Fuzzy matching: admet_ai column names vary by version.  We try common
# patterns to map them to our canonical names.
_COLUMN_ALIASES = {
    "solubility": ["Solubility", "AqSol", "ESOL", "logS", "solubility"],
    "caco2": ["Caco2", "Caco-2", "Caco2_Wang", "caco2"],
    "hia": ["HIA", "HIA_Hou", "hia"],
    "pgp_substrate": ["Pgp_sub", "Pgp-sub", "Pgp_substrate"],
    "pgp_inhibitor": ["Pgp_inh", "Pgp-inh", "Pgp_inhibitor"],
    "bioavailability": ["Bioavailability_Ma", "Bioavailability", "bioavailability", "F20"],
    "ppb": ["PPB", "PPBR", "ppb", "PPB_opt"],
    "vdss": ["VDss", "VD", "VDss_Lombardo", "vdss"],
    "bbb": ["BBB", "BBB_Martins", "bbb"],
    "cyp1a2_substrate": ["CYP1A2_sub", "CYP1A2-sub", "CYP1A2_Substrate"],
    "cyp1a2_inhibitor": ["CYP1A2_inh", "CYP1A2-inh", "CYP1A2_Veith"],
    "cyp2c9_substrate": ["CYP2C9_sub", "CYP2C9-sub", "CYP2C9_Substrate"],
    "cyp2c9_inhibitor": ["CYP2C9_inh", "CYP2C9-inh", "CYP2C9_Veith"],
    "cyp2c19_substrate": ["CYP2C19_sub", "CYP2C19-sub", "CYP2C19_Substrate"],
    "cyp2c19_inhibitor": ["CYP2C19_inh", "CYP2C19-inh", "CYP2C19_Veith"],
    "cyp2d6_substrate": ["CYP2D6_sub", "CYP2D6-sub", "CYP2D6_Substrate"],
    "cyp2d6_inhibitor": ["CYP2D6_inh", "CYP2D6-inh", "CYP2D6_Veith"],
    "cyp3a4_substrate": ["CYP3A4_sub", "CYP3A4-sub", "CYP3A4_Substrate"],
    "cyp3a4_inhibitor": ["CYP3A4_inh", "CYP3A4-inh", "CYP3A4_Veith"],
    "clearance": ["CL_Hepa", "CL-Hepa", "Clearance_Hepatocyte", "clearance", "CL-Micro"],
    "half_life": ["Half_Life", "Half-Life", "Half_Life_Obach", "half_life", "T12"],
    "herg": ["hERG", "hERG_inh", "hERG_Karim", "herg"],
    "ames": ["AMES", "Ames", "ames"],
    "dili": ["DILI", "dili"],
    "hepatotoxicity": ["Hepatotoxicity", "hepatotoxicity"],
    "skin_sensitization": ["Skin", "Skin_Reaction", "skin_sensitization"],
    "ld50": ["LD50", "LD50_Zhu", "ld50"],
    "carcinogenicity": ["Carcinogenicity", "Carcinogens_Lagunin", "carcinogenicity"],
}


def _build_column_map(df_columns: list[str]) -> dict[str, str]:
    """Build a mapping from admet_ai DataFrame column -> canonical endpoint name."""
    col_map = {}
    col_lower = {c.lower(): c for c in df_columns}

    for canonical, aliases in _COLUMN_ALIASES.items():
        for alias in aliases:
            if alias in df_columns:
                col_map[alias] = canonical
                break
            if alias.lower() in col_lower:
                col_map[col_lower[alias.lower()]] = canonical
                break
    return col_map


# ---------------------------------------------------------------------------
# Rule-based fallback predictions (from RDKit descriptors)
# ---------------------------------------------------------------------------

def _rdkit_descriptors(mol: Chem.Mol) -> dict[str, Any]:
    """Compute molecular descriptors using RDKit."""
    mw = Descriptors.MolWt(mol)
    logp = Descriptors.MolLogP(mol)
    hba = rdMolDescriptors.CalcNumHBA(mol)
    hbd = rdMolDescriptors.CalcNumHBD(mol)
    psa = Descriptors.TPSA(mol)
    rotatable = rdMolDescriptors.CalcNumRotatableBonds(mol)
    return {
        "mw": mw, "logp": logp, "hba": hba, "hbd": hbd,
        "psa": psa, "rotatable_bonds": rotatable,
    }


def _rule_based_predict(smiles: str) -> dict[str, Any] | None:
    """Compute rule-based ADMET estimates from molecular descriptors.

    This is the fallback when admet_ai is not available.  It provides
    heuristic estimates for a subset of endpoints.
    """
    mol = Chem.MolFromSmiles(smiles)
    if mol is None:
        return None

    d = _rdkit_descriptors(mol)
    mw, logp, hba, hbd, psa = d["mw"], d["logp"], d["hba"], d["hbd"], d["psa"]

    endpoints = {}

    # Solubility: ESOL model (Delaney 2004)
    # logS = 0.16 - 0.63*logP - 0.0062*MW + 0.066*RB - 0.74
    est_logs = 0.16 - 0.63 * logp - 0.0062 * mw + 0.066 * d["rotatable_bonds"] - 0.74
    endpoints["solubility"] = {"value": round(est_logs, 2), "unit": "log S", "confidence": 0.4}

    # HIA: PSA < 140 and MW < 500 -> likely high absorption
    hia_prob = 1.0 if (psa < 140 and mw < 500) else 0.3
    endpoints["hia"] = {"value": hia_prob > 0.5, "confidence": 0.3}

    # Caco-2: rough estimate from PSA
    est_caco2 = -5.5 + 0.02 * (140 - min(psa, 200))
    endpoints["caco2"] = {"value": round(est_caco2, 2), "unit": "log Papp (cm/s)", "confidence": 0.3}

    # P-gp substrate: high MW + high PSA -> more likely substrate
    pgp_prob = 0.7 if (mw > 400 and psa > 100) else 0.3
    endpoints["pgp_substrate"] = {"value": pgp_prob > 0.5, "confidence": 0.25}
    endpoints["pgp_inhibitor"] = {"value": logp > 3.5, "confidence": 0.25}

    # Bioavailability: Lipinski compliance as proxy
    ro5_violations = sum([mw > 500, logp > 5, hba > 10, hbd > 5])
    endpoints["bioavailability"] = {"value": ro5_violations <= 1, "confidence": 0.3}

    # PPB: high logP -> high protein binding
    est_ppb = min(99, max(20, 50 + 10 * logp))
    endpoints["ppb"] = {"value": round(est_ppb, 1), "unit": "% bound", "confidence": 0.3}

    # VDss: rough estimate
    est_vd = 0.5 + 0.3 * logp
    endpoints["vdss"] = {"value": round(est_vd, 2), "unit": "L/kg", "confidence": 0.2}

    # BBB: MW < 400 and PSA < 90 and logP 1-3 -> likely penetrant
    bbb_prob = 0.7 if (mw < 400 and psa < 90 and 1 < logp < 3) else 0.3
    endpoints["bbb"] = {"value": bbb_prob > 0.5, "confidence": 0.25}

    # CYP panel: logP-based heuristics
    cyp_isoforms = ["1a2", "2c9", "2c19", "2d6", "3a4"]
    for iso in cyp_isoforms:
        # Substrates: most drugs are metabolized by CYP3A4
        sub_prob = 0.6 if iso == "3a4" else 0.3
        endpoints[f"cyp{iso}_substrate"] = {"value": sub_prob > 0.5, "confidence": 0.2}
        # Inhibitors: high logP correlates with CYP inhibition
        inh_prob = 0.6 if logp > 3.0 else 0.2
        endpoints[f"cyp{iso}_inhibitor"] = {"value": inh_prob > 0.5, "confidence": 0.2}

    # Clearance: rough estimate
    est_cl = max(0.5, 5.0 + 2.0 * logp)
    endpoints["clearance"] = {"value": round(est_cl, 1), "unit": "mL/min/kg", "confidence": 0.2}

    # Half-life: rough estimate (inversely related to clearance)
    est_t12 = max(0.5, 10.0 / max(0.1, est_cl / 5.0))
    endpoints["half_life"] = {"value": round(est_t12, 1), "unit": "hours", "confidence": 0.2}

    # Toxicity endpoints
    # hERG: logP > 3.5 and basic nitrogen -> higher risk
    herg_prob = 0.5 if logp > 3.5 else 0.2
    endpoints["herg"] = {"value": herg_prob > 0.5, "confidence": 0.25}

    # AMES: rule-of-thumb (not very predictive)
    endpoints["ames"] = {"value": False, "confidence": 0.2}

    # DILI/Hepatotoxicity
    endpoints["dili"] = {"value": logp > 3.0 and mw > 400, "confidence": 0.2}
    endpoints["hepatotoxicity"] = {"value": logp > 3.5, "confidence": 0.2}

    # Skin sensitization
    endpoints["skin_sensitization"] = {"value": False, "confidence": 0.15}

    # LD50: rough QSAR estimate
    est_ld50 = max(1.0, 3.0 - 0.3 * logp)
    endpoints["ld50"] = {"value": round(est_ld50, 2), "unit": "mg/kg (log)", "confidence": 0.2}

    # Carcinogenicity
    endpoints["carcinogenicity"] = {"value": False, "confidence": 0.15}

    return endpoints


# ---------------------------------------------------------------------------
# ML-based prediction (admet_ai)
# ---------------------------------------------------------------------------

def _ml_predict_batch(smiles_list: list[str]) -> list[dict[str, Any] | None]:
    """Run admet_ai predictions on a batch of SMILES."""
    if not _ADMET_AI_AVAILABLE or _admet_predictor is None:
        return [None] * len(smiles_list)

    try:
        app.logger.info("[admet] predicting %d SMILES on %s", len(smiles_list), _admet_predictor.device)
        t0 = time.time()
        preds_df = _admet_predictor.predict(smiles=smiles_list)
        elapsed = time.time() - t0
        app.logger.info("[admet] prediction complete: %d compounds in %.2fs (%.0f/s)",
                        len(smiles_list), elapsed, len(smiles_list) / elapsed)
        col_map = _build_column_map(list(preds_df.columns))

        results = []
        for idx in range(len(smiles_list)):
            row = preds_df.iloc[idx]
            endpoints = {}

            for df_col, canonical in col_map.items():
                val = row[df_col]
                meta = CANONICAL_ENDPOINTS.get(canonical, {})

                # Handle NaN
                if isinstance(val, float) and math.isnan(val):
                    continue

                if meta.get("continuous", False):
                    endpoints[canonical] = {
                        "value": round(float(val), 4),
                        "unit": meta.get("unit", ""),
                        "confidence": 0.8,
                    }
                else:
                    # Binary: admet_ai returns probability; threshold at 0.5
                    prob = float(val)
                    endpoints[canonical] = {
                        "value": prob >= 0.5,
                        "confidence": round(abs(prob - 0.5) * 2, 3),
                        "probability": round(prob, 4),
                    }

            results.append(endpoints)
        return results

    except Exception as exc:
        app.logger.error("admet_ai prediction failed: %s\n%s", exc, traceback.format_exc())
        return [None] * len(smiles_list)


def _predict_single(smiles: str, ml_result: dict[str, Any] | None) -> dict[str, Any] | None:
    """Merge ML prediction with rule-based fallback for missing endpoints."""
    if ml_result is not None:
        # Fill in any missing canonical endpoints from rule-based fallback.
        fallback = _rule_based_predict(smiles)
        if fallback:
            for endpoint, fb_data in fallback.items():
                if endpoint not in ml_result:
                    fb_data["source"] = "rule-based"
                    ml_result[endpoint] = fb_data
        return ml_result

    # Pure rule-based fallback.
    result = _rule_based_predict(smiles)
    if result:
        for ep_data in result.values():
            ep_data["source"] = "rule-based"
    return result


def _compute_flags(endpoints: dict[str, Any]) -> dict[str, Any]:
    """Derive safety/alert flags from ADMET endpoints."""
    flags = {}

    # hERG alert
    herg = endpoints.get("herg", {})
    flags["herg_alert"] = bool(herg.get("value", False))

    # AMES positive
    ames = endpoints.get("ames", {})
    flags["ames_positive"] = bool(ames.get("value", False))

    # DILI risk
    dili = endpoints.get("dili", {})
    flags["dili_risk"] = bool(dili.get("value", False))

    # CYP liability count: how many CYP isoforms are inhibited
    cyp_count = 0
    for iso in ["1a2", "2c9", "2c19", "2d6", "3a4"]:
        inh = endpoints.get(f"cyp{iso}_inhibitor", {})
        if inh.get("value", False):
            cyp_count += 1
    flags["cyp_liability_count"] = cyp_count

    # Low bioavailability
    bio = endpoints.get("bioavailability", {})
    flags["low_bioavailability"] = not bool(bio.get("value", True))

    return flags


# ---------------------------------------------------------------------------
# MPO scoring
# ---------------------------------------------------------------------------

# Preset MPO weights: each maps endpoint -> (weight, target_direction)
# target_direction: "high" = higher is better, "low" = lower is better,
#                   "true" = True is desired, "false" = False is desired
MPO_PRESETS = {
    "oral": {
        "hia":              (0.15, "true"),
        "caco2":            (0.10, "high"),
        "bioavailability":  (0.15, "true"),
        "solubility":       (0.10, "high"),
        "cyp3a4_inhibitor": (0.08, "false"),
        "cyp2d6_inhibitor": (0.07, "false"),
        "clearance":        (0.10, "low"),
        "herg":             (0.10, "false"),
        "ames":             (0.08, "false"),
        "dili":             (0.07, "false"),
    },
    "cns": {
        "bbb":              (0.20, "true"),
        "pgp_substrate":    (0.15, "false"),
        "ppb":              (0.10, "low"),
        "hia":              (0.10, "true"),
        "herg":             (0.15, "false"),
        "cyp2d6_inhibitor": (0.10, "false"),
        "clearance":        (0.10, "low"),
        "ames":             (0.10, "false"),
    },
    "oncology": {
        "solubility":       (0.12, "high"),
        "caco2":            (0.10, "high"),
        "hia":              (0.10, "true"),
        "clearance":        (0.12, "low"),
        "herg":             (0.20, "false"),
        "hepatotoxicity":   (0.12, "false"),
        "bioavailability":  (0.12, "true"),
        "cyp3a4_inhibitor": (0.12, "false"),
    },
    "antimicrobial": {
        "solubility":       (0.20, "high"),
        "caco2":            (0.15, "high"),
        "clearance":        (0.15, "low"),
        "ames":             (0.20, "false"),
        "hia":              (0.10, "true"),
        "herg":             (0.10, "false"),
        "dili":             (0.10, "false"),
    },
}


def _score_endpoint(endpoint_data: dict[str, Any], direction: str) -> float:
    """Score a single endpoint on 0-1 scale based on desired direction."""
    val = endpoint_data.get("value")
    if val is None:
        return 0.5  # unknown = neutral

    if direction == "true":
        return 1.0 if bool(val) else 0.0
    elif direction == "false":
        return 0.0 if bool(val) else 1.0
    elif direction == "high":
        # For continuous values, normalize. Higher is better.
        # Use sigmoid-like scaling centered at 0.
        fval = float(val)
        return min(1.0, max(0.0, 0.5 + fval * 0.1))
    elif direction == "low":
        # Lower is better.
        fval = float(val)
        return min(1.0, max(0.0, 0.5 - fval * 0.05))
    return 0.5


def compute_mpo(endpoints: dict[str, Any], profile: str) -> float:
    """Compute MPO score (0-100) for a compound's ADMET endpoints."""
    weights = MPO_PRESETS.get(profile)
    if not weights:
        return 50.0  # unknown profile -> neutral

    total_weight = 0.0
    weighted_sum = 0.0

    for endpoint_name, (weight, direction) in weights.items():
        ep_data = endpoints.get(endpoint_name, {})
        score = _score_endpoint(ep_data, direction)
        weighted_sum += weight * score
        total_weight += weight

    if total_weight == 0:
        return 50.0

    raw_score = weighted_sum / total_weight
    return round(raw_score * 100, 1)


# ---------------------------------------------------------------------------
# Flask endpoints
# ---------------------------------------------------------------------------

@app.route("/health", methods=["GET"])
def health():
    """Liveness probe."""
    return jsonify({
        "status": "healthy",
        "admet_ai_available": _ADMET_AI_AVAILABLE,
    })


@app.route("/readyz", methods=["GET"])
def readyz():
    """Readiness probe. Ready once at least rule-based fallback is available."""
    # We are always ready since rule-based fallback is always available.
    # If admet_ai loaded, note the model load time.
    info = {"ready": True, "engine": "admet_ai" if _ADMET_AI_AVAILABLE else "rule-based"}
    if _model_load_time:
        info["model_loaded_at"] = _model_load_time
    return jsonify(info)


@app.route("/predict", methods=["POST"])
def predict():
    """Batch ADMET prediction.

    Input:
        {
            "smiles_list": ["CCO", "c1ccccc1"],
            "endpoints": "all"
        }

    Returns per-compound predictions with all ADMET endpoints.
    """
    data = request.get_json(force=True)
    smiles_list = data.get("smiles_list", [])

    if not smiles_list:
        return jsonify({"error": "smiles_list is required"}), 400

    if len(smiles_list) > 10000:
        return jsonify({"error": "batch size limited to 10000 SMILES"}), 400

    # Run ML predictions in batch.
    ml_results = _ml_predict_batch(smiles_list)

    results = []
    for i, smiles in enumerate(smiles_list):
        ml_result = ml_results[i] if i < len(ml_results) else None
        endpoints = _predict_single(smiles, ml_result)

        if endpoints is None:
            results.append({
                "smiles": smiles,
                "error": "invalid SMILES or prediction failed",
                "endpoints": {},
                "flags": {},
            })
            continue

        flags = _compute_flags(endpoints)

        results.append({
            "smiles": smiles,
            "endpoints": endpoints,
            "flags": flags,
            "engine": "admet_ai" if (_ADMET_AI_AVAILABLE and ml_result is not None) else "rule-based",
        })

    return jsonify({
        "predictions": results,
        "count": len(results),
        "engine": "admet_ai" if _ADMET_AI_AVAILABLE else "rule-based",
    })


@app.route("/mpo", methods=["POST"])
def mpo():
    """Multi-parameter optimization scoring.

    Input:
        {
            "predictions": [ ... per-compound prediction objects ... ],
            "profile": "oral"
        }

    Returns MPO score (0-100) per compound.
    """
    data = request.get_json(force=True)
    predictions = data.get("predictions", [])
    profile = data.get("profile", "oral")

    if profile not in MPO_PRESETS:
        return jsonify({
            "error": f"unknown profile: {profile}; valid: {list(MPO_PRESETS.keys())}",
        }), 400

    scores = []
    for pred in predictions:
        endpoints = pred.get("endpoints", {})
        score = compute_mpo(endpoints, profile)
        scores.append({
            "smiles": pred.get("smiles", ""),
            "mpo_score": score,
            "mpo_profile": profile,
        })

    return jsonify({
        "scores": scores,
        "profile": profile,
        "count": len(scores),
    })


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8000, debug=False)
