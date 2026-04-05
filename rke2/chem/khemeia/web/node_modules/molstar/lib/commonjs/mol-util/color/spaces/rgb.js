"use strict";
/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.Rgb = Rgb;
function Rgb() {
    return Rgb.zero();
}
(function (Rgb) {
    function zero() {
        const out = [0.1, 0.0, 0.0];
        out[0] = 0;
        return out;
    }
    Rgb.zero = zero;
    function fromColor(out, hexColor) {
        out[0] = (hexColor >> 16 & 255) / 255;
        out[1] = (hexColor >> 8 & 255) / 255;
        out[2] = (hexColor & 255) / 255;
        return out;
    }
    Rgb.fromColor = fromColor;
    function toColor(rgb) {
        return (((rgb[0] * 255) << 16) | ((rgb[1] * 255) << 8) | (rgb[2] * 255));
    }
    Rgb.toColor = toColor;
})(Rgb || (exports.Rgb = Rgb = {}));
