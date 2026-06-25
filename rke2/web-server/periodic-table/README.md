# Dynamic Periodic Table

An interactive, graduate-level periodic table served at
**[ptable.hwcopeland.net](https://ptable.hwcopeland.net)**.

Built with Vite + TypeScript (no UI framework), tested with Vitest, packaged as
a static site behind nginx, and deployed to the RKE2 cluster in the `web-server`
namespace via the `build-periodic-table` GitHub Actions workflow.

## Features

- **118 elements** (Z = 1–118) with a normalized, typed data layer.
- **Color-by trends** — recolor the whole table by category, phase, block,
  atomic mass, electronegativity, electron affinity, **anionic stability**
  (depth of the deepest anion an element forms, i.e. its most-negative
  oxidation state), first ionization energy, density, melting point, or boiling
  point. Numeric trends use a sequential scale with a live min/max legend;
  missing data is greyed.
- **Element detail drawer** — full and noble-gas-core electron configurations
  (rendered with superscripts), a Bohr shell diagram, a **successive
  ionization-energy chart** (log scale, so shell closures are visible), curated
  oxidation states (common states highlighted), and the full property table
  with explicit units throughout (electronegativity in Pauling χ, electron
  affinity in both kJ/mol and eV with a stable/no-stable-anion cue, the
  max anionic oxidation state, density, melting/boiling points in K and °C,
  molar heat capacity, discovery, appearance, summary).
- **Keyboard navigation** — arrow keys walk the grid, `Enter`/click opens the
  drawer, `Esc` closes.

## Data

The base dataset is the MIT-licensed
[Bowserinator/Periodic-Table-JSON](https://github.com/Bowserinator/Periodic-Table-JSON),
vendored at `src/data/periodic-table.json`. It is normalized and enriched in
`src/data/`:

- `enrichment.ts` — curated oxidation states (Greenwood & Earnshaw / CRC),
  category normalization, radioactive/synthetic flags.
- `elements.ts` — merges the raw data with the enrichment layer into the typed
  `Element[]` the UI consumes (element 119 is hypothetical and excluded).

Integrity is enforced by tests: shell occupancies must sum to Z for all 118
elements, grid coordinates must be unique, every element must resolve to a known
category, and every "common" oxidation state must be a subset of the full set.

## Develop

```sh
npm install
npm run dev        # http://localhost:5173
npm test           # vitest (data integrity + trends + format + DOM render)
npm run build      # tsc --noEmit && vite build → dist/
npm run preview    # serve the production build
```

## Deploy

Push any change under `rke2/web-server/periodic-table/` to `main`. The
`build-periodic-table` workflow runs on the self-hosted `arc-chem` runner:

1. Docker build (`Dockerfile`) installs deps, **runs `npm test` + the
   type-check as a gate**, then `vite build`s the static site into an
   `nginx:alpine` image.
2. The image is pushed to `zot.hwcopeland.net/web-server/periodic-table`.
3. `kubectl rollout restart deployment/periodic-table` rolls the new image.

The Kubernetes manifests (`deployment.yaml`, `httproute.yaml`, `dnsrecord.yaml`)
are applied to the cluster directly (same convention as `web-server/blog`).
`ptable.hwcopeland.net` is published as a proxied Cloudflare CNAME and routed
through `hwcopeland-gateway`.
