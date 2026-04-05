/**
 * Copyright (c) 2020-2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 *
 * ported from https://github.com/photopea/UZIP.js/blob/master/UZIP.js
 * MIT License, Copyright (c) 2018 Photopea
 *
 * - added `ungzip`
 */
import { RuntimeContext, Task } from '../../mol-task/index.js';
export declare function Unzip(buf: ArrayBuffer, onlyNames?: boolean): Task<{
    [k: string]: Uint8Array<ArrayBuffer> | {
        size: number;
        csize: number;
    };
}>;
export declare function unzip(runtime: RuntimeContext, buf: ArrayBuffer, onlyNames?: boolean): Promise<{
    [k: string]: Uint8Array<ArrayBuffer> | {
        size: number;
        csize: number;
    };
}>;
export declare function inflateRaw(runtime: RuntimeContext, file: Uint8Array<ArrayBuffer>, buf?: Uint8Array<ArrayBuffer>): Promise<Uint8Array<ArrayBuffer>>;
export declare function inflate(runtime: RuntimeContext, file: Uint8Array<ArrayBuffer>, buf?: Uint8Array<ArrayBuffer>): Promise<Uint8Array<ArrayBuffer>>;
export declare function ungzip(runtime: RuntimeContext, file: Uint8Array<ArrayBuffer>, buf?: Uint8Array<ArrayBuffer>): Promise<Uint8Array<ArrayBuffer>>;
export declare function deflate(runtime: RuntimeContext, data: Uint8Array<ArrayBuffer>, opts?: {
    level: number;
}): Promise<Uint8Array<ArrayBuffer>>;
export declare function Zip(obj: {
    [k: string]: Uint8Array<ArrayBuffer>;
}, noCmpr?: boolean): Task<ArrayBuffer>;
export declare function zip(runtime: RuntimeContext, obj: {
    [k: string]: Uint8Array<ArrayBuffer>;
}, noCmpr?: boolean): Promise<ArrayBuffer>;
