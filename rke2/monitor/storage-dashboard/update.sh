#!/bin/sh
# AppMana kubernetes-storage-dashboard-grafana — exporter + dashboard.
# Panel plugin is preinstalled via kube-prometheus-stack-values.yaml.
# Bump CHART_VERSION when upgrading; see release notes for matching
# GF_PLUGINS_PREINSTALL version in the kps values.
set -eu
CHART_VERSION="${CHART_VERSION:-0.1.1}"
helm upgrade --install kubernetes-storage-dashboard-grafana \
  oci://ghcr.io/appmana/charts/kubernetes-storage-dashboard-grafana \
  --version "$CHART_VERSION" \
  -f values.yaml \
  -n monitor
