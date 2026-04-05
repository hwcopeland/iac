/**
 * Copyright (c) 2019-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { PluginStateObject } from '../objects.js';
import { DistanceData } from '../../mol-repr/shape/loci/distance.js';
import { LabelData } from '../../mol-repr/shape/loci/label.js';
import { OrientationData } from '../../mol-repr/shape/loci/orientation.js';
import { AngleData } from '../../mol-repr/shape/loci/angle.js';
import { DihedralData } from '../../mol-repr/shape/loci/dihedral.js';
import { PlaneData } from '../../mol-repr/shape/loci/plane.js';
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { Mat4, Vec3 } from '../../mol-math/linear-algebra.js';
export declare function getDistanceDataFromStructureSelections(s: ReadonlyArray<PluginStateObject.Molecule.Structure.SelectionEntry>): DistanceData;
export declare function getAngleDataFromStructureSelections(s: ReadonlyArray<PluginStateObject.Molecule.Structure.SelectionEntry>): AngleData;
export declare function getDihedralDataFromStructureSelections(s: ReadonlyArray<PluginStateObject.Molecule.Structure.SelectionEntry>): DihedralData;
export declare function getLabelDataFromStructureSelections(s: ReadonlyArray<PluginStateObject.Molecule.Structure.SelectionEntry>): LabelData;
export declare function getOrientationDataFromStructureSelections(s: ReadonlyArray<PluginStateObject.Molecule.Structure.SelectionEntry>): OrientationData;
export declare function getPlaneDataFromStructureSelections(s: ReadonlyArray<PluginStateObject.Molecule.Structure.SelectionEntry>): PlaneData;
export declare function transformParamsNeedCentroid(src: TransformParam): boolean;
export declare function getTransformFromParams(src: TransformParam, centroid: Vec3): Mat4;
export declare const TransformParam: PD.Mapped<PD.NamedParams<PD.Normalize<{
    data: Mat4;
    transpose: boolean;
}>, "matrix"> | PD.NamedParams<PD.Normalize<{
    translation: Vec3;
    axis: Vec3;
    angle: number;
    rotationCenter: PD.NamedParams<PD.Normalize<unknown>, "centroid"> | PD.NamedParams<PD.Normalize<{
        point: /*elided*/ any;
    }>, "point">;
}>, "components">>;
export type TransformParam = (typeof TransformParam)['defaultValue'];
