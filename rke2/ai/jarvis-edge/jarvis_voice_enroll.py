"""Interactive voice enrollment for JARVIS (cluster edge container).

Usage (from a host with kubectl context for the cluster)::

    kubectl exec -n ai -it <jarvis-edge-pod> -- \
        python /app/jarvis_voice_enroll.py [NAME] [ROLE]

If NAME is omitted, prompts. ROLE defaults to 'owner' on first enrollment,
'trusted' afterwards. Embeddings + profile scaffold are persisted to
``/state/voices/`` and ``/state/users/<slug>/`` on the jarvis-state PVC.

Captures ~36 s of audio via the pod's input device (PortAudio → ALSA →
PulseAudio plugin → host pipewire-pulse socket — same path the daemon
uses) while you read aloud passages chosen to cover pitch / vowel range,
computes Resemblyzer embeddings, averages, and stores under
``/state/voices/``.

Run this interactively — needs a real TTY (``-it``) and live mic access.
"""
from __future__ import annotations

import os
import sys
import time

import numpy as np
import sounddevice as sd

import jarvis_voice_id as vid

# Three short passages — diverse phonetics + pitch range.
PASSAGES = [
    "The quick brown fox jumps over the lazy dog. "
    "She sells seashells down by the seashore. "
    "How much wood would a woodchuck chuck if a woodchuck could chuck wood.",

    "Bright vivid mornings spill golden light through tall kitchen windows. "
    "The kettle whistles, the coffee grinder hums, and the dog stretches. "
    "By noon the storm clouds gather over the western ridge.",

    "Numbers, names, and ordinary words: one two three four five. "
    "Forty seven dollars and sixty two cents. "
    "My name is — pause — and I authorize JARVIS to recognize my voice.",
]

CAPTURE_SECS_PER_PASSAGE = 12  # 3 × 12 = 36 s total — well above Resemblyzer's window
SAMPLE_RATE = 16000  # Resemblyzer's native rate; sounddevice will accept this


def _capture(seconds: float, prompt: str) -> np.ndarray:
    print()
    print("=" * 68)
    print(prompt)
    print("=" * 68)
    for n in range(3, 0, -1):
        print(f"  recording in {n}…", end="\r", flush=True)
        time.sleep(1)
    print("  ●  RECORDING — speak now            ")
    audio = sd.rec(int(seconds * SAMPLE_RATE), samplerate=SAMPLE_RATE,
                   channels=1, dtype="float32")
    sd.wait()
    rms = float(np.sqrt(np.mean(audio ** 2)))
    print(f"  ◾  done  ({seconds:.0f}s, rms={rms:.3f})")
    if rms < 0.01:
        print("  ⚠  audio level very low — check mic input")
    return audio.flatten()


def main() -> None:
    args = sys.argv[1:]
    has_owner = vid.has_owner()
    default_role = "trusted" if has_owner else "owner"

    if args:
        name = args[0]
    else:
        name = input(f"Name to enroll [default role={default_role}]: ").strip()
    if not name:
        print("aborted: empty name", file=sys.stderr)
        sys.exit(2)

    role = (args[1] if len(args) > 1 else default_role).lower()
    if role not in ("owner", "trusted"):
        print(f"role must be owner|trusted, got {role!r}", file=sys.stderr)
        sys.exit(2)
    if role == "owner" and has_owner:
        existing = vid.get_owner()
        print(f"warning: owner is already enrolled ({existing['name']}). "
              f"This will replace them.")
        ans = input("Continue? [y/N] ").strip().lower()
        if ans != "y":
            print("aborted")
            sys.exit(0)

    info = sd.query_devices(kind="input")
    dev_name = info["name"] if isinstance(info, dict) else "<unknown>"
    print(f"\nEnrolling: {name}  (role={role})")
    print(f"Mic:       {dev_name}  @ {SAMPLE_RATE} Hz target")
    print("You'll read 3 short passages (~12s each). Talk normally — your "
          "usual pace + volume, like you'd address JARVIS.\n")

    embeddings = []
    audio_clips = []
    for i, passage in enumerate(PASSAGES, start=1):
        clip = _capture(CAPTURE_SECS_PER_PASSAGE,
                        f"Passage {i}/{len(PASSAGES)} — read this aloud:\n\n  {passage}")
        audio_clips.append(clip)
        try:
            emb = vid.embed_from_audio(clip, sample_rate=SAMPLE_RATE)
        except Exception as exc:  # noqa: BLE001
            print(f"  ✗ embed failed: {exc}")
            continue
        embeddings.append(emb)
        print(f"  ✓ embedded  (norm={np.linalg.norm(emb):.3f})")

    if len(embeddings) < 2:
        print("\nfailed: needed at least 2 successful embeddings", file=sys.stderr)
        sys.exit(1)

    # Inter-clip similarity sanity check (your own voice vs your own voice)
    if len(embeddings) >= 2:
        a, b = embeddings[0], embeddings[1]
        a_n = a / (np.linalg.norm(a) + 1e-9)
        b_n = b / (np.linalg.norm(b) + 1e-9)
        sim = float(np.dot(a_n, b_n))
        print(f"\nself-similarity across your own clips: {sim:.3f}")
        if sim < 0.7:
            print("  ⚠  low self-similarity — your voice is being captured "
                  "inconsistently. Background noise / mic distance / mic "
                  "choice can cause this. Enrollment will proceed but "
                  "identification may be flaky.")
        else:
            print("  ✓ good — JARVIS should reliably recognize you")

    meta = vid.enroll(name, embeddings=embeddings, role=role,
                      enrolled_by=name if role == "owner" else "")
    print(f"\n✓ enrolled  slug={meta['slug']}  clips={meta['num_clips']}  "
          f"role={meta['role']}")
    print(f"  voice files: /state/voices/{meta['slug']}.{{npy,json}}")
    print(f"  profile:     /state/users/{meta['slug']}/profile.md")
    print("\nEdit the profile to give JARVIS context about this person "
          "(location, preferences, etc.).")


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print("\naborted")
        sys.exit(130)
