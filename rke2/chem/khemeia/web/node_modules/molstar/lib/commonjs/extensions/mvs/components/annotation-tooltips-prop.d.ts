/**
 * Copyright (c) 2023-2026 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
import { CustomStructureProperty } from '../../../mol-model-props/common/custom-structure-property.js';
import { Loci } from '../../../mol-model/loci.js';
import { ParamDefinition as PD } from '../../../mol-util/param-definition.js';
/** Parameter definition for custom structure property "MVSAnnotationTooltips" */
export declare const MVSAnnotationTooltipsParams: {
    tooltips: PD.ObjectList<PD.Normalize<{
        annotationId: string;
        fieldName: string;
        textFormat: string;
    }>>;
};
export type MVSAnnotationTooltipsParams = typeof MVSAnnotationTooltipsParams;
/** Values of custom structure property "MVSAnnotationTooltips" (and for its params at the same type) */
export type MVSAnnotationTooltipsProps = PD.Values<MVSAnnotationTooltipsParams>;
/** Provider for custom structure property "MVSAnnotationTooltips" */
export declare const MVSAnnotationTooltipsProvider: CustomStructureProperty.Provider<MVSAnnotationTooltipsParams, MVSAnnotationTooltipsProps>;
/** Label provider based on data from "MVS Annotation" custom model property */
export declare const MVSAnnotationTooltipsLabelProvider: {
    label: (loci: Loci) => string | undefined;
};
