/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 *
 * Color conversion and interpolation code adapted from chroma.js (https://github.com/gka/chroma.js)
 * Copyright (c) 2011-2018, Gregor Aisch, BSD license
 */
import type { Color } from '../color.js';
import { Rgb } from './rgb.js';
export { Hsl };
interface Hsl extends Array<number> {
    [d: number]: number;
    '@type': 'hsl';
    length: 3;
}
declare function Hsl(): Hsl;
declare namespace Hsl {
    function zero(): Hsl;
    function fromColor(out: Hsl, color: Color): Hsl;
    function toColor(hsl: Hsl): Color;
    function fromRgb(out: Hsl, rgb: Rgb): Hsl;
    function toRgb(out: Rgb, hsl: Hsl): Rgb;
    function interpolate(out: Hsl, col1: Hsl, col2: Hsl, t: number): Hsl;
}
