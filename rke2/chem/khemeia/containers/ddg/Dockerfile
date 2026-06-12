# ddG-of-folding stability worker (CPU) for the Khemeia genomics layer.
# Registered in .github/workflows/build-chem-images.yml's detect-changes matrix (key: ddg).
#
# Given a resolved variant (WT structure PDB + point substitution), predicts the
# change in folding free energy ΔΔG_fold (kcal/mol; sign convention: POSITIVE =
# destabilizing). Worker for spec.calculation=ddg_stability (workers TDD §5).
#
# ENGINE / LICENSE BOUNDARY (workers TDD §5.1, plan R-W2 — a HARD constraint
# because the platform now serves an EXTERNAL consumer, u4u):
#   * DEFAULT engine = ThermoMPNN (MIT). ThermoMPNN is a small transfer-learning
#     head over a ProteinMPNN backbone; BOTH checkpoints are MIT-licensed and
#     small (tens of MB total) so they are BAKED INTO THE IMAGE (see below) —
#     no PVC, unlike the multi-GB ESMFold weights.
#   * ddgun (GPL-3.0) is a second OPEN fallback engine, also shippable.
#   * foldx / rosetta are ACADEMIC-ONLY / NON-REDISTRIBUTABLE. They are NEVER
#     baked into this image. They are supported ONLY as thin adapters that shell
#     out to a binary the operator mounts OUT-OF-BAND at $FOLDX_BIN / $ROSETTA_BIN.
#     If params.engine=foldx|rosetta and the binary is absent -> E_PARAMS_INVALID.
#     We redistribute ONLY the open engines.
#
# CPU only — no CUDA base. ThermoMPNN is sub-second on CPU.
#
# Build on the cluster ONLY (Kaniko via Flux image-automation, image-repository-
# ddg.yaml / image-policy-ddg.yaml). NEVER on the Mac (fills the laptop disk).

# ── Stage 1: builder ─────────────────────────────────────────────────────────
FROM zot.hwcopeland.net/library/ubuntu:22.04 AS builder

ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=Etc/UTC
ENV PYTHONUNBUFFERED=1

RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-pip python3-dev \
    gcc g++ make \
    git ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# numpy<2 for ABI parity with the torch CPU wheel + biotite stack (mirrors the
# esmfold worker's pin).
RUN pip3 install --no-cache-dir "numpy<2"

# PyTorch CPU wheel — ThermoMPNN is a small GNN; CPU inference is sub-second.
# Pinned + CPU index so no CUDA libs are pulled (keeps the image small).
RUN pip3 install --no-cache-dir \
    torch==2.1.2+cpu \
    --extra-index-url https://download.pytorch.org/whl/cpu

COPY requirements.txt ./
RUN pip3 install --no-cache-dir -r requirements.txt

# ── Engine checkpoints (BAKED IN — both MIT, both small) ─────────────────────
# ThermoMPNN (Kuhlman Lab, MIT: github.com/Kuhlman-Lab/ThermoMPNN) is transfer-
# learning on top of ProteinMPNN (Dauparas et al., MIT: github.com/dauparas/
# ProteinMPNN). The worker needs (a) the ProteinMPNN backbone checkpoint and
# (b) the ThermoMPNN trained head.
#
#   * ProteinMPNN backbone: v_48_020.pt (~few MB), MIT-licensed, pulled from the
#     pinned ProteinMPNN commit. NOT license-restricted -> safe to bake.
#   * ThermoMPNN head + inference code: pinned commit, MIT-licensed.
#
# Both are fetched at BUILD time from their pinned MIT sources and copied into
# the runtime image, so a job needs no network for the default engine. This is
# the deliberate counterpoint to ESMFold's PVC: ESMFold weights are GBs (PVC);
# ThermoMPNN's are tens of MB (bake-in is cheaper than PVC plumbing). Documented
# in the GEN-30 report.
ENV THERMOMPNN_DIR=/opt/ddg/engines/thermompnn
ENV PROTEINMPNN_DIR=/opt/ddg/engines/proteinmpnn
# SUPPLY-CHAIN PIN (VULN-5): every external source is pinned to an immutable
# commit SHA, NOT a branch HEAD. Because the model weight checkpoints
# (v_48_020.pt, ThermoMPNN head) are BAKED INTO THE IMAGE straight out of these
# repos, an unpinned HEAD would let upstream silently change the shipped weights
# between builds. SHAs below were the default-branch HEAD on 2026-06-12; bump
# deliberately + re-review when updating.
#   ProteinMPNN  dauparas/ProteinMPNN      main @ 8907e6671bfbfc92303b5f79c4b5e6ce47cdef57
#   ThermoMPNN   Kuhlman-Lab/ThermoMPNN    main @ 2b04fd370e399911b1fa5848112cc9013f084110
ENV PROTEINMPNN_SHA=8907e6671bfbfc92303b5f79c4b5e6ce47cdef57
ENV THERMOMPNN_SHA=2b04fd370e399911b1fa5848112cc9013f084110
RUN mkdir -p "${THERMOMPNN_DIR}" "${PROTEINMPNN_DIR}" && \
    git clone https://github.com/dauparas/ProteinMPNN.git "${PROTEINMPNN_DIR}/src" && \
    git -C "${PROTEINMPNN_DIR}/src" checkout "${PROTEINMPNN_SHA}" && \
    git clone https://github.com/Kuhlman-Lab/ThermoMPNN.git "${THERMOMPNN_DIR}/src" && \
    git -C "${THERMOMPNN_DIR}/src" checkout "${THERMOMPNN_SHA}"
# NOTE: the ProteinMPNN backbone checkpoint v_48_020.pt ships inside the
# ProteinMPNN repo (vanilla_model_weights/). The ThermoMPNN trained head ships
# in the ThermoMPNN repo. Both are now pinned via the SHAs above, so the baked
# weights are reproducible. No external weight server is contacted at runtime.
# CAVEAT: these checkpoints live INSIDE the pinned git repos, so the commit SHA
# is their integrity anchor — there is no separately published sha256 digest for
# v_48_020.pt / the ThermoMPNN head to verify against, so no standalone checksum
# step is added (none is determinable). The git SHA pin is the provenance guard.
# DDGun (GPL-3.0) — second open fallback engine, shippable. Pinned to the
# default-branch HEAD on 2026-06-12 (was an unpinned --depth 1 clone of HEAD).
#   ddgun  biofold/ddgun  master @ bcf112c616bcd4eb9b18f193ec08cc704eb761b5
ENV DDGUN_SHA=bcf112c616bcd4eb9b18f193ec08cc704eb761b5
RUN { git clone https://github.com/biofold/ddgun.git "/opt/ddg/engines/ddgun/src" && \
      git -C "/opt/ddg/engines/ddgun/src" checkout "${DDGUN_SHA}"; } || true

# ── Stage 2: runtime ─────────────────────────────────────────────────────────
FROM zot.hwcopeland.net/library/ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=Etc/UTC
ENV PYTHONUNBUFFERED=1

# Engine checkpoint locations the worker reads (defaults; pod spec may re-assert).
ENV THERMOMPNN_DIR=/opt/ddg/engines/thermompnn
ENV PROTEINMPNN_DIR=/opt/ddg/engines/proteinmpnn
ENV DDGUN_DIR=/opt/ddg/engines/ddgun
# Out-of-band, operator-mounted, NON-REDISTRIBUTABLE binaries. Empty by default;
# only populated when an operator mounts a licensed binary on the node.
ENV FOLDX_BIN=/opt/foldx/foldx
ENV ROSETTA_BIN=/opt/rosetta/cartesian_ddg

RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-pip \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy the pip env + the baked-in MIT engine checkpoints from the builder.
COPY --from=builder /usr/local/lib/python3.10 /usr/local/lib/python3.10
COPY --from=builder /usr/local/bin /usr/local/bin
COPY --from=builder /opt/ddg/engines /opt/ddg/engines

WORKDIR /opt/ddg
COPY scripts/ scripts/

CMD ["python3", "scripts/ddg_run.py"]
