/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
export * from './behavior/behavior.js';
import * as StaticState from './behavior/static/state.js';
import * as StaticRepresentation from './behavior/static/representation.js';
import * as StaticCamera from './behavior/static/camera.js';
import * as StaticMisc from './behavior/static/misc.js';
import * as DynamicRepresentation from './behavior/dynamic/representation.js';
import * as DynamicCamera from './behavior/dynamic/camera.js';
import * as DynamicState from './behavior/dynamic/state.js';
import * as DynamicCustomProps from './behavior/dynamic/custom-props.js';
export const BuiltInPluginBehaviors = {
    State: StaticState,
    Representation: StaticRepresentation,
    Camera: StaticCamera,
    Misc: StaticMisc
};
export const PluginBehaviors = {
    Representation: DynamicRepresentation,
    Camera: DynamicCamera,
    State: DynamicState,
    CustomProps: DynamicCustomProps
};
