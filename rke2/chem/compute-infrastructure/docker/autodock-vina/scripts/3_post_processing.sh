#!/bin/sh
set -e

[ $# != 2 ] && echo "usage: $0 pdbid db_label" && exit 1

PDBID=$1
DB_LABEL=$2

# filter results => only keep best energy for each ligand
# How it works:
# - grep: only keep relevant lines
# - awk: only keep best (rank == 1) energy (column 4)
# - grep: only keep negative energy values
# - sort: increasingly
# - only keep the first (best) value
best_energy=$(grep -h RANKING ligand_${DB_LABEL}_*.dlg | awk '{ if ($2 == 1) print $4 }' | grep '-' | sort -n | head -n 1)

echo "Best energy: $best_energy"
