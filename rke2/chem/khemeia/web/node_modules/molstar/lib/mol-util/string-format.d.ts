/**
 * Copyright (c) 2026 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
/** Formatting template created from a Python-like f-string.
 * Supports simplified f-string functionality:
 * - only variable names (no expressions);
 * - supported types: `deEfF%cs`;
 * - not supported: types `bgGnoxX`, options `z` and `#`. */
export interface FormatTemplate {
    /** Source f-string for this template */
    fstring: string;
    /** Apply format template to values obtained by calling `valueGetter` on variable names. If any of the obtained values is `undefined`, return `undefined`. */
    format: (valueGetter: (name: string) => string | undefined) => string | undefined;
}
export declare function FormatTemplate(fstring: string): FormatTemplate;
declare const FORMAT_TYPES: readonly ["b", "d", "e", "E", "f", "F", "g", "G", "n", "o", "x", "X", "%", "c", "s"];
type FormatType = typeof FORMAT_TYPES[number];
interface FormatSpec {
    /** Formatting type */
    type: FormatType;
    /** Controls adding explicit sign for non-negative numbers */
    sign: '+' | '-' | ' ' | '';
    /** Converts negative zeros to positive (NOT SUPPORTED) */
    z: 'z' | '';
    /** Use alternative form (NOT SUPPORTED) */
    alt: '#' | '';
    /** Sets fill char to '0' and align to '=' */
    zeros: '0' | '';
    /** Min required width of output string */
    width: number;
    /** Controls digit grouping in large numbers */
    grouping: ',' | '_' | '';
    /** Number of decimal digits for types eEfF% */
    precision: number;
    /** Fill character for padding */
    fillChar: string;
    /** Align direction for padding */
    align: '<' | '>' | '=' | '^';
}
/** Format a value a la f-string, e.g. `formatValue('1.2', '.2f')` -> `'1.20'` */
export declare function formatValue(value: string, formatSpec: FormatSpec | string): string;
export {};
