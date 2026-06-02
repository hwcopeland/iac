# flockmap — shared multi-stage build for every cmd/<name> binary.
#
# One Dockerfile, parameterised by the CMD build-arg, builds all five pipeline
# images (camera-spine, geocode, agency-resolve, funding-federal, match). The
# build context is the flockmap module root (rke2/flockmap) so the whole module
# — go.mod, go.sum, internal/store, db/, and every cmd/ — is present for the
# `go build ./cmd/${CMD}` step.
#
#   docker build --build-arg CMD=camera-spine -t .../flockmap/camera-spine .
#
# Build on the self-hosted arc-chem ARC runner only (no Mac builds). Pushed with
# provenance:false sbom:false for zot compatibility (see build-flockmap.yml).

# --- build stage -----------------------------------------------------------
FROM golang:1.22-bookworm AS build

# Which cmd/<name> to compile. Set per-matrix-entry by the workflow.
ARG CMD
ENV CGO_ENABLED=0 GOFLAGS=-mod=mod

WORKDIR /src

# Module graph first for layer caching; only stdlib + lib/pq is required and it
# is already vendored in go.sum, so this is a no-network resolve.
COPY go.mod go.sum ./
RUN go mod download

# Then the full source tree (internal/, db/, cmd/).
COPY . .

# Fail loudly if the matrix passed a cmd that does not exist in this build.
RUN test -d "./cmd/${CMD}" || (echo "no such cmd: ./cmd/${CMD}" >&2 && exit 1)

# Static binary, stripped, deterministic output path so the run stage is generic.
RUN go build -trimpath -ldflags='-s -w' -o /out/flockmap-cmd "./cmd/${CMD}"

# --- runtime stage ---------------------------------------------------------
# Distroless static: no shell, no package manager, runs as nonroot. The binaries
# only need outbound HTTPS (Overpass / USAspending / Census) + Postgres TCP, all
# satisfied by the static base's CA bundle.
FROM gcr.io/distroless/static-debian12:nonroot AS runtime

WORKDIR /
COPY --from=build /out/flockmap-cmd /usr/local/bin/flockmap-cmd

USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/flockmap-cmd"]
