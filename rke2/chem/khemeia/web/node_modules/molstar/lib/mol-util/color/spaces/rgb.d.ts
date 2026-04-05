/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import type { Color } from '../color.js';
export { Rgb };
interface Rgb extends Array<number> {
    [d: number]: number;
    '@type': 'normalized-rgb';
    length: 3;
}
declare function Rgb(): Rgb;
declare namespace Rgb {
    function zero(): Rgb;
    function fromColor(out: Rgb, hexColor: Color): Rgb;
    function toColor(rgb: Rgb): Color;
}
