/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import * as Structure from '../../mol-model/structure.js';
import { DataLoci, EveryLoci, Loci } from '../../mol-model/loci.js';
import { Volume } from '../../mol-model/volume.js';
import { Shape, ShapeGroup } from '../../mol-model/shape.js';
import * as LinearAlgebra3D from '../../mol-math/linear-algebra/3d.js';
import { PluginContext } from '../../mol-plugin/context.js';
import { PluginConfig } from '../../mol-plugin/config.js';
import { PluginBehavior } from '../../mol-plugin/behavior.js';
import { DefaultPluginSpec, PluginSpec } from '../../mol-plugin/spec.js';
import { DefaultPluginUISpec } from '../../mol-plugin-ui/spec.js';
import { PluginStateObject, PluginStateTransform } from '../../mol-plugin-state/objects.js';
import { StateTransforms } from '../../mol-plugin-state/transforms.js';
import { StateActions } from '../../mol-plugin-state/actions.js';
import { PluginExtensions } from './extensions.js';
export const lib = {
    structure: {
        ...Structure,
    },
    volume: {
        Volume,
    },
    shape: {
        Shape,
        ShapeGroup,
    },
    loci: {
        Loci,
        DataLoci,
        EveryLoci,
    },
    math: {
        LinearAlgebra: {
            ...LinearAlgebra3D,
        }
    },
    plugin: {
        PluginContext,
        PluginConfig,
        PluginBehavior,
        PluginSpec,
        PluginStateObject,
        PluginStateTransform,
        StateTransforms,
        StateActions,
        DefaultPluginSpec,
        DefaultPluginUISpec,
    },
    extensions: {
        ...PluginExtensions
    }
};
