/**
 * Copyright (c) 2020-2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Sebastian Bittrich <sebastian.bittrich@rcsb.org>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { StructureProperties, Unit } from '../../mol-model/structure.js';
import { CustomPropertyDescriptor } from '../../mol-model/custom-property.js';
import { ANVILParams, computeANVIL, isInMembranePlane } from './algorithm.js';
import { CustomStructureProperty } from '../../mol-model-props/common/custom-structure-property.js';
import { Vec3 } from '../../mol-math/linear-algebra.js';
import { QuerySymbolRuntime } from '../../mol-script/runtime/query/base.js';
import { CustomPropSymbol } from '../../mol-script/language/symbol.js';
import { Type } from '../../mol-script/language/type.js';
export const MembraneOrientationParams = {
    ...ANVILParams
};
export { MembraneOrientation };
var MembraneOrientation;
(function (MembraneOrientation) {
    let Tag;
    (function (Tag) {
        Tag["Representation"] = "membrane-orientation-3d";
    })(Tag = MembraneOrientation.Tag || (MembraneOrientation.Tag = {}));
    const pos = Vec3();
    MembraneOrientation.symbols = {
        isTransmembrane: QuerySymbolRuntime.Dynamic(CustomPropSymbol('computed', 'membrane-orientation.is-transmembrane', Type.Bool), ctx => {
            const { unit, structure } = ctx.element;
            const { x, y, z } = StructureProperties.atom;
            if (!Unit.isAtomic(unit))
                return 0;
            const membraneOrientation = MembraneOrientationProvider.get(structure).value;
            if (!membraneOrientation)
                return 0;
            Vec3.set(pos, x(ctx.element), y(ctx.element), z(ctx.element));
            const { normalVector, planePoint1, planePoint2 } = membraneOrientation;
            return isInMembranePlane(pos, normalVector, planePoint1, planePoint2);
        })
    };
})(MembraneOrientation || (MembraneOrientation = {}));
export const MembraneOrientationProvider = CustomStructureProperty.createProvider({
    label: 'Membrane Orientation',
    descriptor: CustomPropertyDescriptor({
        name: 'anvil_computed_membrane_orientation',
        symbols: MembraneOrientation.symbols,
        // TODO `cifExport`
    }),
    type: 'root',
    defaultParams: MembraneOrientationParams,
    getParams: (data) => MembraneOrientationParams,
    isApplicable,
    obtain: async (ctx, data, props) => {
        const p = { ...PD.getDefaultValues(MembraneOrientationParams), ...props };
        try {
            return { value: await computeAnvil(ctx, data, p) };
        }
        catch (e) {
            // the "Residues Embedded in Membrane" symbol may bypass isApplicable() checks
            console.warn('Failed to predict membrane orientation. This happens for short peptides and entries without amino acids.');
            return { value: undefined };
        }
    }
});
function isApplicable(structure) {
    if (!structure.isAtomic)
        return false;
    for (const model of structure.models) {
        const { byEntityKey } = model.sequence;
        for (const key of Object.keys(byEntityKey)) {
            const { kind, length } = byEntityKey[+key].sequence;
            if (kind !== 'protein')
                continue; // can only process protein chains
            if (length >= 15)
                return true; // short peptides might fail
        }
    }
    return false;
}
async function computeAnvil(ctx, data, props) {
    const p = { ...PD.getDefaultValues(ANVILParams), ...props };
    return await computeANVIL(data, p).runInContext(ctx.runtime);
}
