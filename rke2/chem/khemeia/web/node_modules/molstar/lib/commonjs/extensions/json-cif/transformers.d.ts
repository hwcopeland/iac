/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { PluginStateObject } from '../../mol-plugin-state/objects.js';
import { StateTransformer } from '../../mol-state/index.js';
import { ParamDefinition } from '../../mol-util/param-definition.js';
import { JSONCifFile } from './model.js';
export declare const ParseJSONCifFileData: StateTransformer<PluginStateObject.Root, PluginStateObject.Format.Cif, ParamDefinition.Normalize<{
    data: JSONCifFile;
}>>;
