#!/bin/sh
set -e

[ $# != 2 ] && echo "usage: $0 pdbid db_label" && exit 1

PDBID=$1
DB_LABEL=$2

# Parse AutoDock Vina .log files — extract mode-1 affinity (kcal/mol) from each
# docked ligand result, then report the overall best (most negative) value.
#
# Vina log format (mode-1 line starts with spaces + "1 "):
#   mode | affinity (kcal/mol) | ...
#      1       -7.1       0.000    0.000
best_energy=$(
  find . -path "*${DB_LABEL}*/docked/*.log" 2>/dev/null \
    | xargs grep -h "^   1 " 2>/dev/null \
    | awk '{print $2}' \
    | grep '^-' \
    | sort -n \
    | head -n 1
)

echo "Best energy: $best_energy"
