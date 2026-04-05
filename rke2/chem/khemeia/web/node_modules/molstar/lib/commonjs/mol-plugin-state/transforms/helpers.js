"use strict";
/**
 * Copyright (c) 2019-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.TransformParam = void 0;
exports.getDistanceDataFromStructureSelections = getDistanceDataFromStructureSelections;
exports.getAngleDataFromStructureSelections = getAngleDataFromStructureSelections;
exports.getDihedralDataFromStructureSelections = getDihedralDataFromStructureSelections;
exports.getLabelDataFromStructureSelections = getLabelDataFromStructureSelections;
exports.getOrientationDataFromStructureSelections = getOrientationDataFromStructureSelections;
exports.getPlaneDataFromStructureSelections = getPlaneDataFromStructureSelections;
exports.transformParamsNeedCentroid = transformParamsNeedCentroid;
exports.getTransformFromParams = getTransformFromParams;
const param_definition_1 = require("../../mol-util/param-definition.js");
const linear_algebra_1 = require("../../mol-math/linear-algebra.js");
function getDistanceDataFromStructureSelections(s) {
    const lociA = s[0].loci;
    const lociB = s[1].loci;
    return { pairs: [{ loci: [lociA, lociB] }] };
}
function getAngleDataFromStructureSelections(s) {
    const lociA = s[0].loci;
    const lociB = s[1].loci;
    const lociC = s[2].loci;
    return { triples: [{ loci: [lociA, lociB, lociC] }] };
}
function getDihedralDataFromStructureSelections(s) {
    const lociA = s[0].loci;
    const lociB = s[1].loci;
    const lociC = s[2].loci;
    const lociD = s[3].loci;
    return { quads: [{ loci: [lociA, lociB, lociC, lociD] }] };
}
function getLabelDataFromStructureSelections(s) {
    const loci = s[0].loci;
    return { infos: [{ loci }] };
}
function getOrientationDataFromStructureSelections(s) {
    return { locis: s.map(v => v.loci) };
}
function getPlaneDataFromStructureSelections(s) {
    return { locis: s.map(v => v.loci) };
}
const GetTransformState = {
    center: (0, linear_algebra_1.Vec3)(),
    rotation: (0, linear_algebra_1.Mat4)(),
    translationToCenter: (0, linear_algebra_1.Mat4)(),
    translationFromCenter: (0, linear_algebra_1.Mat4)(),
    translation: (0, linear_algebra_1.Mat4)(),
    local: (0, linear_algebra_1.Mat4)(),
};
function transformParamsNeedCentroid(src) {
    var _a;
    if (src.name === 'components' && ((_a = src.params.rotationCenter) === null || _a === void 0 ? void 0 : _a.name) === 'centroid') {
        return true;
    }
    return false;
}
function getTransformFromParams(src, centroid) {
    var _a, _b;
    if (src.name === 'matrix') {
        const transform = (0, linear_algebra_1.Mat4)();
        linear_algebra_1.Mat4.copy(transform, src.params.data);
        if (src.params.transpose)
            linear_algebra_1.Mat4.transpose(transform, transform);
        return transform;
    }
    else {
        if (((_a = src.params.rotationCenter) === null || _a === void 0 ? void 0 : _a.name) === 'centroid') {
            linear_algebra_1.Vec3.copy(GetTransformState.center, centroid);
        }
        else if (((_b = src.params.rotationCenter) === null || _b === void 0 ? void 0 : _b.name) === 'point') {
            linear_algebra_1.Vec3.copy(GetTransformState.center, src.params.rotationCenter.params.point);
        }
        else {
            linear_algebra_1.Vec3.set(GetTransformState.center, 0, 0, 0);
        }
        linear_algebra_1.Mat4.fromTranslation(GetTransformState.translationToCenter, GetTransformState.center);
        linear_algebra_1.Mat4.fromRotation(GetTransformState.rotation, src.params.angle * Math.PI / 180, src.params.axis);
        linear_algebra_1.Mat4.fromTranslation(GetTransformState.translationFromCenter, linear_algebra_1.Vec3.negate(GetTransformState.center, GetTransformState.center));
        const transform = linear_algebra_1.Mat4.mul3((0, linear_algebra_1.Mat4)(), GetTransformState.translationToCenter, GetTransformState.rotation, GetTransformState.translationFromCenter);
        linear_algebra_1.Mat4.fromTranslation(GetTransformState.translation, src.params.translation);
        linear_algebra_1.Mat4.mul(transform, GetTransformState.translation, transform);
        return transform;
    }
}
exports.TransformParam = param_definition_1.ParamDefinition.MappedStatic('matrix', {
    matrix: param_definition_1.ParamDefinition.Group({
        data: param_definition_1.ParamDefinition.Mat4(linear_algebra_1.Mat4.identity()),
        transpose: param_definition_1.ParamDefinition.Boolean(false),
    }, { isFlat: true }),
    components: param_definition_1.ParamDefinition.Group({
        translation: param_definition_1.ParamDefinition.Vec3(linear_algebra_1.Vec3.create(0, 0, 0)),
        axis: param_definition_1.ParamDefinition.Vec3(linear_algebra_1.Vec3.create(1, 0, 0)),
        angle: param_definition_1.ParamDefinition.Numeric(0, { min: -360, max: 360, step: 1 }, { description: 'Angle in Degrees' }),
        rotationCenter: param_definition_1.ParamDefinition.MappedStatic('point', {
            point: param_definition_1.ParamDefinition.Group({ point: param_definition_1.ParamDefinition.Vec3(linear_algebra_1.Vec3.create(0, 0, 0)) }, { isFlat: true }),
            centroid: param_definition_1.ParamDefinition.Group({})
        }),
    }, { isFlat: true }),
}, { label: 'Kind' });
