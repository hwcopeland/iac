"use strict";
/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author Ventura Rivera <venturaxrivera@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.DefaultPluginUISpec = void 0;
const transformers_1 = require("../mol-plugin/behavior/dynamic/volume-streaming/transformers.js");
const spec_1 = require("../mol-plugin/spec.js");
const volume_1 = require("./custom/volume.js");
const DefaultPluginUISpec = () => ({
    ...(0, spec_1.DefaultPluginSpec)(),
    customParamEditors: [
        [transformers_1.CreateVolumeStreamingBehavior, volume_1.VolumeStreamingCustomControls]
    ],
});
exports.DefaultPluginUISpec = DefaultPluginUISpec;
