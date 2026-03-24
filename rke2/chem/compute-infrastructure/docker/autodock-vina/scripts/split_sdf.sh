#!/bin/sh
set -e

# split_sdf: (n:int, db_label:str) -> n_batches: int

[ $# != 2 ] && echo "usage: $0 n db_label" && exit 1

n=$1        # batch size
db_label=$2 # db_label

fname=$db_label.sdf

# We split the .sdf file into several label_labelXXX.sdf files
# the split happens every Nth "$$$$" separator.
# Each output file name is printed to stdout
awk -v n=$n -v db_label=$db_label '
BEGIN { i = n; j = -1; }
/\$\$\$\$/ { i++; }
(i == n) { 
    j++; i = 0;
    
    # print the filename to stdout
    out = db_label "_batch" j ".sdf";

    if(j != 0) next;
}
{ print > out }
END { print j } 
' $db_label.sdf 
