#!/usr/bin/env python3
"""Generate mock AutoDock Vina output log files for testing.

Creates the directory structure produced by the docking pipeline:
  {output_dir}/{db_label}_batch{N}/docked/ligand_{M}.log
"""

import os
import random
import argparse

_VINA_LOG_TEMPLATE = """\
#################################################################
# If you used AutoDock Vina in your work, please cite:          #
#                                                               #
# O. Trott, A. J. Olson,                                        #
# AutoDock Vina: improving the speed and accuracy of docking    #
# with a new scoring function, efficient optimization, and      #
# multithreading, Journal of Computational Chemistry 31 (2010)  #
# 455-461                                                       #
#################################################################

Detected 4 CPUs
Reading input ... done.
Setting up the scoring function ... done.
Analyzing the binding site ... done.
Using random seed: {seed}
Performing search ... done.
Refining results ... done.

mode |   affinity | dist from best mode
     | (kcal/mol) | rmsd l.b.| rmsd u.b.
-----+------------+----------+----------
   1      {aff1:>6.1f}      0.000      0.000
   2      {aff2:>6.1f}      1.234      2.345
   3      {aff3:>6.1f}      2.101      3.567
"""


def generate_log(affinity: float) -> str:
    return _VINA_LOG_TEMPLATE.format(
        seed=random.randint(1_000_000, 9_999_999),
        aff1=affinity,
        aff2=round(affinity + random.uniform(0.2, 0.8), 1),
        aff3=round(affinity + random.uniform(0.9, 1.6), 1),
    )


def main():
    parser = argparse.ArgumentParser(description="Generate mock Vina output logs for testing.")
    parser.add_argument("--output-dir", required=True, help="Root directory for generated data.")
    parser.add_argument("--count", type=int, default=1000, help="Total number of ligands.")
    parser.add_argument("--batches", type=int, default=5, help="Number of batches to split into.")
    parser.add_argument("--db-label", default="ligands", help="DB label prefix for batch dirs.")
    args = parser.parse_args()

    os.makedirs(args.output_dir, exist_ok=True)

    per_batch, remainder = divmod(args.count, args.batches)
    ligand_idx = 1

    for batch_num in range(args.batches):
        batch_size = per_batch + (1 if batch_num < remainder else 0)
        docked_dir = os.path.join(
            args.output_dir,
            f"{args.db_label}_batch{batch_num}",
            "docked",
        )
        os.makedirs(docked_dir, exist_ok=True)

        for _ in range(batch_size):
            affinity = round(random.uniform(-12.0, -3.0), 1)
            log_path = os.path.join(docked_dir, f"ligand_{ligand_idx}.log")
            with open(log_path, "w") as f:
                f.write(generate_log(affinity))
            ligand_idx += 1

    total = ligand_idx - 1
    print(f"Generated {total} mock log file(s) across {args.batches} batch(es) in {args.output_dir}")
    return total


if __name__ == "__main__":
    main()
