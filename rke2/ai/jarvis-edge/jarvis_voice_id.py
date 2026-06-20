"""Voice identification + per-user inventory for JARVIS.

Stores enrolled voices at ``/state/voices/`` and a per-user
knowledge-base directory at ``/state/users/<slug>/`` that other
parts of JARVIS can read to personalize responses.

Layout::

    /state/voices/
        hampton.npy           # mean speaker embedding (256-dim float32)
        hampton.json          # {name, slug, role: owner|trusted, enrolled_at,
                              #  enrolled_by, num_clips, model}
        _pending.json         # single in-flight unknown speaker awaiting
                              # owner authentication. Holds name + embedding
                              # as a list. Cleared on auth or on next unknown.

    /state/users/<slug>/
        profile.md            # human-edited "what JARVIS should know about
                              # this person" notes. Optional. Loaded by the
                              # daemon and injected into the brain's per-turn
                              # context.
        memories.jsonl        # JARVIS-appended observations. Future.

Roles:
- ``owner`` — first-enrolled user (Hampton). Sole authority to authenticate
  new voices via "Jarvis, authenticate them" / "Jarvis, this is <name>".
- ``trusted`` — enrolled by the owner. Full daemon access.
- ``unknown`` — never matched. Daemon refuses commands; if addressed, JARVIS
  asks "I don't recognize you. What is your name?" and stashes the embedding
  in ``_pending.json`` to await owner authentication.

The actual gating + state-machine lives in ``voice_daemon_cmd.py``; this
module just gives it the primitives. Local/uncommitted like everything else.
"""
from __future__ import annotations

import json
import os
import re
import time
from pathlib import Path
from typing import Optional

import numpy as np

_VOICES_DIR = Path("/state/voices")
_USERS_DIR = Path("/state/users")
_PENDING_PATH = _VOICES_DIR / "_pending.json"

# ── Embedding model selection ─────────────────────────────────────────────────
# VOICE_MODEL chooses the embedding backend. DEFAULT is resemblyzer so an
# un-set flag is byte-for-byte behaviour-identical to the historical daemon.
#   resemblyzer → Resemblyzer 2019 d-vectors, 256-d, ~5% EER (legacy fallback)
#   campplus    → 3D-Speaker / wespeaker CAM++ ONNX, 192-d, ~0.65% EER (16k EN)
# The stored .json carries a ``model`` field; identify() refuses to score an
# embedding from a different model than the active one (incompatible spaces).
_VOICE_MODEL = (os.environ.get("VOICE_MODEL", "resemblyzer") or "resemblyzer").strip().lower()
if _VOICE_MODEL not in ("resemblyzer", "campplus"):
    _VOICE_MODEL = "resemblyzer"

if _VOICE_MODEL == "campplus":
    _MODEL_NAME = "campplus-voxceleb-en-16k"
    _EMBED_DIM = 192
else:
    _MODEL_NAME = "resemblyzer-1.0"
    _EMBED_DIM = 256

# CAM++ ONNX model location inside the image (vendored / build-time fetched).
# See Dockerfile + RUNBOOK for provenance. Overridable for local testing.
_CAMPPLUS_ONNX_PATH = os.environ.get(
    "CAMPPLUS_ONNX_PATH", "/app/models/campplus_voxceleb.onnx")
# CAM++ kaldi-fbank frontend spec (MUST match the model's training frontend:
# 3D-Speaker / wespeaker CAM++ → 80-dim fbank, 16 kHz, 25 ms window / 10 ms
# hop, with per-utterance mean normalisation (CMN). See RUNBOOK "I/O spec".
_CAMPPLUS_SR = 16000
_CAMPPLUS_NUM_MEL_BINS = 80
_CAMPPLUS_FRAME_LENGTH_MS = 25.0
_CAMPPLUS_FRAME_SHIFT_MS = 10.0

# Cosine-similarity thresholds. Resemblyzer's typical "same speaker"
# region is ≥0.70; we accept ≥0.70 as an identification, ≥0.60 as a
# borderline match worth retrying. Below that = unknown.
#
# NOTE: these defaults are RESEMBLYZER-tuned. CAM++ cosine score
# distributions differ — run calibrate_voiceid.py and override
# _THRESHOLD_MATCH / _THRESHOLD_BORDER / OWNER_THRESHOLD via env when
# VOICE_MODEL=campplus. All three are env-overridable below.
_THRESHOLD_MATCH = float(os.environ.get("VOICE_THRESHOLD_MATCH", "0.70"))
_THRESHOLD_BORDER = float(os.environ.get("VOICE_THRESHOLD_BORDER", "0.60"))

# Granting OWNER (full access) requires a STRICTER match than mere
# same-speaker identification. An owner false-positive hands a stranger full
# access to Hampton's data; an owner false-negative just re-challenges
# Hampton. So the identity resolver (jarvis_identity.resolve_voice) downgrades
# an owner match scoring below this to TRUSTED rather than granting OWNER.
# identify() itself is unchanged — this only governs the OWNER *grant*.
# Env-overridable: CAM++ calibration (calibrate_voiceid.py) recommends a
# model-appropriate value; the 0.75 default is Resemblyzer-tuned.
OWNER_THRESHOLD = float(os.environ.get("VOICE_OWNER_THRESHOLD", "0.75"))

# ── Continuous voice adaptation (learn the owner's voice over time) ───────────
# Enrollment is a one-shot averaged reference; voices drift (time of day,
# illness, mic position) and the static template slowly mismatches. We fold
# HIGH-CONFIDENCE owner utterances back into the stored embedding via a decaying
# running average so recognition sharpens + tracks the owner over time.
#
# GUARDED HARD against drift onto another voice:
#   * Only adapt on a score WELL ABOVE the owner grant threshold
#     (ADAPT_MIN_SCORE, default 0.82 ≫ OWNER_THRESHOLD 0.75). A borderline or
#     impostor-range match NEVER adapts — the dangerous direction is pulling
#     the owner template toward a stranger, so we bias far against it.
#   * Tiny per-turn step (ADAPT_ALPHA, default 0.05): even a rare bad accept
#     moves the template only marginally, and subsequent genuine owner turns
#     pull it back. The template can never lurch onto a new voice in one turn.
#   * Cheap: one dot product + normalize, no re-embedding, no disk scan beyond
#     the single owner .npy. Safe to call every owner turn.
ADAPT_ENABLED = os.environ.get("VOICE_ADAPT_ENABLED", "1") == "1"
# NOTE: 0.82 default is Resemblyzer-tuned; calibrate per-model and override.
ADAPT_MIN_SCORE = float(os.environ.get("VOICE_ADAPT_MIN_SCORE", "0.82"))
ADAPT_ALPHA = float(os.environ.get("VOICE_ADAPT_ALPHA", "0.05"))

# Singleton encoders — loaded lazily so importing this module is cheap.
# Resemblyzer's VoiceEncoder and the CAM++ onnxruntime session each get their
# own singleton; only the one selected by VOICE_MODEL is ever instantiated.
_encoder = None          # resemblyzer VoiceEncoder
_onnx_session = None      # onnxruntime InferenceSession for CAM++


def _enc():
    """Lazy-load the Resemblyzer encoder (legacy/fallback path)."""
    global _encoder
    if _encoder is None:
        from resemblyzer import VoiceEncoder
        _encoder = VoiceEncoder(verbose=False)
    return _encoder


# ── CAM++ (3D-Speaker / wespeaker) ONNX backend ──────────────────────────────

def _campplus_session():
    """Lazy-load the CAM++ onnxruntime InferenceSession (CPU)."""
    global _onnx_session
    if _onnx_session is None:
        import onnxruntime as ort
        if not Path(_CAMPPLUS_ONNX_PATH).exists():
            raise FileNotFoundError(
                f"CAM++ ONNX model not found at {_CAMPPLUS_ONNX_PATH}. "
                "It must be vendored into the image (see Dockerfile/RUNBOOK) "
                "or CAMPPLUS_ONNX_PATH must point at the .onnx file.")
        so = ort.SessionOptions()
        so.intra_op_num_threads = int(os.environ.get("CAMPPLUS_THREADS", "2"))
        _onnx_session = ort.InferenceSession(
            _CAMPPLUS_ONNX_PATH, sess_options=so,
            providers=["CPUExecutionProvider"])
    return _onnx_session


def _campplus_fbank(audio: np.ndarray, sample_rate: int) -> "np.ndarray":
    """float32 mono ndarray → (T, 80) CMN-normalised kaldi fbank at 16 kHz.

    Mirrors the CAM++/wespeaker training frontend exactly: resample→16k mono,
    80-dim kaldi fbank (25 ms / 10 ms), then per-utterance cepstral mean
    normalisation (subtract the per-bin mean over time)."""
    import torch
    import torchaudio
    import torchaudio.compliance.kaldi as kaldi

    wav = np.asarray(audio, dtype=np.float32)
    if wav.ndim > 1:  # collapse to mono
        wav = wav.mean(axis=1)
    t = torch.from_numpy(wav).unsqueeze(0)  # (1, N)
    if sample_rate != _CAMPPLUS_SR:
        t = torchaudio.functional.resample(t, sample_rate, _CAMPPLUS_SR)
    # kaldi.fbank expects int16-scaled floats by default; wespeaker trains on
    # the standard kaldi pipeline. Use raw_energy/default opts matching CAM++.
    feat = kaldi.fbank(
        t,
        num_mel_bins=_CAMPPLUS_NUM_MEL_BINS,
        frame_length=_CAMPPLUS_FRAME_LENGTH_MS,
        frame_shift=_CAMPPLUS_FRAME_SHIFT_MS,
        dither=0.0,
        sample_frequency=float(_CAMPPLUS_SR),
    )  # (T, 80)
    # Per-utterance CMN (subtract the mean fbank over time).
    feat = feat - feat.mean(dim=0, keepdim=True)
    return feat.numpy().astype(np.float32)


def _campplus_embed(audio: np.ndarray, sample_rate: int) -> np.ndarray:
    """float32 mono ndarray + source SR → L2-normalised 192-d CAM++ embedding."""
    feat = _campplus_fbank(audio, sample_rate)            # (T, 80)
    sess = _campplus_session()
    inp = sess.get_inputs()[0]
    # CAM++ ONNX expects a batched feature tensor (1, T, 80) float32.
    x = feat[np.newaxis, :, :].astype(np.float32)
    out = sess.run(None, {inp.name: x})[0]
    emb = np.asarray(out, dtype=np.float32).reshape(-1)   # (192,)
    emb /= (np.linalg.norm(emb) + 1e-9)
    return emb.astype(np.float32)


def _slugify(name: str) -> str:
    s = re.sub(r"[^a-z0-9]+", "_", (name or "").lower()).strip("_")
    return s or "unknown"


def _ensure_dirs() -> None:
    _VOICES_DIR.mkdir(parents=True, exist_ok=True)
    _USERS_DIR.mkdir(parents=True, exist_ok=True)


# ── enrollment ───────────────────────────────────────────────────────────────

def embed_from_wav(path: str | Path) -> np.ndarray:
    """Compute an L2-normalisable speaker embedding from a WAV/audio file.

    Dispatches on VOICE_MODEL. The resemblyzer path is unchanged; the campplus
    path loads the WAV via soundfile and routes through the CAM++ frontend."""
    if _VOICE_MODEL == "campplus":
        import soundfile as sf
        wav, sr = sf.read(str(path), dtype="float32", always_2d=False)
        return _campplus_embed(np.asarray(wav, dtype=np.float32), int(sr))
    from resemblyzer import preprocess_wav
    wav = preprocess_wav(Path(path))
    return _enc().embed_utterance(wav).astype(np.float32)


def embed_from_audio(audio: np.ndarray, sample_rate: int) -> np.ndarray:
    """Compute an embedding directly from a float32 mono numpy array.

    Dispatches on VOICE_MODEL. The resemblyzer path is unchanged; the campplus
    path runs the kaldi-fbank → ONNX frontend. Both return an L2-normalisable
    float32 embedding with the same call interface."""
    if _VOICE_MODEL == "campplus":
        return _campplus_embed(np.asarray(audio, dtype=np.float32),
                               int(sample_rate))
    from resemblyzer import preprocess_wav
    # preprocess_wav accepts (np.ndarray, source_sr) by passing the array
    # AND the source sample rate (Resemblyzer downsamples to 16k internally).
    wav = preprocess_wav(audio, source_sr=sample_rate)
    return _enc().embed_utterance(wav).astype(np.float32)


def enroll(name: str, embeddings: list[np.ndarray] | np.ndarray,
           role: str = "trusted", enrolled_by: str = "") -> dict:
    """Persist a speaker. ``embeddings`` is either a list of embeddings
    (averaged) or a single embedding. ``role`` is owner|trusted."""
    _ensure_dirs()
    if isinstance(embeddings, np.ndarray) and embeddings.ndim == 1:
        embs = [embeddings]
    else:
        embs = list(embeddings)
    if not embs:
        raise ValueError("no embeddings provided")
    mean = np.mean(np.stack(embs, axis=0), axis=0).astype(np.float32)
    # Normalize so cosine similarity is just a dot product later.
    mean /= np.linalg.norm(mean) + 1e-9

    slug = _slugify(name)
    np.save(_VOICES_DIR / f"{slug}.npy", mean)
    meta = {
        "name": name,
        "slug": slug,
        "role": role,
        "enrolled_at": int(time.time()),
        "enrolled_by": enrolled_by,
        "num_clips": len(embs),
        "model": _MODEL_NAME,
    }
    with open(_VOICES_DIR / f"{slug}.json", "w") as f:
        json.dump(meta, f, indent=2)

    # Scaffold per-user knowledge-base dir + an empty profile the user can edit.
    user_dir = _USERS_DIR / slug
    user_dir.mkdir(parents=True, exist_ok=True)
    profile_path = user_dir / "profile.md"
    if not profile_path.exists():
        with open(profile_path, "w") as f:
            f.write(
                f"# {name}\n\n"
                f"_Enrolled {time.strftime('%Y-%m-%d')} as **{role}**._\n\n"
                "## What JARVIS should know\n"
                "<!-- Free-form notes the brain reads as context when this "
                "person speaks. Examples: location, preferences, in-progress "
                "projects, communication style. -->\n\n"
                "## Interests\n"
                "<!-- Comma-separated keywords. The morning briefing filters "
                "overnight news by these — anything matching gets a 'worth "
                "your attention, sir' framing; everything else is dropped. "
                "Replace the example line below with your own. -->\n"
                "Interests: ai, kubernetes, anthropic, claude, drum and bass, "
                "murfreesboro, nashville, cs2\n\n"
                "## Don't do\n"
                "<!-- Things JARVIS should refuse or warn about for this "
                "person specifically. -->\n"
            )
    return meta


def adapt_owner(embedding: np.ndarray, score: float) -> dict:
    """Fold a HIGH-CONFIDENCE owner utterance into the owner's stored template
    via a decaying running average. Returns a status dict (never raises into
    the caller — fails closed by skipping adaptation).

    Hard guards (see ADAPT_* constants):
      * adaptation must be enabled,
      * ``score`` MUST be >= ADAPT_MIN_SCORE (well above OWNER_THRESHOLD), so
        only an unambiguous owner match can ever move the template,
      * an owner must actually be enrolled.

    On adapt: ``new = norm((1-α)·old + α·utterance)``. α is small, so a single
    accidental accept barely moves the template and genuine owner turns pull it
    back. ``num_clips`` is bumped so the metadata reflects the adaptation count.
    Persisted in place to /state/voices/<owner>.npy + .json (atomic-ish: numpy
    save then json dump)."""
    if not ADAPT_ENABLED:
        return {"status": "skipped", "reason": "disabled"}
    # GUARD: only adapt on a score WELL above the owner grant threshold. This
    # is the security-critical line — never adapt on borderline/impostor scores.
    if not (float(score) >= ADAPT_MIN_SCORE):
        return {"status": "skipped", "reason": "below_adapt_threshold",
                "score": float(score), "min": ADAPT_MIN_SCORE}
    owner = None
    for e in _load_enrolled():
        if e["role"] == "owner":
            owner = e
            break
    if owner is None:
        return {"status": "skipped", "reason": "no_owner"}
    try:
        emb = np.asarray(embedding, dtype=np.float32)
        emb_n = emb / (np.linalg.norm(emb) + 1e-9)
        old = np.asarray(owner["embedding"], dtype=np.float32)
        old_n = old / (np.linalg.norm(old) + 1e-9)
        blended = (1.0 - ADAPT_ALPHA) * old_n + ADAPT_ALPHA * emb_n
        blended /= np.linalg.norm(blended) + 1e-9
        blended = blended.astype(np.float32)

        slug = owner["slug"]
        np.save(_VOICES_DIR / f"{slug}.npy", blended)
        # Bump metadata so list_enrolled / debugging shows the adaptation count.
        meta_path = _VOICES_DIR / f"{slug}.json"
        try:
            with open(meta_path) as f:
                meta = json.load(f)
        except (OSError, ValueError):
            meta = {k: v for k, v in owner.items()
                    if k not in ("embedding", "profile_path")}
        meta["num_clips"] = int(meta.get("num_clips", 1)) + 1
        meta["adapted_at"] = int(time.time())
        meta["adapt_count"] = int(meta.get("adapt_count", 0)) + 1
        with open(meta_path, "w") as f:
            json.dump(meta, f, indent=2)
        # How far the template moved this turn (cosine of old vs new). Useful
        # for spotting drift in logs/traces; should stay very close to 1.0.
        moved = float(np.dot(old_n, blended))
        return {"status": "adapted", "slug": slug, "score": float(score),
                "alpha": ADAPT_ALPHA, "template_cos": round(moved, 5),
                "adapt_count": meta["adapt_count"]}
    except Exception as exc:  # noqa: BLE001
        return {"status": "error", "message": str(exc)}


def remove(name: str) -> bool:
    slug = _slugify(name)
    removed = False
    for ext in (".npy", ".json"):
        p = _VOICES_DIR / f"{slug}{ext}"
        if p.exists():
            p.unlink()
            removed = True
    # Intentionally do NOT delete /state/users/<slug>/ — the KB may
    # have hand-edited notes worth keeping if the user re-enrolls.
    return removed


# ── identification ───────────────────────────────────────────────────────────

def _load_enrolled(*, include_incompatible: bool = False) -> list[dict]:
    """Return list of {slug, name, role, embedding, profile_path}.

    CROSS-MODEL GUARD: a voice whose stored ``model`` field differs from the
    active ``_MODEL_NAME`` is SKIPPED (treated as not enrolled) — Resemblyzer
    (256-d) and CAM++ (192-d) embeddings live in incompatible spaces and a
    cosine across them is meaningless garbage. This makes a flag flip BEFORE
    re-enrollment fail safe (the owner reads as no_enrollments → re-enroll)
    rather than silently mis-scoring. Voices missing a ``model`` field are
    assumed legacy resemblyzer. Pass include_incompatible=True only for
    diagnostics / migration tooling that wants the full inventory."""
    out = []
    if not _VOICES_DIR.exists():
        return out
    for meta_path in sorted(_VOICES_DIR.glob("*.json")):
        if meta_path.name.startswith("_"):
            continue
        try:
            with open(meta_path) as f:
                meta = json.load(f)
        except (OSError, ValueError):
            continue
        stored_model = meta.get("model", "resemblyzer-1.0")
        if not include_incompatible and stored_model != _MODEL_NAME:
            # Enrolled under a different embedding model — skip (force re-enroll).
            continue
        emb_path = _VOICES_DIR / f"{meta['slug']}.npy"
        if not emb_path.exists():
            continue
        try:
            emb = np.load(emb_path)
        except Exception:
            continue
        out.append({**meta, "embedding": emb,
                    "profile_path": str(_USERS_DIR / meta["slug"] / "profile.md")})
    return out


def list_enrolled(*, include_incompatible: bool = False) -> list[dict]:
    """Public-facing list without the embedding blob. By default only voices
    compatible with the active model. Pass include_incompatible=True to also
    see voices enrolled under a different model (each tagged
    ``compatible: False``) — useful for explaining why the owner 'vanished'
    after a VOICE_MODEL flip before re-enrollment."""
    if not include_incompatible:
        return [{k: v for k, v in e.items() if k != "embedding"}
                for e in _load_enrolled()]
    out = []
    for e in _load_enrolled(include_incompatible=True):
        rec = {k: v for k, v in e.items() if k != "embedding"}
        rec["compatible"] = (e.get("model", "resemblyzer-1.0") == _MODEL_NAME)
        out.append(rec)
    return out


def identify(embedding: np.ndarray) -> dict:
    """Compare an embedding against the enrolled inventory.
    Returns ``{status, name?, role?, slug?, score, ranking}``.

    ``status`` ∈ ``{match, borderline, unknown, no_enrollments}``."""
    enrolled = _load_enrolled()
    if not enrolled:
        return {"status": "no_enrollments", "score": 0.0, "ranking": []}
    emb = embedding.astype(np.float32)
    emb_n = emb / (np.linalg.norm(emb) + 1e-9)
    ranking = []
    for e in enrolled:
        ref = e["embedding"]
        ref_n = ref / (np.linalg.norm(ref) + 1e-9)
        score = float(np.dot(emb_n, ref_n))
        ranking.append({"name": e["name"], "slug": e["slug"],
                        "role": e["role"], "score": round(score, 3)})
    ranking.sort(key=lambda r: r["score"], reverse=True)
    top = ranking[0]
    if top["score"] >= _THRESHOLD_MATCH:
        status = "match"
    elif top["score"] >= _THRESHOLD_BORDER:
        status = "borderline"
    else:
        status = "unknown"
    return {"status": status, "name": top["name"], "slug": top["slug"],
            "role": top["role"], "score": top["score"], "ranking": ranking}


# ── pending / authentication state machine ───────────────────────────────────

def stash_pending(name: str, embedding: np.ndarray) -> dict:
    """Park an unknown speaker who's told us their name, awaiting owner
    authentication. Overwrites any prior pending entry."""
    _ensure_dirs()
    payload = {
        "name": name,
        "slug": _slugify(name),
        "embedding": embedding.astype(np.float32).tolist(),
        "captured_at": int(time.time()),
    }
    with open(_PENDING_PATH, "w") as f:
        json.dump(payload, f)
    return {"name": name, "slug": payload["slug"]}


def get_pending() -> Optional[dict]:
    if not _PENDING_PATH.exists():
        return None
    try:
        with open(_PENDING_PATH) as f:
            p = json.load(f)
    except (OSError, ValueError):
        return None
    p["embedding"] = np.asarray(p["embedding"], dtype=np.float32)
    return p


def clear_pending() -> None:
    if _PENDING_PATH.exists():
        _PENDING_PATH.unlink()


def authenticate_pending(authorizer_slug: str,
                         override_name: str = "") -> dict:
    """Promote the in-flight pending speaker to ``trusted``. The authorizer
    MUST be the owner (or another trusted user; v1 we restrict to owner)."""
    enrolled = _load_enrolled()
    authz = next((e for e in enrolled if e["slug"] == authorizer_slug), None)
    if authz is None:
        return {"status": "error", "message": "authorizer not enrolled"}
    if authz["role"] != "owner":
        return {"status": "error",
                "message": "only the owner can authenticate new speakers"}
    pending = get_pending()
    if pending is None:
        return {"status": "error", "message": "no pending enrollment"}
    name = (override_name or pending["name"] or "guest").strip()
    enroll(name, embeddings=[pending["embedding"]], role="trusted",
           enrolled_by=authz["name"])
    clear_pending()
    return {"status": "ok", "name": name, "role": "trusted"}


def has_owner() -> bool:
    return any(e["role"] == "owner" for e in _load_enrolled())


def get_owner() -> Optional[dict]:
    for e in _load_enrolled():
        if e["role"] == "owner":
            return {k: v for k, v in e.items() if k != "embedding"}
    return None


def get_owner_slug() -> Optional[str]:
    """Slug of the enrolled owner, or None. Used by the auth state machine to
    pass an authorizer to authenticate_pending()."""
    o = get_owner()
    return o["slug"] if o else None


# ── owner passphrase (fail-OPEN degraded-owner fallback) ─────────────────────
# When the voiceprint recognises the owner WEAKLY (top match is the owner
# template but the score is below the OWNER grant bar), the fail-open gate can
# offer a spoken passphrase challenge instead of silently dropping the owner
# (the historical fail-CLOSED lockout). The passphrase is owner-set, stored on
# the PVC next to the voiceprints, and compared case/whitespace-insensitively.
# A passphrase is OPTIONAL: with none set, the degraded-owner path falls back
# to a simple "it's me, sir?" spoken confirmation (see edge.py gate).
_PASSPHRASE_PATH = _VOICES_DIR / "_owner_passphrase.json"


def _normalize_passphrase(s: str) -> str:
    return re.sub(r"\s+", " ", (s or "").strip().lower())


def set_owner_passphrase(passphrase: str) -> dict:
    """Persist the owner's spoken passphrase for the degraded-owner fallback.
    Stored normalised (lowercased, collapsed whitespace). Empty clears it."""
    _ensure_dirs()
    norm = _normalize_passphrase(passphrase)
    if not norm:
        if _PASSPHRASE_PATH.exists():
            _PASSPHRASE_PATH.unlink()
        return {"status": "cleared"}
    with open(_PASSPHRASE_PATH, "w") as f:
        json.dump({"passphrase": norm, "set_at": int(time.time())}, f)
    return {"status": "set"}


def has_owner_passphrase() -> bool:
    return _PASSPHRASE_PATH.exists()


def check_owner_passphrase(spoken: str) -> bool:
    """True iff a passphrase is set AND the spoken text contains/equals it.
    Substring match (normalised) so the owner can say it inside a sentence."""
    if not _PASSPHRASE_PATH.exists():
        return False
    try:
        with open(_PASSPHRASE_PATH) as f:
            stored = _normalize_passphrase(json.load(f).get("passphrase", ""))
    except (OSError, ValueError):
        return False
    if not stored:
        return False
    return stored in _normalize_passphrase(spoken)


# ── per-user knowledge base accessor ─────────────────────────────────────────

def load_profile(slug: str) -> str:
    """Return the markdown content of a user's profile.md, or ''. Used by the
    brain wrapper to compose per-turn context for the identified speaker."""
    p = _USERS_DIR / slug / "profile.md"
    if not p.exists():
        return ""
    try:
        return p.read_text(encoding="utf-8")
    except OSError:
        return ""
