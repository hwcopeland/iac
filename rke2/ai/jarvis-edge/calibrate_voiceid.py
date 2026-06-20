"""Threshold calibration for JARVIS speaker verification (CAM++ / Resemblyzer).

The shipped _THRESHOLD_MATCH / _THRESHOLD_BORDER / OWNER_THRESHOLD defaults are
RESEMBLYZER-tuned and do NOT carry over to CAM++ (different cosine score
distributions). This script measures the actual distributions on YOUR clips and
recommends model-appropriate thresholds.

Usage (run INSIDE the jarvis-edge pod so it uses the same embedding backend +
bundled ONNX as the daemon)::

    kubectl exec -n ai -it <jarvis-edge-pod> -- \
        env VOICE_MODEL=campplus \
        python /app/calibrate_voiceid.py \
            --owner-dir /state/calib/owner \
            --other-dir /state/calib/other

Inputs: two directories of WAV clips.
  --owner-dir : clips of the OWNER (≥5; the wake word "Jarvis" + a few sentences)
  --other-dir : clips of OTHER speakers (the more distinct voices the better)

It builds an owner reference template from a held-out split of the owner clips,
then scores the remaining owner clips (positives) and all other-speaker clips
(negatives) against it — mirroring exactly how identify() scores live turns
(cosine of L2-normalised embeddings). It prints:
  * positive / negative cosine distributions (min / mean / max / percentiles)
  * the Equal Error Rate (EER) and its threshold
  * a BALANCED operating point (the EER threshold) → _THRESHOLD_BORDER
  * a HIGH-PRECISION owner point (near-zero false accepts) → OWNER_THRESHOLD
  * a recommended _THRESHOLD_MATCH between the two

Honours VOICE_MODEL — run it with the SAME VOICE_MODEL you'll deploy so the
numbers match the live gate.
"""
from __future__ import annotations

import argparse
import sys
from pathlib import Path

import numpy as np

import jarvis_voice_id as vid


def _embed_file(path: Path) -> np.ndarray | None:
    try:
        emb = vid.embed_from_wav(path)
    except Exception as exc:  # noqa: BLE001
        print(f"  ! skip {path.name}: {exc}", file=sys.stderr)
        return None
    emb = np.asarray(emb, dtype=np.float32)
    emb /= (np.linalg.norm(emb) + 1e-9)
    return emb


def _load_dir(d: Path) -> list[np.ndarray]:
    out = []
    for p in sorted(d.glob("*.wav")) + sorted(d.glob("*.WAV")):
        e = _embed_file(p)
        if e is not None:
            out.append(e)
    return out


def _cos(a: np.ndarray, b: np.ndarray) -> float:
    return float(np.dot(a, b))  # both already L2-normalised


def _dist(name: str, xs: np.ndarray) -> None:
    if xs.size == 0:
        print(f"  {name}: (none)")
        return
    pct = np.percentile(xs, [5, 25, 50, 75, 95])
    print(f"  {name}: n={xs.size}  min={xs.min():.3f}  mean={xs.mean():.3f}  "
          f"max={xs.max():.3f}")
    print(f"       p05={pct[0]:.3f} p25={pct[1]:.3f} p50={pct[2]:.3f} "
          f"p75={pct[3]:.3f} p95={pct[4]:.3f}")


def _eer(pos: np.ndarray, neg: np.ndarray) -> tuple[float, float]:
    """Sweep thresholds; return (eer, threshold_at_eer)."""
    if pos.size == 0 or neg.size == 0:
        return float("nan"), float("nan")
    grid = np.unique(np.concatenate([pos, neg]))
    best_t, best_gap, best_eer = grid[0], 1e9, 1.0
    for t in grid:
        far = float((neg >= t).mean())   # false accept rate
        frr = float((pos < t).mean())    # false reject rate
        gap = abs(far - frr)
        if gap < best_gap:
            best_gap, best_eer, best_t = gap, (far + frr) / 2.0, float(t)
    return best_eer, best_t


def _high_precision_threshold(neg: np.ndarray, margin: float = 0.02) -> float:
    """Lowest threshold that rejects ALL negatives, plus a small margin → an
    owner-grant bar with ~zero false accepts on the measured impostor set."""
    if neg.size == 0:
        return float("nan")
    return float(neg.max() + margin)


def main() -> None:
    ap = argparse.ArgumentParser(description="Calibrate voice-id thresholds")
    ap.add_argument("--owner-dir", required=True, type=Path)
    ap.add_argument("--other-dir", required=True, type=Path)
    ap.add_argument("--ref-split", type=int, default=2,
                    help="how many owner clips to average into the reference "
                         "template (rest become positives). Default 2.")
    args = ap.parse_args()

    print(f"VOICE_MODEL = {vid._VOICE_MODEL}  (_MODEL_NAME={vid._MODEL_NAME}, "
          f"dim={vid._EMBED_DIM})\n")

    owner = _load_dir(args.owner_dir)
    other = _load_dir(args.other_dir)
    if len(owner) < args.ref_split + 1:
        print(f"need ≥{args.ref_split + 1} owner clips, got {len(owner)}",
              file=sys.stderr)
        sys.exit(1)
    if not other:
        print("need ≥1 other-speaker clip", file=sys.stderr)
        sys.exit(1)

    # Build the reference template from the first ref-split owner clips
    # (same averaging + normalisation enroll() does), score the rest as
    # positives. This mirrors enroll() → identify() on held-out owner turns.
    ref = np.mean(np.stack(owner[:args.ref_split]), axis=0)
    ref /= (np.linalg.norm(ref) + 1e-9)

    pos = np.array([_cos(e, ref) for e in owner[args.ref_split:]], dtype=np.float32)
    neg = np.array([_cos(e, ref) for e in other], dtype=np.float32)

    print("Cosine score distributions (vs owner reference template):")
    _dist("owner (positives)", pos)
    _dist("other (negatives)", neg)
    print()

    eer, eer_t = _eer(pos, neg)
    hp_t = _high_precision_threshold(neg)
    print(f"Equal Error Rate (EER): {eer * 100:.2f}%  at threshold {eer_t:.3f}")
    print(f"Zero-false-accept owner bar (max negative + 0.02): {hp_t:.3f}\n")

    # Recommendations.
    border = round(max(0.0, eer_t - 0.05), 2)       # permissive "worth retrying"
    match = round(eer_t, 2)                          # balanced identification
    owner_bar = round(max(match + 0.05, hp_t), 2)    # strict owner grant
    print("Recommended env overrides (set on the jarvis-stack pod):")
    print(f"  VOICE_THRESHOLD_BORDER={border}")
    print(f"  VOICE_THRESHOLD_MATCH={match}")
    print(f"  VOICE_OWNER_THRESHOLD={owner_bar}")
    print(f"  # adapt bar should sit comfortably above the owner bar:")
    print(f"  VOICE_ADAPT_MIN_SCORE={round(owner_bar + 0.05, 2)}")
    print("\nTwo operating points to choose between:")
    print(f"  BALANCED      → OWNER_THRESHOLD={match}  (more owner accepts, "
          "small impostor risk)")
    print(f"  HIGH-PRECISION→ OWNER_THRESHOLD={owner_bar}  (≈zero false accepts "
          "on this impostor set, more owner re-challenges)")


if __name__ == "__main__":
    main()
