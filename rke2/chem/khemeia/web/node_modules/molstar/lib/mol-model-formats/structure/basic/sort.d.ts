/**
 * Copyright (c) 2018 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { Column } from '../../../mol-data/db.js';
import { RuntimeContext } from '../../../mol-task/index.js';
import { AtomSite } from './schema.js';
export type SortedAtomSite = {
    atom_site: AtomSite;
    sourceIndex: Column<number>;
};
export declare function sortAtomSite(ctx: RuntimeContext, atom_site: AtomSite, start: number, end: number): Promise<SortedAtomSite>;
