"""Cross-domain identity → authorization for JARVIS (the spine).

Every front-end (edge mic loop, Mac ``jarvis listen`` ingest, Discord, IG)
resolves an incoming turn to a single ``Principal`` BEFORE the gate runs.
The gate (``gate_and_respond`` in edge.py) then decides capability purely from
the Principal's role — the model is never the security boundary.

This module sits ON TOP of ``jarvis_voice_id`` (it imports the voiceprint
primitives, it does NOT duplicate them) and adds:

  * ``Role`` / ``Principal``        — the normalized identity any domain emits
  * ``resolve_voice(embedding)``    — voiceprint → Principal. Granting OWNER
                                       requires score ≥ OWNER_THRESHOLD; a
                                       weaker owner match DOWNGRADES to TRUSTED
                                       (never lets a stranger reach OWNER).
  * ``resolve_platform(source,id)`` — Discord/IG id → Principal (env allowlists)
  * ``is_owner_referential(text)``  — Layer-B deterministic classifier: is a
                                       non-owner's turn asking about the owner?
                                       (over-broad on purpose → deflect)
  * ``capability_for(role)``        — base capability tier for a role
  * pending-state re-exports        — the enroll-by-voice state machine

Storage root is read from ``JARVIS_STATE_ROOT`` (default ``/state``) so this
exact file can later be imported by the openjarvis Mac daemon unchanged. On the
edge it is always ``/state`` (matching jarvis_voice_id). Local/uncommitted-
sensitive like the rest of the edge.
"""
from __future__ import annotations

import os
import re
from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path
from typing import Any, Optional

import jarvis_voice_id as _vid

# Re-export the voiceprint + pending primitives so callers use ONE module.
from jarvis_voice_id import (  # noqa: F401
    OWNER_THRESHOLD,
    authenticate_pending,
    clear_pending,
    embed_from_audio,
    get_owner,
    get_owner_slug,
    get_pending,
    has_owner,
    load_profile,
    remove,
    stash_pending,
)

_STATE_ROOT = Path(os.environ.get("JARVIS_STATE_ROOT", "/state"))


class Role(str, Enum):
    OWNER = "owner"
    TRUSTED = "trusted"
    UNKNOWN = "unknown"


@dataclass
class Principal:
    """The normalized identity every channel produces and the gate consumes."""

    role: Role
    user_id: str                 # "voice:<slug>" | "discord:<id>" | "ig:<igsid>"
    display_name: str = ""
    source: str = "voice"        # "voice" | "discord" | "ig"
    confidence: float = 0.0
    profile_path: Optional[str] = None
    embedding: Any = None        # carried for enroll-by-voice (Phase 2)
    raw: dict = field(default_factory=dict)

    @property
    def mem_scope(self) -> str:
        """Memory partition key. ALL owner identities (voice OR platform)
        collapse to the canonical ``owner`` scope so mem0 doesn't fragment the
        owner's memories per-domain. Everyone else gets their own user_id."""
        return "owner" if self.role is Role.OWNER else self.user_id

    @property
    def is_owner(self) -> bool:
        return self.role is Role.OWNER


def _profile_path(slug: str) -> str:
    return str(_STATE_ROOT / "users" / slug / "profile.md")


# ── voiceprint resolution ────────────────────────────────────────────────────

def resolve_voice(embedding, *, source: str = "voice") -> Principal:
    """Map a Resemblyzer embedding to a Principal.

    OWNER is granted ONLY on a strong match (score ≥ OWNER_THRESHOLD). A weaker
    match to the owner's voiceprint is DOWNGRADED to TRUSTED — the dangerous
    direction is a stranger scoring high enough to impersonate the owner, so we
    bias against it. A genuine trusted match stays TRUSTED; anything
    borderline / unknown / no-enrollments is UNKNOWN.
    """
    res = _vid.identify(embedding)
    status = res.get("status")
    if status != "match":  # borderline / unknown / no_enrollments
        return Principal(
            role=Role.UNKNOWN,
            user_id="voice:unknown",
            source=source,
            confidence=float(res.get("score", 0.0)),
            embedding=embedding,
            raw=res,
        )

    slug = res["slug"]
    name = res["name"]
    score = float(res["score"])
    matched_role = res["role"]  # "owner" | "trusted"

    if matched_role == "owner" and score >= OWNER_THRESHOLD:
        role = Role.OWNER
    else:
        # Owner match below OWNER_THRESHOLD downgrades here; a trusted match
        # stays trusted. Never UNKNOWN→OWNER.
        role = Role.TRUSTED

    return Principal(
        role=role,
        user_id=f"voice:{slug}",
        display_name=name,
        source=source,
        confidence=score,
        profile_path=_profile_path(slug),
        embedding=embedding,
        raw=res,
    )


# ── platform resolution (Discord / IG) ───────────────────────────────────────

def _csv_env(name: str) -> set[str]:
    raw = os.environ.get(name, "") or ""
    return {x.strip() for x in raw.split(",") if x.strip()}


def resolve_platform(source: str, author_id: str | int) -> Principal:
    """Map a platform user id to a Principal using the same env allowlists the
    Discord transport already uses (``DISCORD_OWNER_USER_ID`` /
    ``DISCORD_ALLOWED_USER_IDS``). Voice and platform owner ids are different
    strings but both carry ``Role.OWNER`` and collapse to mem_scope 'owner'.
    """
    aid = str(author_id)
    owner_id = (os.environ.get("DISCORD_OWNER_USER_ID", "") or "").strip()
    allowed = _csv_env("DISCORD_ALLOWED_USER_IDS")
    if owner_id and aid == owner_id:
        role = Role.OWNER
    elif aid in allowed:
        role = Role.TRUSTED
    else:
        role = Role.UNKNOWN
    return Principal(
        role=role,
        user_id=f"{source}:{aid}",
        display_name=aid,
        source=source,
        confidence=1.0,
        raw={"author_id": aid},
    )


# ── Layer B: deterministic owner-referential classifier ──────────────────────

# Personal-data nouns: a non-owner asking about any of these w.r.t. the owner
# is owner-referential. Deliberately over-broad — a false positive just yields
# a harmless deflection, while a miss is backstopped by Layer A (no tools).
_PERSONAL_NOUN = (
    r"schedule|calendar|agenda|appointment|meeting|reminder|todo|task|"
    r"location|whereabouts|address|where\s+(?:is|are|was|does)|live|living|"
    r"phone|number|email|password|passcode|pin|credential|token|"
    r"memor(?:y|ies)|profile|note|plan|plans|itinerary|routine|habit|"
    r"health|medical|finance|financial|bank|account|salary|income|"
    r"family|girlfriend|partner|relationship|contact"
)
# Third-person / owner reference markers (a non-owner referring TO the owner).
_OWNER_REF = r"\b(?:he|him|his|himself|they|them|their|that\s+guy|the\s+owner)\b"

# Cache: (owner_key, name_regex, pii_regex). Recompiled when the owner changes.
# owner_key is the owner slug, or None when no owner is enrolled — so the
# "not yet computed" sentinel must be a distinct object, not None (otherwise
# the first call with no owner enrolled would hit the empty initial cache).
_UNSET = object()
_owner_re_cache: tuple = (_UNSET, None, None)


def _owner_patterns():
    """Compile (and cache) the owner-name + personal-data matchers for the
    currently-enrolled owner. Recompiles if the owner changes."""
    global _owner_re_cache
    owner = get_owner()
    owner_key = owner["slug"] if owner else None
    if _owner_re_cache[0] == owner_key:
        return _owner_re_cache[1], _owner_re_cache[2]

    name_re = None
    if owner:
        toks: set[str] = set()
        full = (owner.get("name") or "").strip()
        if full:
            toks.add(full.lower())
            toks.update(p for p in re.split(r"\s+", full.lower()) if len(p) >= 3)
        if owner.get("slug"):
            toks.add(owner["slug"].replace("_", " ").lower())
        escaped = {re.escape(t) for t in toks if t}
        if escaped:
            name_re = re.compile(
                r"\b(?:" + "|".join(sorted(escaped)) + r")\b", re.I
            )

    # A personal-data noun within ~40 chars of a 3rd-person/owner reference.
    pii_re = re.compile(
        r"(?:" + _OWNER_REF + r").{0,40}?(?:" + _PERSONAL_NOUN + r")"
        r"|(?:" + _PERSONAL_NOUN + r").{0,40}?(?:" + _OWNER_REF + r")",
        re.I | re.S,
    )
    _owner_re_cache = (owner_key, name_re, pii_re)
    return name_re, pii_re


def is_owner_referential(text: str) -> bool:
    """True if ``text`` (from a NON-owner) is asking about the owner. The gate
    calls this for TRUSTED turns and deflects deterministically on a hit — no
    brain is ever spawned, so the model can't be talked into answering."""
    if not text:
        return False
    name_re, pii_re = _owner_patterns()
    if name_re is not None and name_re.search(text):
        return True
    if pii_re.search(text):
        return True
    return False


# ── capability tiers ─────────────────────────────────────────────────────────

def capability_for(role: Role) -> str:
    """Base capability for a role: 'full' (owner brain + all MCP tools),
    'locked' (no MCP tools at all), or 'none' (no brain). The gate layers the
    owner-referential deflection on top of 'locked' for TRUSTED turns."""
    if role is Role.OWNER:
        return "full"
    if role is Role.TRUSTED:
        return "locked"
    return "none"
