/**
 * Copyright (c) 2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { Trajectory } from '../../mol-model/structure.js';
import { LociLabelProvider } from '../../mol-plugin-state/manager/loci-label.js';
import { QuerySymbolRuntime } from '../../mol-script/runtime/query/base.js';
import { Task } from '../../mol-task/index.js';
import { G3dDataBlock } from './data.js';
import { FormatPropertyProvider } from '../../mol-model-formats/structure/common/property.js';
export declare function trajectoryFromG3D(data: G3dDataBlock): Task<Trajectory>;
export declare const G3dSymbols: {
    haplotype: QuerySymbolRuntime;
    chromosome: QuerySymbolRuntime;
    region: QuerySymbolRuntime;
};
export declare const G3dInfoDataProperty: FormatPropertyProvider<G3dInfoData>;
export declare function g3dHaplotypeQuery(haplotype: string): import("../../mol-script/language/expression.js").Expression;
export declare function g3dChromosomeQuery(chr: string): import("../../mol-script/language/expression.js").Expression;
export declare function g3dRegionQuery(chr: string, start: number, end: number): import("../../mol-script/language/expression.js").Expression;
export interface G3dInfoData {
    haplotypes: string[];
    haplotype: string[];
    start: Int32Array;
    resolution: number;
    chroms: string[];
}
export declare const G3dLabelProvider: LociLabelProvider;
