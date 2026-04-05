"use strict";
/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.lib = void 0;
const tslib_1 = require("tslib");
const Structure = tslib_1.__importStar(require("../../mol-model/structure.js"));
const loci_1 = require("../../mol-model/loci.js");
const volume_1 = require("../../mol-model/volume.js");
const shape_1 = require("../../mol-model/shape.js");
const LinearAlgebra3D = tslib_1.__importStar(require("../../mol-math/linear-algebra/3d.js"));
const context_1 = require("../../mol-plugin/context.js");
const config_1 = require("../../mol-plugin/config.js");
const behavior_1 = require("../../mol-plugin/behavior.js");
const spec_1 = require("../../mol-plugin/spec.js");
const spec_2 = require("../../mol-plugin-ui/spec.js");
const objects_1 = require("../../mol-plugin-state/objects.js");
const transforms_1 = require("../../mol-plugin-state/transforms.js");
const actions_1 = require("../../mol-plugin-state/actions.js");
const extensions_1 = require("./extensions.js");
exports.lib = {
    structure: {
        ...Structure,
    },
    volume: {
        Volume: volume_1.Volume,
    },
    shape: {
        Shape: shape_1.Shape,
        ShapeGroup: shape_1.ShapeGroup,
    },
    loci: {
        Loci: loci_1.Loci,
        DataLoci: loci_1.DataLoci,
        EveryLoci: loci_1.EveryLoci,
    },
    math: {
        LinearAlgebra: {
            ...LinearAlgebra3D,
        }
    },
    plugin: {
        PluginContext: context_1.PluginContext,
        PluginConfig: config_1.PluginConfig,
        PluginBehavior: behavior_1.PluginBehavior,
        PluginSpec: spec_1.PluginSpec,
        PluginStateObject: objects_1.PluginStateObject,
        PluginStateTransform: objects_1.PluginStateTransform,
        StateTransforms: transforms_1.StateTransforms,
        StateActions: actions_1.StateActions,
        DefaultPluginSpec: spec_1.DefaultPluginSpec,
        DefaultPluginUISpec: spec_2.DefaultPluginUISpec,
    },
    extensions: {
        ...extensions_1.PluginExtensions
    }
};
