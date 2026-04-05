/**
 * Copyright (c) 2017 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { Column } from '../../../../../mol-data/db.js';
import { Tokens } from '../tokenizer.js';
export declare function FixedColumnProvider(lines: Tokens): <T extends Column.Schema>(offset: number, width: number, type: T) => Column<T["T"]>;
export declare function FixedColumn<T extends Column.Schema>(lines: Tokens, offset: number, width: number, schema: T): Column<T['T']>;
