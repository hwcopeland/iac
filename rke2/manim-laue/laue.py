"""Laue / Ewald diffraction of an NaCl crystal by a Cu Kα plane wave.

Pure physics in module scope; Manim scene at the bottom.
Run `python3 laue.py --self-check` to verify 2θ predictions without rendering.
Run `manim -qh laue.py LaueDiffraction` to render the animation.
"""
from __future__ import annotations

import math
import sys
from dataclasses import dataclass

import numpy as np

# ---------------------------------------------------------------------------
# Constants  (Å, Å⁻¹, dimensionless)
# ---------------------------------------------------------------------------
A_NACL = 5.6402            # NaCl cubic lattice constant, Å
LAMBDA_CU = 1.5418         # Cu Kα weighted average, Å
K_MAG = 2.0 * math.pi / LAMBDA_CU  # |k_in| = |k_out| in reciprocal space

# Cromer–Mann coefficients (neutral atoms; International Tables vol. C).
# f(s) = Σ a_i exp(-b_i s²) + c,   s = sinθ/λ = |Q|/(4π)
CROMER_MANN = {
    "Na": dict(
        a=[4.7626, 3.1736, 1.2674, 1.1128],
        b=[3.2850, 8.8422, 0.3136, 129.424],
        c=0.676,
    ),
    "Cl": dict(
        a=[11.4604, 7.1964, 6.2556, 1.6455],
        b=[0.0104, 1.1662, 18.5194, 47.7784],
        c=-9.5574,
    ),
}


# ---------------------------------------------------------------------------
# Pure physics — no Manim imports here
# ---------------------------------------------------------------------------
def form_factor(element: str, q_mag: float) -> float:
    """Atomic form factor f(Q) via the 4-Gaussian Cromer–Mann parametrization."""
    s = q_mag / (4.0 * math.pi)
    p = CROMER_MANN[element]
    return sum(a * math.exp(-b * s * s) for a, b in zip(p["a"], p["b"])) + p["c"]


def d_spacing(h: int, k: int, l: int, a: float = A_NACL) -> float:
    """Interplanar spacing for cubic lattice."""
    return a / math.sqrt(h * h + k * k + l * l)


def two_theta_deg(h: int, k: int, l: int,
                  a: float = A_NACL, wavelength: float = LAMBDA_CU) -> float | None:
    """Bragg 2θ in degrees; None if the reflection is geometrically inaccessible."""
    d = d_spacing(h, k, l, a)
    sin_theta = wavelength / (2.0 * d)
    if sin_theta > 1.0:
        return None
    return 2.0 * math.degrees(math.asin(sin_theta))


def structure_factor(h: int, k: int, l: int, q_mag: float) -> complex:
    """NaCl rock-salt structure factor for the conventional cubic cell.

    F_hkl = [f_Na + f_Cl · exp(iπ(h+k+l))] · [1 + e^{iπ(h+k)} + e^{iπ(h+l)} + e^{iπ(k+l)}]

    Second bracket = 4 when (h,k,l) all same parity, 0 otherwise — the FCC rule.
    First bracket gives the NaCl basis: large for all-even, small for all-odd.
    """
    parities = {h & 1, k & 1, l & 1}
    if len(parities) != 1:
        return 0.0 + 0.0j  # mixed parity → forbidden
    f_na = form_factor("Na", q_mag)
    f_cl = form_factor("Cl", q_mag)
    basis = f_na + f_cl * cmath_exp_ipi(h + k + l)
    return 4.0 * basis  # bracket evaluates to 4 here


def cmath_exp_ipi(n: int) -> complex:
    """exp(iπn) without floating drift: ±1 exactly."""
    return 1.0 + 0.0j if (n % 2 == 0) else -1.0 + 0.0j


def multiplicity(h: int, k: int, l: int) -> int:
    """Cubic-system multiplicity of {hkl} family (positive indices assumed)."""
    s = sorted([abs(h), abs(k), abs(l)])
    a, b, c = s
    if a == 0 and b == 0 and c == 0:
        return 1
    zeros = s.count(0)
    distinct = len(set(s))
    if zeros == 2:                # (00l)
        return 6
    if zeros == 1:
        return 12 if distinct == 2 else 24   # (0kk) vs (0kl)
    # zeros == 0
    if distinct == 1:             # (hhh)
        return 8
    if distinct == 2:             # (hhl)
        return 24
    return 48                     # (hkl) all different


def lorentz_polarization(two_theta_rad: float) -> float:
    """Combined Lorentz–polarization factor for a powder/rotating-crystal geometry.

    L·P = (1 + cos²2θ) / (sin²θ · cos θ)
    Standard textbook (Cullity, Elements of X-ray Diffraction, eq. 4-21).
    """
    th = 0.5 * two_theta_rad
    sin_th = math.sin(th)
    cos_th = math.cos(th)
    if sin_th == 0.0 or cos_th == 0.0:
        return 0.0
    return (1.0 + math.cos(two_theta_rad) ** 2) / (sin_th * sin_th * cos_th)


@dataclass
class Reflection:
    hkl: tuple[int, int, int]
    g: np.ndarray         # reciprocal-lattice vector, Å⁻¹
    d: float              # plane spacing, Å
    two_theta: float      # degrees
    F: complex            # structure factor
    intensity: float      # |F|² · m · L·P (arbitrary units)
    allowed: bool         # parity rule
    mult: int


def enumerate_reflections(hkl_max: int = 4) -> list[Reflection]:
    """All (h,k,l) with |h|,|k|,|l| ≤ hkl_max, skipping (0,0,0) and duplicates by family.

    For animation purposes we keep one representative per (h,k,l) with non-negative
    h, k, l (lattice symmetry), since signs only affect direction in reciprocal space.
    The Manim scene re-expands to all signed permutations when placing points.
    """
    out: list[Reflection] = []
    for h in range(0, hkl_max + 1):
        for k in range(0, hkl_max + 1):
            for l in range(0, hkl_max + 1):
                if (h, k, l) == (0, 0, 0):
                    continue
                G = (2.0 * math.pi / A_NACL) * np.array([h, k, l], dtype=float)
                d = d_spacing(h, k, l)
                tt = two_theta_deg(h, k, l)
                if tt is None:
                    continue
                q_mag = float(np.linalg.norm(G))
                F = structure_factor(h, k, l, q_mag)
                allowed = abs(F) > 1e-9
                m = multiplicity(h, k, l) if allowed else 0
                lp = lorentz_polarization(math.radians(tt)) if allowed else 0.0
                I = abs(F) ** 2 * m * lp
                out.append(Reflection(
                    hkl=(h, k, l), g=G, d=d, two_theta=tt,
                    F=F, intensity=I, allowed=allowed, mult=m,
                ))
    return out


# ---------------------------------------------------------------------------
# Self-check  —  must pass before render
# ---------------------------------------------------------------------------
EXPECTED_2THETA = {
    (2, 0, 0): 31.74,
    (2, 2, 0): 45.50,
    (2, 2, 2): 56.51,
    (4, 0, 0): 66.30,
    (4, 2, 0): 75.40,
}


def self_check(tolerance_deg: float = 0.1) -> bool:
    print(f"NaCl,  a = {A_NACL} Å   Cu Kα,  λ = {LAMBDA_CU} Å   |k| = {K_MAG:.4f} Å⁻¹")
    print(f"{'hkl':<10}{'d (Å)':<10}{'2θ (°)':<10}{'expected':<10}{'Δ':<8}{'|F|':<10}{'I (a.u.)':<12}{'rule':<10}")
    ok = True
    for hkl, expected in EXPECTED_2THETA.items():
        tt = two_theta_deg(*hkl)
        q_mag = (2 * math.pi / A_NACL) * math.sqrt(sum(x * x for x in hkl))
        F = structure_factor(*hkl, q_mag)
        m = multiplicity(*hkl)
        lp = lorentz_polarization(math.radians(tt))
        I = abs(F) ** 2 * m * lp
        d = d_spacing(*hkl)
        delta = abs(tt - expected)
        flag = "OK" if delta <= tolerance_deg else "FAIL"
        if delta > tolerance_deg:
            ok = False
        rule = "even" if all(x % 2 == 0 for x in hkl) else "odd" if all(x % 2 == 1 for x in hkl) else "mixed"
        print(f"{str(hkl):<10}{d:<10.4f}{tt:<10.3f}{expected:<10.2f}{delta:<8.3f}{abs(F):<10.2f}{I:<12.2e}{rule:<6}{flag}")
    print()
    print("Forbidden-reflection check (must be zero):")
    for hkl in [(1, 0, 0), (1, 1, 0), (2, 1, 0), (2, 1, 1), (3, 2, 0)]:
        q_mag = (2 * math.pi / A_NACL) * math.sqrt(sum(x * x for x in hkl))
        F = structure_factor(*hkl, q_mag)
        flag = "OK" if abs(F) < 1e-9 else "FAIL"
        print(f"  {str(hkl):<10} |F| = {abs(F):.3e}  {flag}")
        if abs(F) >= 1e-9:
            ok = False
    return ok


# ---------------------------------------------------------------------------
# Manim scene  —  only imports manim when actually rendering
# ---------------------------------------------------------------------------
def _build_scene_class():
    from manim import (
        ThreeDScene, Sphere, VGroup, Arrow3D, Dot3D, Line3D, Surface, Square,
        ParametricFunction, Text, Create, FadeIn, FadeOut, Transform, Write,
        AnimationGroup, Rotate, MoveAlongPath, UpdateFromAlphaFunc,
        always_redraw, DEGREES, ORIGIN, OUT, RIGHT, UP, LEFT, DOWN, IN,
        YELLOW, GREEN, BLUE, RED, WHITE, GRAY, BLUE_E, TEAL, PURPLE,
        config,
    )
    import numpy as _np

    # Visualization scale: 1 Å of lattice = 0.5 Manim units; reciprocal-space
    # plot uses its own scale because |G| values are O(1–5) Å⁻¹.
    A_VIS = 0.5
    G_VIS = 0.6        # Å⁻¹ → Manim units in reciprocal-space frame
    K_VIS_LEN = K_MAG * G_VIS   # |k| in viz units

    NA_COLOR = "#F2D43A"   # sodium yellow
    CL_COLOR = "#5BC85B"   # chloride green
    NA_RADIUS_VIS = 0.15
    CL_RADIUS_VIS = 0.22

    class LaueDiffraction(ThreeDScene):
        """Six-phase Laue diffraction visualization.

        Phase 1: NaCl crystal in real space.
        Phase 2: Incoming plane wave.
        Phase 3: Reciprocal lattice with structure-factor-weighted points.
        Phase 4: Ewald sphere construction.
        Phase 5: Detector spot pattern.
        Phase 6: Rotating crystal — spots sweep.
        """

        def construct(self):
            self.set_camera_orientation(phi=70 * DEGREES, theta=-45 * DEGREES, distance=10)
            self.refls = enumerate_reflections(hkl_max=3)

            self.phase1_real_lattice()
            self.phase2_plane_wave()
            self.phase3_reciprocal_lattice()
            self.phase4_ewald_sphere()
            self.phase5_detector()
            self.phase6_rotation()

        # ----- Phase 1: real-space NaCl ----------------------------------
        def phase1_real_lattice(self):
            title = Text("NaCl, a = 5.64 Å", font_size=28).to_corner(UP + LEFT)
            self.add_fixed_in_frame_mobjects(title)

            atoms = VGroup()
            # 2x2x2 conventional cells
            for i in range(-2, 3):
                for j in range(-2, 3):
                    for k in range(-2, 3):
                        pos = _np.array([i, j, k]) * (A_VIS * 0.5)
                        # Na at integer sum-even sites of the doubled (½a) grid; Cl at odd-sum
                        if (i + j + k) % 2 == 0:
                            atom = Sphere(radius=NA_RADIUS_VIS, color=NA_COLOR, resolution=(12, 12))
                        else:
                            atom = Sphere(radius=CL_RADIUS_VIS, color=CL_COLOR, resolution=(12, 12))
                        atom.move_to(pos)
                        atoms.add(atom)

            self.play(FadeIn(atoms, lag_ratio=0.005, run_time=3))
            self.begin_ambient_camera_rotation(rate=0.15, about="theta")
            self.wait(6)
            self.stop_ambient_camera_rotation()
            self.crystal = atoms
            self.title1 = title

        # ----- Phase 2: plane wave ---------------------------------------
        def phase2_plane_wave(self):
            label = Text(f"Cu Kα,  λ = {LAMBDA_CU} Å", font_size=24).to_corner(UP + RIGHT)
            self.add_fixed_in_frame_mobjects(label)

            # k_in along +x (in lab frame). Wavefronts: planes ⊥ x, spaced by λ.
            k_dir = _np.array([1.0, 0.0, 0.0])
            wavefronts = VGroup()
            extent = 1.8
            n_planes = 8
            spacing = LAMBDA_CU * A_VIS  # to match the lattice scale visually
            for n in range(n_planes):
                plane = Square(side_length=2 * extent).set_fill(BLUE, opacity=0.18).set_stroke(BLUE, width=1)
                plane.rotate(90 * DEGREES, axis=UP)  # face normal along x
                plane.move_to((-3.5 + n * spacing) * k_dir)
                wavefronts.add(plane)

            k_arrow = Arrow3D(
                start=_np.array([-4.0, 0, 0]),
                end=_np.array([-2.5, 0, 0]),
                color=BLUE,
            )
            k_label = Text("k_in", font_size=22, color=BLUE).move_to(_np.array([-3.2, 0, 0.7]))

            self.play(Create(k_arrow), FadeIn(k_label), FadeIn(wavefronts), run_time=2)

            # Animate wavefronts translating along k_in for a few periods.
            self.play(
                wavefronts.animate.shift(2.5 * k_dir),
                rate_func=lambda t: t,
                run_time=4,
            )
            self.wait(0.5)
            self.play(FadeOut(wavefronts), FadeOut(k_arrow), FadeOut(k_label))
            self.wavefront_label = label

        # ----- Phase 3: reciprocal lattice -------------------------------
        def phase3_reciprocal_lattice(self):
            txt = Text("Reciprocal lattice  G = (2π/a)(h,k,l)", font_size=22).to_edge(DOWN)
            self.add_fixed_in_frame_mobjects(txt)

            self.play(self.crystal.animate.set_opacity(0.15), run_time=1.5)

            recip = VGroup()
            self.recip_points = []   # list of (Dot3D, hkl, allowed, intensity)
            max_I = max((r.intensity for r in self.refls if r.allowed), default=1.0)
            for r in self.refls:
                # expand to all signed permutations
                signs = set()
                for sh in (-1, 1):
                    for sk in (-1, 1):
                        for sl in (-1, 1):
                            signs.add((sh * r.hkl[0], sk * r.hkl[1], sl * r.hkl[2]))
                for hkl_s in signs:
                    G = (2.0 * math.pi / A_NACL) * _np.array(hkl_s, dtype=float) * G_VIS
                    if r.allowed:
                        radius = 0.04 + 0.16 * (r.intensity / max_I) ** 0.5
                        dot = Sphere(radius=radius, color=WHITE, resolution=(8, 8)).set_opacity(0.95)
                    else:
                        dot = Sphere(radius=0.04, color=GRAY, resolution=(8, 8)).set_opacity(0.25)
                    dot.move_to(G)
                    recip.add(dot)
                    self.recip_points.append((dot, hkl_s, r.allowed, r.intensity))

            self.play(FadeIn(recip, lag_ratio=0.01, run_time=3))
            self.reciprocal = recip
            self.recip_caption = txt
            self.wait(2)

        # ----- Phase 4: Ewald sphere -------------------------------------
        def phase4_ewald_sphere(self):
            # k_in in reciprocal space points along +x with magnitude |k| (viz units).
            k_in = _np.array([K_VIS_LEN, 0.0, 0.0])
            sphere_center = -k_in   # Ewald construction
            ewald = Sphere(radius=K_VIS_LEN, resolution=(36, 36))
            ewald.move_to(sphere_center).set_opacity(0.12).set_color(BLUE)

            k_in_arrow = Arrow3D(start=sphere_center, end=ORIGIN, color=BLUE)
            k_label = Text("k_in", font_size=20, color=BLUE)
            self.add_fixed_in_frame_mobjects(k_label)
            k_label.to_corner(UP + LEFT)

            self.play(Create(ewald), Create(k_in_arrow), run_time=2)

            # Find reflections lying on (within ε of) the sphere.
            eps = 0.18 * G_VIS  # tolerance in viz units
            excited_arrows = VGroup()
            self.excited_hkls = []
            for dot, hkl_s, allowed, intensity in self.recip_points:
                if not allowed:
                    continue
                G_vis = dot.get_center()
                # excited iff |G_vis - sphere_center| ≈ K_VIS_LEN
                dist = float(_np.linalg.norm(G_vis - sphere_center))
                if abs(dist - K_VIS_LEN) < eps:
                    dot.set_color(YELLOW).set_opacity(1.0)
                    k_out = G_vis - sphere_center
                    arr = Arrow3D(start=sphere_center, end=sphere_center + k_out,
                                  color=YELLOW, thickness=0.015)
                    excited_arrows.add(arr)
                    self.excited_hkls.append((hkl_s, k_out, intensity))

            self.play(Create(excited_arrows, lag_ratio=0.15), run_time=2)
            self.wait(2)
            self.ewald = ewald
            self.k_in_arrow = k_in_arrow
            self.excited_arrows = excited_arrows

        # ----- Phase 5: detector spots -----------------------------------
        def phase5_detector(self):
            # Detector: plane at x = D_DET downstream of crystal.
            D_DET = 3.2
            detector = Square(side_length=4.5).set_fill(BLUE_E, opacity=0.35).set_stroke(WHITE, width=2)
            detector.rotate(90 * DEGREES, axis=UP)
            detector.move_to(_np.array([D_DET, 0, 0]))

            self.play(FadeIn(detector), run_time=1.5)

            spots = VGroup()
            max_I = max((I for _, _, I in self.excited_hkls), default=1.0)
            for hkl_s, k_out, I in self.excited_hkls:
                # Direction of outgoing wave: k_out vector points from crystal to spot.
                khat = k_out / _np.linalg.norm(k_out)
                if khat[0] <= 0.01:
                    continue   # back-scattered, doesn't hit forward detector
                t = D_DET / khat[0]
                hit = khat * t
                radius = 0.06 + 0.20 * (I / max_I) ** 0.5
                spot = Dot3D(point=hit, radius=radius, color=YELLOW)
                spots.add(spot)

            self.play(FadeIn(spots, lag_ratio=0.05), run_time=2)
            self.detector = detector
            self.spots = spots
            self.wait(2)

        # ----- Phase 6: rotating crystal --------------------------------
        def phase6_rotation(self):
            note = Text("Rotating crystal — Laue spots sweep", font_size=22).to_edge(DOWN)
            self.add_fixed_in_frame_mobjects(note)

            rot_group = VGroup(self.reciprocal, self.excited_arrows)
            self.play(
                Rotate(rot_group, angle=60 * DEGREES, axis=OUT, about_point=ORIGIN),
                run_time=5,
            )
            self.wait(1)
            self.play(
                Rotate(rot_group, angle=-90 * DEGREES, axis=UP, about_point=ORIGIN),
                run_time=5,
            )
            self.wait(1.5)

    return LaueDiffraction


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------
if "--self-check" in sys.argv:
    ok = self_check()
    sys.exit(0 if ok else 1)

# Lazy import: when manim discovers scenes it imports this module; bind class at import.
try:
    LaueDiffraction = _build_scene_class()
except ImportError:
    # numpy-only environments (the self-check path) shouldn't fail at import.
    pass
