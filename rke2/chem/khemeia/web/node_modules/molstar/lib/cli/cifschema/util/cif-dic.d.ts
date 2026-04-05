/**
 * Copyright (c) 2017-2022 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Database, Column } from './schema.js';
import { CifFrame } from '../../../mol-io/reader/cif/data-model.js';
export declare function getFieldType(type: string, description: string, values?: string[], container?: string): Column;
type Imports = Map<string, CifFrame[]>;
export declare function generateSchema(frames: CifFrame[], imports?: Imports): Database;
export {};
