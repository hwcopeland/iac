/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { create } from 'mutative/dist/index.js';
/** Apply changes to an immutable-like object */
export function produce(base, recipe) {
    if (typeof base === 'object' && !('prototype' in base)) {
        return create({ ...base }, recipe);
    }
    return create(base, recipe);
}
