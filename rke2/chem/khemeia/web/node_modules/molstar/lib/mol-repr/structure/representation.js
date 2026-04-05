/**
 * Copyright (c) 2018-2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { Representation } from '../representation.js';
export const StructureRepresentationStateBuilder = {
    create: () => {
        return {
            ...Representation.createState(),
            unitTransforms: null,
            unitTransformsVersion: -1
        };
    },
    update: (state, update) => {
        Representation.updateState(state, update);
        if (update.unitTransforms !== undefined)
            state.unitTransforms = update.unitTransforms;
    }
};
export function StructureRepresentationProvider(p) { return p; }
//
export { ComplexRepresentation } from './complex-representation.js';
export { ComplexVisual } from './complex-visual.js';
export { UnitsRepresentation } from './units-representation.js';
export { UnitsVisual } from './units-visual.js';
