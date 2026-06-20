# RUNBOOK — CAM++ speaker-verification upgrade (jarvis-edge)

STAGED change. Nothing below is applied until **you** run it. The running
`jarvis-stack` pod keeps using Resemblyzer (default `VOICE_MODEL` unset) and the
fail-open gate stays off (`VOICE_FAIL_OPEN` unset) until you flip the flags.

## What changed on disk (all under `rke2/ai/jarvis-edge/`)
- `jarvis_voice_id.py` — CAM++ ONNX backend (`_campplus_fbank` / `_campplus_embed`),
  `VOICE_MODEL` dispatch in `embed_from_wav`/`embed_from_audio`, cross-model guard
  in `_load_enrolled`, env-overridable thresholds, owner-passphrase primitives.
- `jarvis_identity.py` — `Principal.degraded_owner` flag + `resolve_voice` sets it;
  passphrase re-exports.
- `edge.py` — fail-OPEN degraded-owner gate in `gate_and_respond` (opt-in `VOICE_FAIL_OPEN`).
- `Dockerfile.base` — adds `torchaudio==2.2.2`, `soundfile==0.12.1`.
- `Dockerfile` — build-time CAM++ ONNX fetch → `/app/models/campplus_voxceleb.onnx`,
  copies `calibrate_voiceid.py`.
- `calibrate_voiceid.py` — new threshold-calibration script.

## CAM++ model provenance / I-O spec
- Model: wespeaker CAM++ VoxCeleb (English, 16 kHz). ModelScope mirror
  `iic/speech_campplus_sv_en_voxceleb_16k`. ~28 MB, CPU, ONNX.
- I/O: input `(1, T, 80)` float32 = CMN'd 80-dim kaldi fbank (25 ms / 10 ms,
  16 kHz, dither 0); output `(1, 192)` float32 embedding (L2-normalised by us).
- **TODO-TO-FETCH**: the Dockerfile download URL is written to spec but UNVERIFIED
  from the build env. Before first campplus build, confirm the URL resolves, or
  vendor the file into the build context as `models/campplus_voxceleb.onnx` and
  switch the Dockerfile `RUN curl` to a `COPY`. If the fetch fails the image
  still builds (resemblyzer works); campplus raises `FileNotFoundError` on first use.

## Rollout order

### (a) Build + deploy via kaniko-from-git — YOU trigger
torchaudio/soundfile live in the BASE image, so the **base must rebuild first**:
1. Rebuild `jarvis-edge-base` (whatever job/workflow builds `Dockerfile.base`)
   so `torchaudio` + `soundfile` are present.
2. Push this branch (`ai/jarvis-edge`), update the SHA tags in
   `build-job-kaniko.yaml`, then:
   `kubectl delete job -n ai jarvis-edge-build --ignore-not-found && kubectl apply -f rke2/ai/jarvis-edge/build-job-kaniko.yaml`
3. After the build pushes `zot.hwcopeland.net/ai/jarvis-edge:latest`, restart:
   `kubectl rollout restart deploy/jarvis-stack -n ai` (Recreate → brief GPU downtime).

### (b) Collect ~5 owner wake-word clips
Record ≥5 clips of "Jarvis" (plus a couple of sentences) into a pod dir, e.g.
`/state/calib/owner/*.wav`, and ≥3 other-speaker clips into `/state/calib/other/`.

### (c) Re-enroll the owner under campplus
The cross-model guard treats the existing resemblyzer owner as NOT enrolled once
`VOICE_MODEL=campplus`, so you MUST re-enroll:
```
kubectl exec -n ai -it <jarvis-edge-pod> -- \
  env VOICE_MODEL=campplus python /app/jarvis_voice_enroll.py owner owner
```
This writes `/state/voices/owner.{npy,json}` with `model=campplus-voxceleb-en-16k`.
The old resemblyzer `owner.*` stays on disk (harmless; skipped while campplus is active).

### (d) Calibrate + set thresholds
```
kubectl exec -n ai -it <jarvis-edge-pod> -- \
  env VOICE_MODEL=campplus python /app/calibrate_voiceid.py \
    --owner-dir /state/calib/owner --other-dir /state/calib/other
```
Set the printed `VOICE_THRESHOLD_*` / `VOICE_OWNER_THRESHOLD` / `VOICE_ADAPT_MIN_SCORE`
env vars on the jarvis-stack pod, plus `VOICE_MODEL=campplus`. Optionally
`VOICE_FAIL_OPEN=1` and a passphrase (see below).

### (e) Restart the daemon
`kubectl rollout restart deploy/jarvis-stack -n ai`.

### (f) A/B verify against real clips
With `VOICE_MODEL=campplus`, speak to the daemon; confirm owner turns score above
`VOICE_OWNER_THRESHOLD` in the logs and get the full brain. Flip `VOICE_MODEL` back
to `resemblyzer` (no re-enroll needed — the old owner.* is still there) to A/B.

### (g) Only then remove resemblyzer
Once campplus is trusted: drop `resemblyzer==0.1.4` from `Dockerfile.base`, remove
the resemblyzer branches in `jarvis_voice_id.py`, rebuild. Do this LAST.

## Fail-OPEN gate (`VOICE_FAIL_OPEN=1`) — behaviour change
- **Default (unset)**: identical to today. A weak owner match downgrades to
  TRUSTED and is silently dropped (the lockout).
- **Enabled**: a degraded-owner turn (top match is the owner template but below
  the OWNER grant bar, no sticky session) gets a SPOKEN passphrase challenge.
  A correct passphrase on the next turn (within `VOICE_FAILOPEN_WINDOW_S`, default
  30 s) is promoted to the full OWNER brain. With no passphrase set, a spoken
  "it's me" confirms instead (weaker — nudge: set a passphrase). It NEVER grants
  OWNER on voiceprint alone (second factor) and NEVER makes the owner worse off
  than the silent drop.
- Set a passphrase:
  `kubectl exec -n ai -it <pod> -- python -c "import jarvis_voice_id as v; print(v.set_owner_passphrase('open sesame'))"`

## Rollback
Unset `VOICE_MODEL` (→ resemblyzer) and `VOICE_FAIL_OPEN`, restart the pod. The
original resemblyzer owner voiceprint is untouched on the PVC.
