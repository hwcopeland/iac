/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
/** Apply changes to an immutable-like object */
export declare function produce<T>(base: T, recipe: (draft: T) => T | void): T;
