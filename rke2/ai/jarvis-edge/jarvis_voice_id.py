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
_MODEL_NAME = "resemblyzer-1.0"
_EMBED_DIM = 256

# Cosine-similarity thresholds. Resemblyzer's typical "same speaker"
# region is ≥0.70; we accept ≥0.70 as an identification, ≥0.60 as a
# borderline match worth retrying. Below that = unknown.
_THRESHOLD_MATCH = 0.70
_THRESHOLD_BORDER = 0.60

# Granting OWNER (full access) requires a STRICTER match than mere
# same-speaker identification. An owner false-positive hands a stranger full
# access to Hampton's data; an owner false-negative just re-challenges
# Hampton. So the identity resolver (jarvis_identity.resolve_voice) downgrades
# an owner match scoring below this to TRUSTED rather than granting OWNER.
# identify() itself is unchanged — this only governs the OWNER *grant*.
OWNER_THRESHOLD = 0.75

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
ADAPT_MIN_SCORE = float(os.environ.get("VOICE_ADAPT_MIN_SCORE", "0.82"))
ADAPT_ALPHA = float(os.environ.get("VOICE_ADAPT_ALPHA", "0.05"))

# Singleton encoder — loaded lazily so importing this module is cheap.
_encoder = None


def _enc():
    global _encoder
    if _encoder is None:
        from resemblyzer import VoiceEncoder
        _encoder = VoiceEncoder(verbose=False)
    return _encoder


def _slugify(name: str) -> str:
    s = re.sub(r"[^a-z0-9]+", "_", (name or "").lower()).strip("_")
    return s or "unknown"


def _ensure_dirs() -> None:
    _VOICES_DIR.mkdir(parents=True, exist_ok=True)
    _USERS_DIR.mkdir(parents=True, exist_ok=True)


# ── enrollment ───────────────────────────────────────────────────────────────

def embed_from_wav(path: str | Path) -> np.ndarray:
    """Compute a Resemblyzer mean utterance embedding from a WAV/audio file."""
    from resemblyzer import preprocess_wav
    wav = preprocess_wav(Path(path))
    return _enc().embed_utterance(wav).astype(np.float32)


def embed_from_audio(audio: np.ndarray, sample_rate: int) -> np.ndarray:
    """Compute embedding directly from a float32 mono numpy array."""
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

def _load_enrolled() -> list[dict]:
    """Return list of {slug, name, role, embedding, profile_path}."""
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


def list_enrolled() -> list[dict]:
    """Public-facing list without the embedding blob."""
    return [{k: v for k, v in e.items() if k != "embedding"}
            for e in _load_enrolled()]


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
