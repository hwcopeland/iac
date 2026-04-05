/**
 * Copyright (c) 2019-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { Mat4, Vec3 } from '../../mol-math/linear-algebra.js';
export function getDistanceDataFromStructureSelections(s) {
    const lociA = s[0].loci;
    const lociB = s[1].loci;
    return { pairs: [{ loci: [lociA, lociB] }] };
}
export function getAngleDataFromStructureSelections(s) {
    const lociA = s[0].loci;
    const lociB = s[1].loci;
    const lociC = s[2].loci;
    return { triples: [{ loci: [lociA, lociB, lociC] }] };
}
export function getDihedralDataFromStructureSelections(s) {
    const lociA = s[0].loci;
    const lociB = s[1].loci;
    const lociC = s[2].loci;
    const lociD = s[3].loci;
    return { quads: [{ loci: [lociA, lociB, lociC, lociD] }] };
}
export function getLabelDataFromStructureSelections(s) {
    const loci = s[0].loci;
    return { infos: [{ loci }] };
}
export function getOrientationDataFromStructureSelections(s) {
    return { locis: s.map(v => v.loci) };
}
export function getPlaneDataFromStructureSelections(s) {
    return { locis: s.map(v => v.loci) };
}
const GetTransformState = {
    center: Vec3(),
    rotation: Mat4(),
    translationToCenter: Mat4(),
    translationFromCenter: Mat4(),
    translation: Mat4(),
    local: Mat4(),
};
export function transformParamsNeedCentroid(src) {
    var _a;
    if (src.name === 'components' && ((_a = src.params.rotationCenter) === null || _a === void 0 ? void 0 : _a.name) === 'centroid') {
        return true;
    }
    return false;
}
export function getTransformFromParams(src, centroid) {
    var _a, _b;
    if (src.name === 'matrix') {
        const transform = Mat4();
        Mat4.copy(transform, src.params.data);
        if (src.params.transpose)
            Mat4.transpose(transform, transform);
        return transform;
    }
    else {
        if (((_a = src.params.rotationCenter) === null || _a === void 0 ? void 0 : _a.name) === 'centroid') {
            Vec3.copy(GetTransformState.center, centroid);
        }
        else if (((_b = src.params.rotationCenter) === null || _b === void 0 ? void 0 : _b.name) === 'point') {
            Vec3.copy(GetTransformState.center, src.params.rotationCenter.params.point);
        }
        else {
            Vec3.set(GetTransformState.center, 0, 0, 0);
        }
        Mat4.fromTranslation(GetTransformState.translationToCenter, GetTransformState.center);
        Mat4.fromRotation(GetTransformState.rotation, src.params.angle * Math.PI / 180, src.params.axis);
        Mat4.fromTranslation(GetTransformState.translationFromCenter, Vec3.negate(GetTransformState.center, GetTransformState.center));
        const transform = Mat4.mul3(Mat4(), GetTransformState.translationToCenter, GetTransformState.rotation, GetTransformState.translationFromCenter);
        Mat4.fromTranslation(GetTransformState.translation, src.params.translation);
        Mat4.mul(transform, GetTransformState.translation, transform);
        return transform;
    }
}
export const TransformParam = PD.MappedStatic('matrix', {
    matrix: PD.Group({
        data: PD.Mat4(Mat4.identity()),
        transpose: PD.Boolean(false),
    }, { isFlat: true }),
    components: PD.Group({
        translation: PD.Vec3(Vec3.create(0, 0, 0)),
        axis: PD.Vec3(Vec3.create(1, 0, 0)),
        angle: PD.Numeric(0, { min: -360, max: 360, step: 1 }, { description: 'Angle in Degrees' }),
        rotationCenter: PD.MappedStatic('point', {
            point: PD.Group({ point: PD.Vec3(Vec3.create(0, 0, 0)) }, { isFlat: true }),
            centroid: PD.Group({})
        }),
    }, { isFlat: true }),
}, { label: 'Kind' });
