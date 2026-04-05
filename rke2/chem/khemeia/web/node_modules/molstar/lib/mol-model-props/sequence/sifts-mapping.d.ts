/**
 * Copyright (c) 2021-23 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { Model } from '../../mol-model/structure.js';
import { StructureElement } from '../../mol-model/structure/structure.js';
import { CustomModelProperty } from '../common/custom-model-property.js';
export { SIFTSMapping as SIFTSMapping };
interface SIFTSMappingMapping {
    readonly dbName: string[];
    readonly accession: string[];
    readonly num: string[];
    readonly residue: string[];
}
declare namespace SIFTSMapping {
    const Provider: CustomModelProperty.Provider<{}, SIFTSMappingMapping>;
    function isAvailable(model: Model): boolean;
    function getKey(loc: StructureElement.Location): string;
    function getLabel(loc: StructureElement.Location): string | undefined;
}
