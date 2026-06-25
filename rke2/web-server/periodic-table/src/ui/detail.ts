// Builds the element detail panel: electronic structure, a Bohr shell diagram,
// a successive-ionization-energy chart (log scale — values span orders of
// magnitude and the jumps reveal shell closures), and the full property table.

import type { Element } from "../types";
import { fmtNum, kJmolToEv, kToC, parseConfig, signed } from "../format";
import { CATEGORY_COLORS, maxAnionicState } from "../trends";
import { h, svg } from "./dom";

function row(label: string, value: Node | string): HTMLElement {
  return h(
    "div",
    { class: "prop" },
    h("span", { class: "prop-label" }, label),
    h("span", { class: "prop-value" }, value),
  );
}

function configNode(config: string): HTMLElement {
  const wrap = h("span", { class: "config" });
  for (const t of parseConfig(config)) {
    if (t.core) {
      wrap.appendChild(h("span", { class: "config-core" }, t.core + " "));
    } else if (t.shell) {
      const span = h("span", { class: "config-shell" }, t.shell);
      if (t.count != null) span.appendChild(h("sup", {}, String(t.count)));
      span.appendChild(document.createTextNode(" "));
      wrap.appendChild(span);
    }
  }
  return wrap;
}

function shellDiagram(e: Element): SVGElement {
  const size = 200;
  const cx = size / 2;
  const cy = size / 2;
  const root = svg("svg", {
    viewBox: `0 0 ${size} ${size}`,
    class: "shell-diagram",
    role: "img",
    "aria-label": `Bohr model of ${e.name}: shells ${e.shells.join(", ")}`,
  });
  const nShells = e.shells.length || 1;
  const maxR = size / 2 - 14;
  const minR = 26;

  // nucleus
  root.appendChild(svg("circle", { cx, cy, r: 12, fill: CATEGORY_COLORS[e.category] ?? "#888" }));
  root.appendChild(
    svg("text", { x: cx, y: cy + 4, "text-anchor": "middle", class: "shell-nucleus" }, e.symbol),
  );

  e.shells.forEach((count, i) => {
    const r = nShells === 1 ? minR : minR + ((maxR - minR) * i) / (nShells - 1);
    root.appendChild(
      svg("circle", { cx, cy, r, fill: "none", stroke: "#3a4150", "stroke-width": 1 }),
    );
    // electrons distributed around the ring
    const dots = Math.min(count, 32);
    for (let k = 0; k < dots; k++) {
      const ang = (2 * Math.PI * k) / dots - Math.PI / 2;
      root.appendChild(
        svg("circle", {
          cx: cx + r * Math.cos(ang),
          cy: cy + r * Math.sin(ang),
          r: 2.4,
          fill: "#e9ecef",
        }),
      );
    }
    // count label on the ring (top)
    root.appendChild(
      svg(
        "text",
        { x: cx, y: cy - r - 2, "text-anchor": "middle", class: "shell-count" },
        String(count),
      ),
    );
  });
  return root;
}

function ionizationChart(e: Element): HTMLElement {
  const data = e.ionizationEnergies.slice(0, 10);
  if (data.length === 0) {
    return h("p", { class: "muted" }, "No ionization-energy data available.");
  }
  const w = 320;
  const hgt = 150;
  const pad = { l: 4, r: 4, t: 10, b: 22 };
  const innerW = w - pad.l - pad.r;
  const innerH = hgt - pad.t - pad.b;
  const logs = data.map((v) => Math.log10(v));
  const lo = Math.min(...logs);
  const hi = Math.max(...logs);
  const span = hi - lo || 1;
  const bw = innerW / data.length;

  const root = svg("svg", {
    viewBox: `0 0 ${w} ${hgt}`,
    class: "ie-chart",
    role: "img",
    "aria-label": `Successive ionization energies for ${e.name} (log scale)`,
  });

  data.forEach((v, i) => {
    const t = (logs[i] - lo) / span; // 0..1
    const barH = 6 + t * (innerH - 6);
    const x = pad.l + i * bw + bw * 0.15;
    const y = pad.t + (innerH - barH);
    root.appendChild(
      svg("rect", {
        x,
        y,
        width: bw * 0.7,
        height: barH,
        rx: 2,
        fill: "#4dabf7",
        "data-ie": v,
      }),
    );
    root.appendChild(
      svg(
        "text",
        { x: x + bw * 0.35, y: hgt - 8, "text-anchor": "middle", class: "ie-axis" },
        String(i + 1),
      ),
    );
  });

  return h(
    "div",
    {},
    root,
    h(
      "p",
      { class: "muted small" },
      `IE₁..IE${data.length} (kJ/mol), log scale. First: ${fmtNum(data[0], 1)} · highest shown: ${fmtNum(
        data[data.length - 1],
        1,
      )}`,
    ),
  );
}

/**
 * Electron affinity with both units and a plain-language sign cue: a positive
 * EA releases energy (a stable anion forms); a negative EA is endothermic
 * (no bound anion).
 */
function electronAffinityNode(e: Element): Node {
  if (e.electronAffinity == null) return h("span", { class: "muted" }, "—");
  const ev = kJmolToEv(e.electronAffinity);
  const wrap = h(
    "span",
    {},
    `${fmtNum(e.electronAffinity, 1)} kJ/mol (${fmtNum(ev, 2)} eV)`,
  );
  wrap.appendChild(
    h(
      "span",
      { class: "muted small ea-note" },
      e.electronAffinity > 0 ? " · stable anion" : " · no stable anion",
    ),
  );
  return wrap;
}

/** Deepest anion the element forms (most-negative oxidation state). */
function anionicNode(e: Element): Node {
  const s = maxAnionicState(e);
  if (s == null) return h("span", { class: "muted" }, e.number > 108 ? "predicted / unknown" : "—");
  if (s === 0) return h("span", { class: "muted" }, "none (forms no anion)");
  return h("span", { class: "ox common" }, signed(s));
}

function oxidationNode(e: Element): Node {
  if (e.oxidationStates.length === 0) {
    return h("span", { class: "muted" }, e.number > 108 ? "predicted / unknown" : "—");
  }
  const wrap = h("span", { class: "ox-states" });
  for (const s of e.oxidationStates) {
    const common = e.commonOxidationStates.includes(s);
    wrap.appendChild(h("span", { class: common ? "ox common" : "ox" }, signed(s)));
  }
  return wrap;
}

export function buildDetail(e: Element): HTMLElement {
  const badges = h("div", { class: "badges" });
  badges.appendChild(
    h("span", { class: "badge cat", style: `--c:${CATEGORY_COLORS[e.category] ?? "#888"}` }, e.categoryRaw),
  );
  badges.appendChild(h("span", { class: "badge" }, `Block ${e.block}`));
  if (e.radioactive) badges.appendChild(h("span", { class: "badge warn" }, "radioactive"));
  if (e.synthetic) badges.appendChild(h("span", { class: "badge warn" }, "synthetic"));

  const meltC = kToC(e.meltK);
  const boilC = kToC(e.boilK);

  const props = h(
    "div",
    { class: "props" },
    row("Atomic number", String(e.number)),
    row("Atomic mass", `${fmtNum(e.atomicMass, 4)} u`),
    row("Period / Group", `${e.period} / ${e.group ?? "f-block"}`),
    row("Phase (STP)", e.phase),
    row("Electronegativity", e.electronegativity == null ? "—" : `${fmtNum(e.electronegativity, 2)} χ (Pauling)`),
    row("Electron affinity", electronAffinityNode(e)),
    row("Max anionic state", anionicNode(e)),
    row("Outer-shell e⁻", String(e.valenceElectrons)),
    row("Oxidation states", oxidationNode(e)),
    row("Density", e.density == null ? "—" : `${fmtNum(e.density, 3)} g/cm³`),
    row(
      "Melting point",
      e.meltK == null ? "—" : `${fmtNum(e.meltK, 1)} K (${fmtNum(meltC, 1)} °C)`,
    ),
    row(
      "Boiling point",
      e.boilK == null ? "—" : `${fmtNum(e.boilK, 1)} K (${fmtNum(boilC, 1)} °C)`,
    ),
    row("Molar heat", e.molarHeat == null ? "—" : `${fmtNum(e.molarHeat, 2)} J/(mol·K)`),
  );

  const discovery = h(
    "div",
    { class: "props" },
    row("Discovered by", e.discoveredBy ?? "—"),
    row("Named by", e.namedBy ?? "—"),
    row("Appearance", e.appearance ?? "—"),
  );

  return h(
    "div",
    { class: "detail-body" },
    h(
      "div",
      { class: "detail-head" },
      h("div", { class: "detail-num" }, String(e.number)),
      h(
        "div",
        {},
        h("div", { class: "detail-symbol" }, e.symbol),
        h("div", { class: "detail-name" }, e.name),
      ),
      badges,
    ),

    h("h3", {}, "Electronic structure"),
    h("div", { class: "config-block" }, configNode(e.electronConfiguration)),
    h("div", { class: "config-block muted" }, configNode(e.electronConfigurationSemantic)),

    h(
      "div",
      { class: "two-col" },
      h("div", {}, h("h3", {}, "Shell structure"), shellDiagram(e)),
      h("div", {}, h("h3", {}, "Ionization energies"), ionizationChart(e)),
    ),

    h("h3", {}, "Properties"),
    props,

    h("h3", {}, "Discovery"),
    discovery,

    e.summary ? h("h3", {}, "Summary") : null,
    e.summary ? h("p", { class: "summary" }, e.summary) : null,
  );
}
