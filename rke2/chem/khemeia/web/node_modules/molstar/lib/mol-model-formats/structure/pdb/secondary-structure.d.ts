/**
 * Copyright (c) 2019-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author Kim Juho <juho_kim@outlook.com>
 */
import { CifCategory } from '../../../mol-io/reader/cif.js';
import { Tokens } from '../../../mol-io/reader/common/text/tokenizer.js';
export declare function parseHelix(lines: Tokens, lineStart: number, lineEnd: number): CifCategory;
export declare function parseSheet(lines: Tokens, lineStart: number, lineEnd: number): CifCategory;
