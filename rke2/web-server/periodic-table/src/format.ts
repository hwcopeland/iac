// Small pure formatting helpers shared by the UI. Kept DOM-free for testing.

export function fmtNum(v: number | null | undefined, digits = 3): string {
  if (v == null || Number.isNaN(v)) return "—";
  if (v !== 0 && (Math.abs(v) >= 1e5 || Math.abs(v) < 1e-3)) {
    return v.toExponential(2);
  }
  const r = Number(v.toFixed(digits));
  return String(r);
}

export function kToC(k: number | null): number | null {
  return k == null ? null : k - 273.15;
}

export function kToF(k: number | null): number | null {
  return k == null ? null : (k - 273.15) * (9 / 5) + 32;
}

/** 1 eV = 96.485 kJ/mol (Faraday constant). Used to dual-label energies. */
export function kJmolToEv(v: number | null): number | null {
  return v == null ? null : v / 96.485;
}

/** "+2", "-1", "0" with explicit sign for oxidation-state display. */
export function signed(n: number): string {
  if (n === 0) return "0";
  return n > 0 ? `+${n}` : String(n);
}

/**
 * Parse an electron configuration string ("1s2 2s2 2p6" or "[Ne] 3s1") into
 * tokens so the UI can render the trailing electron count as a superscript.
 */
export interface ConfigToken {
  core?: string; // e.g. "[Ne]"
  shell?: string; // e.g. "3s"
  count?: number; // e.g. 1
}

export function parseConfig(config: string): ConfigToken[] {
  const tokens: ConfigToken[] = [];
  for (const part of config.trim().split(/\s+/)) {
    if (!part) continue;
    if (part.startsWith("[")) {
      tokens.push({ core: part });
      continue;
    }
    const m = part.match(/^(\d+[spdf])(\d+)$/);
    if (m) {
      tokens.push({ shell: m[1], count: Number(m[2]) });
    } else {
      tokens.push({ shell: part });
    }
  }
  return tokens;
}
