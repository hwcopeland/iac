/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { MVSData } from './mvs-data.js';
/**
 * Creates an MVSX zip file with from the provided data and assets
 */
export declare function createMVSX(data: MVSData, assets: {
    name: string;
    content: string | Uint8Array<ArrayBuffer>;
}[]): Promise<Uint8Array<ArrayBuffer>>;
