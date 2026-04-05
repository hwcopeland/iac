/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { PluginStateObject } from '../../mol-plugin-state/objects.js';
import { StateTransformer } from '../../mol-state/index.js';
import { Task } from '../../mol-task/index.js';
import { ParamDefinition } from '../../mol-util/param-definition.js';
import { parseJSONCif } from './parser.js';
const Transform = StateTransformer.builderFactory('json-cif');
export const ParseJSONCifFileData = Transform({
    name: 'parse-json-cif-data',
    from: PluginStateObject.Root,
    to: PluginStateObject.Format.Cif,
    params: {
        data: ParamDefinition.Value(undefined, { isHidden: true }),
    }
})({
    apply({ params }) {
        return Task.create('Parse JSON Cif', async (ctx) => {
            const parsed = parseJSONCif(params.data);
            return new PluginStateObject.Format.Cif(parsed, { label: 'CIF Data' });
        });
    }
});
