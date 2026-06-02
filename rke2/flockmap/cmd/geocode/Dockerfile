# flockmap geocode — Go binary + GDAL (ogr2ogr) for TIGER shapefile load.
#
# Two stages: build the static-ish Go binary, then drop it onto a GDAL image so
# ogr2ogr (which the program shells out to) is on PATH. Build context is the
# repo root (rke2/flockmap) so go.mod, internal/, db/ are available to the build.
#
# Build (on the arc-chem ARC runner — NEVER on the Mac):
#   docker build -f cmd/geocode/Dockerfile -t zot.hwcopeland.net/flockmap/geocode:latest .

# ---- build stage ----
FROM golang:1.22-bookworm AS build
WORKDIR /src

# Module graph first for layer caching.
COPY go.mod go.sum ./
RUN go mod download

# Source. internal/store embeds db/*.sql copies via schema_embed/, so the whole
# tree is needed.
COPY . .

ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build -trimpath -ldflags="-s -w" -o /out/geocode ./cmd/geocode

# ---- runtime stage ----
# osgeo/gdal ships ogr2ogr with the PostgreSQL driver. ubuntu-small is amd64.
FROM ghcr.io/osgeo/gdal:ubuntu-small-3.8.5 AS runtime

# ca-certificates for HTTPS to www2.census.gov.
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates \
 && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/geocode /usr/local/bin/geocode
# Ship the standalone SQL + spot-check docs for audit/manual runs.
COPY cmd/geocode/sql/ /opt/flockmap/geocode/sql/

ENV TIGER_WORKDIR=/tmp/tiger
ENTRYPOINT ["/usr/local/bin/geocode"]
