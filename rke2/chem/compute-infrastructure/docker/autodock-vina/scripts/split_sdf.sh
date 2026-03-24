#!/bin/sh
set -e

# split_sdf: (n:int, db_label:str) -> n_batches: int

[ $# != 2 ] && echo "usage: $0 n db_label" && exit 1

n=$1        # batch size (molecules per batch)
db_label=$2 # db_label

# Split the .sdf file into {db_label}_batch0.sdf, _batch1.sdf, etc.
# Each SDF molecule terminates with "$$$$". We close each batch after n
# molecules, keeping the "$$$$" terminator in the current batch before
# switching — so every output file is valid SDF.
# Prints the number of non-empty output batches to stdout.
awk -v n="$n" -v db_label="$db_label" '
BEGIN { count = 0; batch = 0; out = db_label "_batch0.sdf" }
{
    print > out
    if (/\$\$\$\$/) {
        count++
        if (count == n) {
            count = 0
            batch++
            out = db_label "_batch" batch ".sdf"
        }
    }
}
END { print (count > 0) ? batch + 1 : batch }
' "$db_label.sdf"
