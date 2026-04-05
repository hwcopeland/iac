/**
 * Copyright (c) 2020-2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 *
 * ported from https://github.com/photopea/UZIP.js/blob/master/UZIP.js
 * MIT License, Copyright (c) 2018 Photopea
 */
import { RuntimeContext } from '../../mol-task/index.js';
export declare function _inflate(runtime: RuntimeContext, data: Uint8Array<ArrayBuffer>, buf?: Uint8Array<ArrayBuffer>): Promise<Uint8Array<ArrayBuffer>>;
