"use strict";
/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.SnapshotControls = void 0;
const param_definition_1 = require("../../../mol-util/param-definition.js");
const behavior_1 = require("../behavior.js");
const binding_1 = require("../../../mol-util/binding.js");
const input_observer_1 = require("../../../mol-util/input/input-observer.js");
const M = input_observer_1.ModifiersKeys;
const Key = binding_1.Binding.TriggerKey;
const DefaultSnapshotControlsBindings = {
    next: (0, binding_1.Binding)([
        Key('ArrowRight', M.create({ control: true })),
        Key('GamepadY'),
    ]),
    previous: (0, binding_1.Binding)([
        Key('ArrowLeft', M.create({ control: true })),
        Key('GamepadX'),
    ]),
    first: (0, binding_1.Binding)([
        Key('ArrowUp', M.create({ control: true })),
    ]),
    last: (0, binding_1.Binding)([
        Key('ArrowDown', M.create({ control: true })),
    ]),
};
const SnapshotControlsParams = {
    bindings: param_definition_1.ParamDefinition.Value(DefaultSnapshotControlsBindings, { isHidden: true }),
};
exports.SnapshotControls = behavior_1.PluginBehavior.create({
    name: 'snapshot-controls',
    category: 'interaction',
    ctor: class extends behavior_1.PluginBehavior.Handler {
        register() {
            this.subscribeObservable(this.ctx.behaviors.interaction.keyReleased, ({ code, modifiers, key }) => {
                if (!this.ctx.canvas3d || this.ctx.isBusy)
                    return;
                const b = this.params.bindings;
                const { snapshot } = this.ctx.managers;
                if (binding_1.Binding.matchKey(b.next, code, modifiers, key)) {
                    snapshot.applyNext(1);
                }
                if (binding_1.Binding.matchKey(b.previous, code, modifiers, key)) {
                    snapshot.applyNext(-1);
                }
                if (binding_1.Binding.matchKey(b.first, code, modifiers, key)) {
                    const e = snapshot.state.entries.get(0);
                    const s = snapshot.setCurrent(e.snapshot.id);
                    if (s)
                        return this.ctx.state.setSnapshot(s);
                }
                if (binding_1.Binding.matchKey(b.last, code, modifiers, key)) {
                    const e = snapshot.state.entries.get(snapshot.state.entries.size - 1);
                    const s = snapshot.setCurrent(e.snapshot.id);
                    if (s)
                        return this.ctx.state.setSnapshot(s);
                }
            });
        }
    },
    params: () => SnapshotControlsParams,
    display: { name: 'Snapshot Controls' }
});
