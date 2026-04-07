#!/usr/bin/env python3
"""Standalone DFRATOM input file generator for Khemeia plugin system.

Generates input files from atom type, charge, and basis set parameters.
No external dependencies — runs with just Python 3 stdlib.

Usage:
  python3 input_gen.py C 0 UGBS
  python3 input_gen.py Sn -2
"""

import sys

ELECTRON_CONFIGS = {
    ('C', 0): '[He] 2s(2) 2p(2)',
    ('C', -1): '[He] 2s(2) 2p(3)',
    ('C', -2): '[He] 2s(2) 2p(4)',
    ('C', -3): '[He] 2s(2) 2p(5)',
    ('C', -4): '[He] 2s(2) 2p(6)',
    ('N', 0): '[He] 2s(2) 2p(3)',
    ('N', -1): '[He] 2s(2) 2p(4)',
    ('N', -2): '[He] 2s(2) 2p(5)',
    ('N', -3): '[He] 2s(2) 2p(6)',
    ('O', 0): '[He] 2s(2) 2p(4)',
    ('O', -1): '[He] 2s(2) 2p(5)',
    ('O', -2): '[He] 2s(2) 2p(6)',
    ('Sn', 0): '[Kr] 4d(10) 5s(2) 5p(2)',
    ('Sn', -1): '[Kr] 4d(10) 5s(2) 5p(3)',
    ('Sn', -2): '[Kr] 4d(10) 5s(2) 5p(4)',
    ('Sn', -3): '[Kr] 4d(10) 5s(2) 5p(5)',
    ('Sn', -4): '[Kr] 4d(10) 5s(2) 5p(6)',
    ('Te', 0): '[Kr] 4d(10) 5s(2) 5p(4)',
    ('Te', -1): '[Kr] 4d(10) 5s(2) 5p(5)',
    ('Te', -2): '[Kr] 4d(10) 5s(2) 5p(6)',
    ('Sb', 0): '[Kr] 4d(10) 5s(2) 5p(3)',
    ('Sb', -1): '[Kr] 4d(10) 5s(2) 5p(4)',
    ('Sb', -2): '[Kr] 4d(10) 5s(2) 5p(5)',
    ('Sb', -3): '[Kr] 4d(10) 5s(2) 5p(6)',
    ('Po', 0): '[Xe] 4f(14) 5d(10) 6s(2) 6p(4)',
    ('Po', -1): '[Xe] 4f(14) 5d(10) 6s(2) 6p(5)',
    ('Po', -2): '[Xe] 4f(14) 5d(10) 6s(2) 6p(6)',
    ('At', 0): '[Xe] 4f(14) 5d(10) 6s(2) 6p(5)',
    ('At', -1): '[Xe] 4f(14) 5d(10) 6s(2) 6p(6)',
    ('I', 0): '[Kr] 4d(10) 5s(2) 5p(5)',
    ('I', -1): '[Kr] 4d(10) 5s(2) 5p(6)',
    ('Hg', 0): '[Xe] 4f(14) 5d(10) 6s(2)',
    ('Pb', 0): '[Xe] 4f(14) 5d(10) 6s(2) 6p(2)',
    ('Pb', -1): '[Xe] 4f(14) 5d(10) 6s(2) 6p(3)',
    ('Pb', -2): '[Xe] 4f(14) 5d(10) 6s(2) 6p(4)',
    ('Bi', 0): '[Xe] 4f(14) 5d(10) 6s(2) 6p(3)',
    ('Bi', -1): '[Xe] 4f(14) 5d(10) 6s(2) 6p(4)',
    ('Bi', -2): '[Xe] 4f(14) 5d(10) 6s(2) 6p(5)',
}

ATOMIC_NUMBERS = {
    'H': 1,  'He': 2,  'Li': 3,  'Be': 4,  'B': 5,   'C': 6,   'N': 7,   'O': 8,   'F': 9,   'Ne': 10,
    'Na': 11, 'Mg': 12,'Al': 13, 'Si': 14, 'P': 15,  'S': 16,  'Cl': 17, 'Ar': 18, 'K': 19,  'Ca': 20,
    'Sc': 21, 'Ti': 22,'V': 23,  'Cr': 24, 'Mn': 25, 'Fe': 26, 'Co': 27, 'Ni': 28, 'Cu': 29, 'Zn': 30,
    'Ga': 31, 'Ge': 32,'As': 33, 'Se': 34, 'Br': 35, 'Kr': 36, 'Rb': 37, 'Sr': 38, 'Y': 39,  'Zr': 40,
    'Nb': 41, 'Mo': 42,'Tc': 43, 'Ru': 44, 'Rh': 45, 'Pd': 46, 'Ag': 47, 'Cd': 48, 'In': 49, 'Sn': 50,
    'Sb': 51, 'Te': 52,'I': 53,  'Xe': 54, 'Cs': 55, 'Ba': 56, 'La': 57, 'Ce': 58, 'Pr': 59, 'Nd': 60,
    'Pm': 61, 'Sm': 62,'Eu': 63, 'Gd': 64, 'Tb': 65, 'Dy': 66, 'Ho': 67, 'Er': 68, 'Tm': 69, 'Yb': 70,
    'Lu': 71, 'Hf': 72,'Ta': 73, 'W': 74,  'Re': 75, 'Os': 76, 'Ir': 77, 'Pt': 78, 'Au': 79, 'Hg': 80,
    'Tl': 81, 'Pb': 82,'Bi': 83, 'Po': 84, 'At': 85, 'Rn': 86, 'Fr': 87, 'Ra': 88, 'Ac': 89, 'Th': 90,
    'Pa': 91, 'U': 92, 'Np': 93, 'Pu': 94, 'Am': 95, 'Cm': 96, 'Bk': 97, 'Cf': 98, 'Es': 99, 'Fm': 100,
    'Md': 101,'No': 102,'Lr': 103,'Rf': 104,'Db': 105,'Sg': 106,'Bh': 107,'Hs': 108,'Mt': 109,'Ds': 110,
    'Rg': 111,'Cn': 112,'Nh': 113,'Fl': 114,'Mc': 115,'Lv': 116,'Ts': 117,'Og': 118,
}


def generate_input(atom: str, charge: int, basis: str = "UGBS") -> str:
    z = ATOMIC_NUMBERS.get(atom)
    if z is None:
        print(f"ERROR: Unknown element '{atom}'", file=sys.stderr)
        sys.exit(1)
    config = ELECTRON_CONFIGS.get((atom, charge), f"See atomic data for {atom}")
    return f"""Atom {atom} , charge =   {charge:+3d}, isoelectronic with {atom}
     LS configuration = {config}

NUCLEAR CHARGE {z:2d}.00000

NUCLEUS MODEL: FINITE SPHERE NUCLEUS

SPEED OF LIGHT    0.137035999177D+03

BASIS SET: {basis}
"""


if __name__ == '__main__':
    if len(sys.argv) < 3:
        print("Usage: python3 input_gen.py ATOM CHARGE [BASIS]", file=sys.stderr)
        sys.exit(1)
    atom = sys.argv[1]
    charge = int(sys.argv[2])
    basis = sys.argv[3] if len(sys.argv) > 3 else "UGBS"
    print(generate_input(atom, charge, basis))
