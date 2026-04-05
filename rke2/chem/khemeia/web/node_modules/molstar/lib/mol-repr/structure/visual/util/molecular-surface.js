/**
 * Copyright (c) 2019-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Task } from '../../../../mol-task/index.js';
import { getUnitConformationAndRadius, ensureReasonableResolution, getStructureConformationAndRadius } from './common.js';
import { MolecularSurfaceCalculationParams, calcMolecularSurface } from '../../../../mol-math/geometry/molecular-surface.js';
import { OrderedSet } from '../../../../mol-data/int.js';
import { BaseGeometry } from '../../../../mol-geo/geometry/base.js';
import { ParamDefinition as PD } from '../../../../mol-util/param-definition.js';
export const CommonMolecularSurfaceCalculationParams = {
    ...MolecularSurfaceCalculationParams,
    resolution: { ...MolecularSurfaceCalculationParams.resolution, ...BaseGeometry.CustomQualityParamInfo },
    probePositions: { ...MolecularSurfaceCalculationParams.probePositions, ...BaseGeometry.CustomQualityParamInfo },
    floodfill: PD.Select('off', PD.arrayToOptions(['off', 'inside', 'outside']), { description: 'If and how to floodfill the molecular surface.' }),
};
function getUnitPositionDataAndMaxRadius(structure, unit, sizeTheme, props) {
    const { probeRadius } = props;
    const { position, boundary, radius } = getUnitConformationAndRadius(structure, unit, sizeTheme, props);
    const { indices } = position;
    const n = OrderedSet.size(indices);
    const radii = new Float32Array(OrderedSet.end(indices));
    let maxRadius = 0;
    for (let i = 0; i < n; ++i) {
        const j = OrderedSet.getAt(indices, i);
        const r = radius(j);
        if (maxRadius < r)
            maxRadius = r;
        radii[j] = r + probeRadius;
    }
    return { position: { ...position, radius: radii }, boundary, maxRadius };
}
export function computeUnitMolecularSurface(structure, unit, sizeTheme, props) {
    const { position, boundary, maxRadius } = getUnitPositionDataAndMaxRadius(structure, unit, sizeTheme, props);
    const p = ensureReasonableResolution(boundary.box, props);
    return Task.create('Molecular Surface', async (ctx) => {
        return await MolecularSurface(ctx, position, boundary, maxRadius, boundary.box, p);
    });
}
//
function getStructurePositionDataAndMaxRadius(structure, sizeTheme, props) {
    const { probeRadius } = props;
    const { position, boundary, radius } = getStructureConformationAndRadius(structure, sizeTheme, props);
    const { indices } = position;
    const n = OrderedSet.size(indices);
    const radii = new Float32Array(OrderedSet.end(indices));
    let maxRadius = 0;
    for (let i = 0; i < n; ++i) {
        const j = OrderedSet.getAt(indices, i);
        const r = radius(j);
        if (maxRadius < r)
            maxRadius = r;
        radii[j] = r + probeRadius;
    }
    return { position: { ...position, radius: radii }, boundary, maxRadius };
}
export function computeStructureMolecularSurface(structure, sizeTheme, props) {
    const { position, boundary, maxRadius } = getStructurePositionDataAndMaxRadius(structure, sizeTheme, props);
    const p = ensureReasonableResolution(boundary.box, props);
    return Task.create('Molecular Surface', async (ctx) => {
        return await MolecularSurface(ctx, position, boundary, maxRadius, boundary.box, p);
    });
}
//
async function MolecularSurface(ctx, position, boundary, maxRadius, box, props) {
    return calcMolecularSurface(ctx, position, boundary, maxRadius, box, props);
}
