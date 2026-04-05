/**
 * Copyright (c) 2019-2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Sebastian Bittrich <sebastian.bittrich@rcsb.org>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { AccessibleSurfaceArea } from './accessible-surface-area/shrake-rupley.js';
import { CustomStructureProperty } from '../common/custom-structure-property.js';
import { QuerySymbolRuntime } from '../../mol-script/runtime/query/compiler.js';
export declare const AccessibleSurfaceAreaParams: {
    numberOfSpherePoints: PD.Numeric;
    probeSize: PD.Numeric;
    nonPolymer: PD.BooleanParam;
    traceOnly: PD.BooleanParam;
};
export type AccessibleSurfaceAreaParams = typeof AccessibleSurfaceAreaParams;
export type AccessibleSurfaceAreaProps = PD.Values<AccessibleSurfaceAreaParams>;
export declare const AccessibleSurfaceAreaSymbols: {
    isBuried: QuerySymbolRuntime;
    isAccessible: QuerySymbolRuntime;
};
export type AccessibleSurfaceAreaValue = AccessibleSurfaceArea;
export declare const AccessibleSurfaceAreaProvider: CustomStructureProperty.Provider<AccessibleSurfaceAreaParams, AccessibleSurfaceAreaValue>;
