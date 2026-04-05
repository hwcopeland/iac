"use strict";
/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.produce = produce;
const index_js_1 = require("mutative/dist/index.js");
/** Apply changes to an immutable-like object */
function produce(base, recipe) {
    if (typeof base === 'object' && !('prototype' in base)) {
        return (0, index_js_1.create)({ ...base }, recipe);
    }
    return (0, index_js_1.create)(base, recipe);
}
