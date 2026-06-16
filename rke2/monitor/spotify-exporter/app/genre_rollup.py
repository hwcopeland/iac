"""Load the versioned genre_rollup.yaml without pulling in a YAML dependency.

The file is a deliberately flat structure:

    version: N
    mappings:
      "raw tag": ParentBucket
      ...

so a tiny line parser is enough and keeps the image dep-free. If the schema of
that file ever grows nested structures, swap this for PyYAML.
"""
from __future__ import annotations

import os
from typing import Tuple

_PATH = os.path.join(os.path.dirname(os.path.abspath(__file__)), "genre_rollup.yaml")


def load(path: str = _PATH) -> Tuple[int, dict[str, str]]:
    """Return (version, {raw_genre_lower: parent_genre})."""
    version = 0
    mappings: dict[str, str] = {}
    in_mappings = False
    try:
        with open(path) as f:
            lines = f.readlines()
    except OSError as exc:
        print(f"genre_rollup: cannot read {path}: {exc}", flush=True)
        return version, mappings

    for raw_line in lines:
        line = raw_line.rstrip("\n")
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        if stripped.startswith("version:"):
            try:
                version = int(stripped.split(":", 1)[1].strip())
            except ValueError:
                version = 0
            continue
        if stripped == "mappings:":
            in_mappings = True
            continue
        if not in_mappings:
            continue
        # Only treat indented "key: value" lines as mappings.
        if ":" not in stripped:
            continue
        key, _, value = stripped.partition(":")
        key = key.strip().strip('"').strip("'").lower()
        # Strip inline comments from the value, then quotes/space.
        value = value.split("#", 1)[0].strip().strip('"').strip("'")
        if key and value:
            mappings[key] = value
    return version, mappings


def parent_of(raw_genre: str, mappings: dict[str, str]) -> str:
    """Resolve a raw Spotify tag to its parent bucket, else 'Other'."""
    if not raw_genre:
        return "Other"
    return mappings.get(raw_genre.lower(), "Other")
