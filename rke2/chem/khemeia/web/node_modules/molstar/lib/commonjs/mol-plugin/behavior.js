"use strict";
/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.PluginBehaviors = exports.BuiltInPluginBehaviors = void 0;
const tslib_1 = require("tslib");
tslib_1.__exportStar(require("./behavior/behavior.js"), exports);
const StaticState = tslib_1.__importStar(require("./behavior/static/state.js"));
const StaticRepresentation = tslib_1.__importStar(require("./behavior/static/representation.js"));
const StaticCamera = tslib_1.__importStar(require("./behavior/static/camera.js"));
const StaticMisc = tslib_1.__importStar(require("./behavior/static/misc.js"));
const DynamicRepresentation = tslib_1.__importStar(require("./behavior/dynamic/representation.js"));
const DynamicCamera = tslib_1.__importStar(require("./behavior/dynamic/camera.js"));
const DynamicState = tslib_1.__importStar(require("./behavior/dynamic/state.js"));
const DynamicCustomProps = tslib_1.__importStar(require("./behavior/dynamic/custom-props.js"));
exports.BuiltInPluginBehaviors = {
    State: StaticState,
    Representation: StaticRepresentation,
    Camera: StaticCamera,
    Misc: StaticMisc
};
exports.PluginBehaviors = {
    Representation: DynamicRepresentation,
    Camera: DynamicCamera,
    State: DynamicState,
    CustomProps: DynamicCustomProps
};
