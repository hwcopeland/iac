/**
 * Copyright (c) 2023-2026 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
import { TextBuilder } from '../../../../mol-geo/geometry/text/text-builder.js';
import { ComplexTextVisual } from '../../../../mol-repr/structure/complex-visual.js';
import * as Original from '../../../../mol-repr/structure/visual/label-text.js';
import { eachSerialElement, ElementIterator, getSerialElementLoci } from '../../../../mol-repr/structure/visual/util/element.js';
import { arrayEqual } from '../../../../mol-util/index.js';
import { ColorNames } from '../../../../mol-util/color/names.js';
import { omitObjectKeys } from '../../../../mol-util/object.js';
import { ParamDefinition as PD } from '../../../../mol-util/param-definition.js';
import { FormatTemplate } from '../../../../mol-util/string-format.js';
import { textPropsForSelection } from '../../helpers/label-text.js';
import { GroupedArray } from '../../helpers/utils.js';
import { getMVSAnnotationForStructure } from '../annotation-prop.js';
export const MVSAnnotationLabelTextParams = {
    annotationId: PD.Text('', { description: 'Reference to "Annotation" custom model property', isEssential: true }),
    fieldName: PD.Text('label', { description: 'Annotation field (column) from which to take label contents', isEssential: true }),
    textFormat: PD.Text('{}', { description: 'Formatting template for the label text. Supports simplified f-string syntax. May reference multiple annotation fields. If value in any field is not defined, label will not be displayed.', isEssential: true }),
    groupByFields: PD.ObjectList({ fieldName: PD.Text(), }, obj => obj.fieldName, { defaultValue: [{ fieldName: 'group_id' }], description: 'Set of annotation fields for grouping annotation rows into label instances (i.e. annotation rows with the same values in all group-by fields will yield one label instance). Annotation row with undefined value in any group-by field is considered a separate label instance.', isEssential: true }),
    ...omitObjectKeys(Original.LabelTextParams, ['level', 'chainScale', 'residueScale', 'elementScale']),
    borderColor: { ...Original.LabelTextParams.borderColor, defaultValue: ColorNames.black },
};
/** Create "label-text" visual for "MVS Annotation Label" representation */
export function MVSAnnotationLabelTextVisual(materialId) {
    return ComplexTextVisual({
        defaultProps: PD.getDefaultValues(MVSAnnotationLabelTextParams),
        createGeometry: createLabelText,
        createLocationIterator: ElementIterator.fromStructure,
        getLoci: getSerialElementLoci,
        eachLocation: eachSerialElement,
        setUpdateState: (state, newProps, currentProps) => {
            state.createGeometry =
                newProps.annotationId !== currentProps.annotationId
                    || newProps.fieldName !== currentProps.fieldName
                    || newProps.textFormat !== currentProps.textFormat
                    || !arrayEqual(newProps.groupByFields, currentProps.groupByFields);
        }
    }, materialId);
}
function createLabelText(ctx, structure, theme, props, text) {
    var _a;
    const { annotation, model } = getMVSAnnotationForStructure(structure, props.annotationId);
    const rows = (_a = annotation === null || annotation === void 0 ? void 0 : annotation.getRows()) !== null && _a !== void 0 ? _a : [];
    const groups = GroupedArray.groupIndices(rows, rowGroupingFunction(annotation, props.groupByFields.map(x => x.fieldName)));
    const builder = TextBuilder.create(props, groups.count, groups.count / 2, text);
    const template = FormatTemplate(props.textFormat);
    for (let iGroup = 0; iGroup < groups.count; iGroup++) {
        const rowIndicesInGroup = GroupedArray.getGroup(groups, iGroup);
        const labelText = template.format(field => annotation.getValueForRow(rowIndicesInGroup[0], field || props.fieldName));
        if (!labelText)
            continue;
        const rowsInGroup = rowIndicesInGroup.map(i => rows[i]);
        const p = textPropsForSelection(structure, rowsInGroup, model);
        if (!p)
            continue;
        builder.add(labelText, p.center[0], p.center[1], p.center[2], p.depth, p.scale, p.group);
    }
    return builder.getText();
}
function rowGroupingFunction(annotation, groupByFields) {
    if (groupByFields.length === 1) {
        const groupByField = groupByFields[0];
        return (row, i) => annotation.getValueForRow(i, groupByField);
    }
    if (groupByFields.length === 0) {
        return () => '';
    }
    return (row, i) => {
        const values = groupByFields.map(field => annotation.getValueForRow(i, field));
        if (values.includes(undefined))
            return undefined;
        return values.join('\t');
    };
}
