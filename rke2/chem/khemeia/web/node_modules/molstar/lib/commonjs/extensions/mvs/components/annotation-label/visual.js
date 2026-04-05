"use strict";
/**
 * Copyright (c) 2023-2026 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.MVSAnnotationLabelTextParams = void 0;
exports.MVSAnnotationLabelTextVisual = MVSAnnotationLabelTextVisual;
const tslib_1 = require("tslib");
const text_builder_1 = require("../../../../mol-geo/geometry/text/text-builder.js");
const complex_visual_1 = require("../../../../mol-repr/structure/complex-visual.js");
const Original = tslib_1.__importStar(require("../../../../mol-repr/structure/visual/label-text.js"));
const element_1 = require("../../../../mol-repr/structure/visual/util/element.js");
const mol_util_1 = require("../../../../mol-util/index.js");
const names_1 = require("../../../../mol-util/color/names.js");
const object_1 = require("../../../../mol-util/object.js");
const param_definition_1 = require("../../../../mol-util/param-definition.js");
const string_format_1 = require("../../../../mol-util/string-format.js");
const label_text_1 = require("../../helpers/label-text.js");
const utils_1 = require("../../helpers/utils.js");
const annotation_prop_1 = require("../annotation-prop.js");
exports.MVSAnnotationLabelTextParams = {
    annotationId: param_definition_1.ParamDefinition.Text('', { description: 'Reference to "Annotation" custom model property', isEssential: true }),
    fieldName: param_definition_1.ParamDefinition.Text('label', { description: 'Annotation field (column) from which to take label contents', isEssential: true }),
    textFormat: param_definition_1.ParamDefinition.Text('{}', { description: 'Formatting template for the label text. Supports simplified f-string syntax. May reference multiple annotation fields. If value in any field is not defined, label will not be displayed.', isEssential: true }),
    groupByFields: param_definition_1.ParamDefinition.ObjectList({ fieldName: param_definition_1.ParamDefinition.Text(), }, obj => obj.fieldName, { defaultValue: [{ fieldName: 'group_id' }], description: 'Set of annotation fields for grouping annotation rows into label instances (i.e. annotation rows with the same values in all group-by fields will yield one label instance). Annotation row with undefined value in any group-by field is considered a separate label instance.', isEssential: true }),
    ...(0, object_1.omitObjectKeys)(Original.LabelTextParams, ['level', 'chainScale', 'residueScale', 'elementScale']),
    borderColor: { ...Original.LabelTextParams.borderColor, defaultValue: names_1.ColorNames.black },
};
/** Create "label-text" visual for "MVS Annotation Label" representation */
function MVSAnnotationLabelTextVisual(materialId) {
    return (0, complex_visual_1.ComplexTextVisual)({
        defaultProps: param_definition_1.ParamDefinition.getDefaultValues(exports.MVSAnnotationLabelTextParams),
        createGeometry: createLabelText,
        createLocationIterator: element_1.ElementIterator.fromStructure,
        getLoci: element_1.getSerialElementLoci,
        eachLocation: element_1.eachSerialElement,
        setUpdateState: (state, newProps, currentProps) => {
            state.createGeometry =
                newProps.annotationId !== currentProps.annotationId
                    || newProps.fieldName !== currentProps.fieldName
                    || newProps.textFormat !== currentProps.textFormat
                    || !(0, mol_util_1.arrayEqual)(newProps.groupByFields, currentProps.groupByFields);
        }
    }, materialId);
}
function createLabelText(ctx, structure, theme, props, text) {
    var _a;
    const { annotation, model } = (0, annotation_prop_1.getMVSAnnotationForStructure)(structure, props.annotationId);
    const rows = (_a = annotation === null || annotation === void 0 ? void 0 : annotation.getRows()) !== null && _a !== void 0 ? _a : [];
    const groups = utils_1.GroupedArray.groupIndices(rows, rowGroupingFunction(annotation, props.groupByFields.map(x => x.fieldName)));
    const builder = text_builder_1.TextBuilder.create(props, groups.count, groups.count / 2, text);
    const template = (0, string_format_1.FormatTemplate)(props.textFormat);
    for (let iGroup = 0; iGroup < groups.count; iGroup++) {
        const rowIndicesInGroup = utils_1.GroupedArray.getGroup(groups, iGroup);
        const labelText = template.format(field => annotation.getValueForRow(rowIndicesInGroup[0], field || props.fieldName));
        if (!labelText)
            continue;
        const rowsInGroup = rowIndicesInGroup.map(i => rows[i]);
        const p = (0, label_text_1.textPropsForSelection)(structure, rowsInGroup, model);
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
