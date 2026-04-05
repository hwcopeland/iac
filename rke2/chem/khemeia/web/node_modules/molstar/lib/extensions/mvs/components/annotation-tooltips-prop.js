/**
 * Copyright (c) 2023-2026 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
import { CustomStructureProperty } from '../../../mol-model-props/common/custom-structure-property.js';
import { CustomPropertyDescriptor } from '../../../mol-model/custom-property.js';
import { StructureElement } from '../../../mol-model/structure.js';
import { ParamDefinition as PD } from '../../../mol-util/param-definition.js';
import { FormatTemplate } from '../../../mol-util/string-format.js';
import { filterDefined } from '../helpers/utils.js';
import { MVSAnnotationsProvider } from './annotation-prop.js';
/** Parameter definition for custom structure property "MVSAnnotationTooltips" */
export const MVSAnnotationTooltipsParams = {
    tooltips: PD.ObjectList({
        annotationId: PD.Text('', { description: 'Reference to "MVS Annotation" custom model property' }),
        fieldName: PD.Text('tooltip', { description: 'Annotation field (column) from which to take color values' }),
        textFormat: PD.Text('{}', { description: 'Formatting template for tooltip text. Supports simplified f-string syntax. May reference multiple annotation fields. If value in any field is not defined, tooltip will not be displayed.' }),
    }, obj => `${obj.annotationId}:${obj.fieldName}`),
};
/** Provider for custom structure property "MVSAnnotationTooltips" */
export const MVSAnnotationTooltipsProvider = CustomStructureProperty.createProvider({
    label: 'MVS Annotation Tooltips',
    descriptor: CustomPropertyDescriptor({
        name: 'mvs-annotation-tooltips',
    }),
    type: 'local',
    defaultParams: MVSAnnotationTooltipsParams,
    getParams: (data) => MVSAnnotationTooltipsParams,
    isApplicable: (data) => data.root === data,
    obtain: async (ctx, data, props) => {
        const fullProps = { ...PD.getDefaultValues(MVSAnnotationTooltipsParams), ...props };
        return { value: fullProps };
    },
});
/** Label provider based on data from "MVS Annotation" custom model property */
export const MVSAnnotationTooltipsLabelProvider = {
    label: (loci) => {
        switch (loci.kind) {
            case 'element-loci':
                if (!loci.structure.customPropertyDescriptors.hasReference(MVSAnnotationTooltipsProvider.descriptor))
                    return undefined;
                const location = StructureElement.Loci.getFirstLocation(loci);
                if (!location)
                    return undefined;
                const tooltipProps = MVSAnnotationTooltipsProvider.get(location.structure).value;
                if (!tooltipProps || tooltipProps.tooltips.length === 0)
                    return undefined;
                const annotations = MVSAnnotationsProvider.get(location.unit.model).value;
                const texts = tooltipProps.tooltips.map(p => FormatTemplate(p.textFormat).format(field => { var _a; return (_a = annotations === null || annotations === void 0 ? void 0 : annotations.getAnnotation(p.annotationId)) === null || _a === void 0 ? void 0 : _a.getValueForLocation(location, field || p.fieldName); }));
                return filterDefined(texts).join(' | ');
            default:
                return undefined;
        }
    }
};
