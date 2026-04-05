/**
 * Copyright (c) 2023-2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
import { CustomStructureProperty } from '../../../mol-model-props/common/custom-structure-property.js';
import { Loci } from '../../../mol-model/loci.js';
import { StructureElement } from '../../../mol-model/structure.js';
import { ParamDefinition as PD } from '../../../mol-util/param-definition.js';
import { ElementSet, Selector } from './selector.js';
/** Parameter definition for custom structure property "CustomTooltips" */
export type CustomTooltipsParams = typeof CustomTooltipsParams;
export declare const CustomTooltipsParams: {
    tooltips: PD.ObjectList<PD.Normalize<{
        text: string;
        selector: PD.NamedParams<"all" | "polymer" | "water" | "branched" | "ligand" | "ion" | "lipid" | "protein" | "coarse" | "nucleic" | "non-standard", "static"> | PD.NamedParams<import("../../../mol-script/types.js").Script, "script"> | PD.NamedParams<import("../../../mol-script/language/expression.js").Expression, "expression"> | PD.NamedParams<StructureElement.Bundle, "bundle"> | PD.NamedParams<PD.Normalize<{
            label: /*elided*/ any;
            nullIfEmpty: /*elided*/ any;
            annotationId: /*elided*/ any;
            fieldName: /*elided*/ any;
            fieldValues: /*elided*/ any;
        }>, "annotation">;
    }>>;
};
/** Parameter values of custom structure property "CustomTooltips" */
export type CustomTooltipsProps = PD.Values<CustomTooltipsParams>;
/** Values of custom structure property "CustomTooltips" (and for its params at the same type) */
export type CustomTooltipsData = {
    selector: Selector;
    text: string;
    elementSet?: ElementSet;
}[];
/** Provider for custom structure property "CustomTooltips" */
export declare const CustomTooltipsProvider: CustomStructureProperty.Provider<CustomTooltipsParams, CustomTooltipsData>;
/** Label provider based on custom structure property "CustomTooltips" */
export declare const CustomTooltipsLabelProvider: {
    label: (loci: Loci) => string | undefined;
};
