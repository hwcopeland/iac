/**
 * Copyright (c) 2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
export declare class ErrorContext {
    private errors;
    get(tag: string): ReadonlyArray<string>;
    add(tag: string, error: string): void;
    clear(tag: string): void;
}
