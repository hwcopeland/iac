/**
 * Copyright (c) 2018-2022 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
import { PluginStateObject, PluginStateTransform } from '../../mol-plugin-state/objects.js';
import { StateTransformer } from '../../mol-state/index.js';
import { Task } from '../../mol-task/index.js';
import { VolsegEntry, VolsegEntryData, createVolsegEntryParams } from './entry-root.js';
import { VolsegState, VolsegStateParams, VOLSEG_STATE_FROM_ENTRY_TRANSFORMER_NAME } from './entry-state.js';
import { VolsegGlobalState, VolsegGlobalStateData, VolsegGlobalStateParams } from './global-state.js';
export const VolsegEntryFromRoot = PluginStateTransform.BuiltIn({
    name: 'volseg-entry-from-root',
    display: { name: 'Vol & Seg Entry', description: 'Vol & Seg Entry' },
    from: PluginStateObject.Root,
    to: VolsegEntry,
    params: (a, plugin) => createVolsegEntryParams(plugin),
})({
    apply({ a, params }, plugin) {
        return Task.create('Load Vol & Seg Entry', async () => {
            const data = await VolsegEntryData.create(plugin, params);
            return new VolsegEntry(data, { label: data.entryId, description: 'Vol & Seg Entry' });
        });
    },
    update({ b, oldParams, newParams }) {
        Object.assign(newParams, oldParams);
        console.error('Changing params of existing VolsegEntry node is not allowed');
        return StateTransformer.UpdateResult.Unchanged;
    }
});
export const VolsegStateFromEntry = PluginStateTransform.BuiltIn({
    name: VOLSEG_STATE_FROM_ENTRY_TRANSFORMER_NAME,
    display: { name: 'Vol & Seg Entry State', description: 'Vol & Seg Entry State' },
    from: VolsegEntry,
    to: VolsegState,
    params: VolsegStateParams,
})({
    apply({ a, params }, plugin) {
        return Task.create('Create Vol & Seg Entry State', async () => {
            return new VolsegState(params, { label: 'State' });
        });
    }
});
export const VolsegGlobalStateFromRoot = PluginStateTransform.BuiltIn({
    name: 'volseg-global-state-from-root',
    display: { name: 'Vol & Seg Global State', description: 'Vol & Seg Global State' },
    from: PluginStateObject.Root,
    to: VolsegGlobalState,
    params: VolsegGlobalStateParams,
})({
    apply({ a, params }, plugin) {
        return Task.create('Create Vol & Seg Global State', async () => {
            const data = new VolsegGlobalStateData(plugin, params);
            return new VolsegGlobalState(data, { label: 'Global State', description: 'Vol & Seg Global State' });
        });
    },
    update({ b, oldParams, newParams }) {
        b.data.currentState.next(newParams);
        return StateTransformer.UpdateResult.Updated;
    }
});
