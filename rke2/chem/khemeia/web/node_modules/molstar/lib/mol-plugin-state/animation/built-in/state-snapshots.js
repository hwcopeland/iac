/**
 * Copyright (c) 2019-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { Canvas3DParams } from '../../../mol-canvas3d/canvas3d.js';
import { PluginState } from '../../../mol-plugin/state.js';
import { ParamDefinition as PD } from '../../../mol-util/param-definition.js';
import { PluginStateAnimation } from '../model.js';
async function setPartialSnapshot(plugin, entry, first = false) {
    var _a, _b, _c, _d, _e, _f, _g;
    if (entry.data) {
        await plugin.runTask(plugin.state.data.setSnapshot(entry.data));
        // update the canvas3d trackball with the snapshot
    }
    if ((_a = entry.canvas3d) === null || _a === void 0 ? void 0 : _a.props) {
        const settings = PD.normalizeParams(Canvas3DParams, entry.canvas3d.props, 'children');
        if (((_b = entry.camera) === null || _b === void 0 ? void 0 : _b.current) || ((_c = entry.camera) === null || _c === void 0 ? void 0 : _c.focus)) {
            // Avoid multiple camera transitions (creates ugly cases when camera in old and new snapshot is the same)
            settings.camera = undefined;
            settings.cameraClipping = undefined;
            settings.cameraFog = undefined;
        }
        (_d = plugin.canvas3d) === null || _d === void 0 ? void 0 : _d.setProps(settings);
    }
    if ((_e = entry.camera) === null || _e === void 0 ? void 0 : _e.current) {
        (_f = plugin.canvas3d) === null || _f === void 0 ? void 0 : _f.requestCameraReset({
            snapshot: entry.camera.current,
            durationMs: first || entry.camera.transitionStyle === 'instant'
                ? 0 : entry.camera.transitionDurationInMs,
        });
    }
    else if ((_g = entry.camera) === null || _g === void 0 ? void 0 : _g.focus) {
        plugin.managers.camera.focusObject({
            ...entry.camera.focus,
            durationMs: first || entry.camera.transitionStyle === 'instant'
                ? 0 : entry.camera.transitionDurationInMs,
        });
    }
}
export const AnimateStateSnapshots = PluginStateAnimation.create({
    name: 'built-in.animate-state-snapshots',
    display: { name: 'State Snapshots' },
    isExportable: true,
    params: () => ({}),
    canApply(plugin) {
        const entries = plugin.managers.snapshot.state.entries;
        if (entries.size < 1) {
            return { canApply: false, reason: 'At least 1 state required.' };
        }
        if (entries.some(e => !!(e === null || e === void 0 ? void 0 : e.snapshot.startAnimation))) {
            return { canApply: false, reason: 'Nested animations not supported.' };
        }
        return { canApply: plugin.managers.snapshot.state.entries.size > 0 };
    },
    setup(_, __, plugin) {
        const pivot = plugin.managers.snapshot.state.entries.get(0);
        setPartialSnapshot(plugin, pivot.snapshot, true);
    },
    getDuration: (_, plugin) => {
        return {
            kind: 'fixed',
            durationMs: plugin.managers.snapshot.state.entries.toArray().reduce((a, b) => { var _a; return a + ((_a = b.snapshot.durationInMs) !== null && _a !== void 0 ? _a : 0); }, 0)
        };
    },
    initialState: (_, plugin) => {
        const snapshots = plugin.managers.snapshot.state.entries.toArray();
        return {
            totalDuration: snapshots.reduce((a, b) => { var _a; return a + ((_a = b.snapshot.durationInMs) !== null && _a !== void 0 ? _a : 0); }, 0),
            snapshots,
            currentIndex: 0,
            currentTransitionFrame: 0,
        };
    },
    async apply(animState, t, ctx) {
        var _a;
        if (t.current >= animState.totalDuration) {
            return { kind: 'finished' };
        }
        let ctime = 0, i = 0;
        let ftime = 0;
        for (const s of animState.snapshots) {
            ftime = t.current - ctime;
            ctime += (_a = s.snapshot.durationInMs) !== null && _a !== void 0 ? _a : 0;
            if (t.current < ctime) {
                break;
            }
            i++;
        }
        if (i >= animState.snapshots.length)
            return { kind: 'finished' };
        const { transition, camera, canvas3d } = animState.snapshots[i].snapshot;
        const frameIndex = PluginState.getStateTransitionFrameIndex(animState.snapshots[i].snapshot, ftime);
        if (transition && frameIndex !== undefined) {
            if (i === animState.currentIndex && frameIndex === animState.currentTransitionFrame) {
                return { kind: 'skip' };
            }
            if (frameIndex === 0 || i !== animState.currentIndex) {
                await setPartialSnapshot(ctx.plugin, {
                    ...transition.frames[frameIndex],
                    camera,
                    canvas3d,
                });
            }
            else {
                await setPartialSnapshot(ctx.plugin, transition.frames[frameIndex]);
            }
            return { kind: 'next', state: { ...animState, currentIndex: i, currentAnimationFrame: frameIndex } };
        }
        if (i === animState.currentIndex) {
            return { kind: 'skip' };
        }
        await setPartialSnapshot(ctx.plugin, animState.snapshots[i].snapshot);
        return { kind: 'next', state: { ...animState, currentIndex: i, currentAnimationFrame: undefined } };
    }
});
export const AnimateStateSnapshotTransition = PluginStateAnimation.create({
    name: 'built-in.animate-state-snapshot-transition',
    display: { name: 'State Snapshot Transition' },
    isExportable: true,
    params: () => ({}),
    canApply(plugin) {
        const { snapshot } = plugin.managers;
        const { current } = snapshot;
        if (!(current === null || current === void 0 ? void 0 : current.snapshot.transition)) {
            return { canApply: false, reason: 'No transition found' };
        }
        return { canApply: true };
    },
    setup(_, __, plugin) {
        var _a, _b;
        const { current } = plugin.managers.snapshot;
        if (!current)
            return;
        setPartialSnapshot(plugin, (_b = (_a = current.snapshot.transition) === null || _a === void 0 ? void 0 : _a.frames[0]) !== null && _b !== void 0 ? _b : current.snapshot, true);
    },
    getDuration: (_, plugin) => {
        var _a, _b;
        const { current } = plugin.managers.snapshot;
        if (!(current === null || current === void 0 ? void 0 : current.snapshot.transition))
            return { kind: 'fixed', durationMs: 0 };
        if ((_a = current.snapshot.transition) === null || _a === void 0 ? void 0 : _a.loop) {
            return { kind: 'infinite' };
        }
        return {
            kind: 'fixed',
            durationMs: (_b = PluginState.getStateTransitionDuration(current.snapshot)) !== null && _b !== void 0 ? _b : 0
        };
    },
    initialState: (_, plugin) => {
        var _a, _b;
        const { current } = plugin.managers.snapshot;
        if (!current)
            return;
        return {
            totalDuration: ((_a = current.snapshot.transition) === null || _a === void 0 ? void 0 : _a.loop) ? Number.MAX_VALUE : ((_b = PluginState.getStateTransitionDuration(current.snapshot)) !== null && _b !== void 0 ? _b : 0),
            snapshots: [current],
            currentIndex: 0,
            currentTransitionFrame: undefined,
            isInitial: true,
        };
    },
    async apply(animState, t, ctx) {
        var _a;
        const snapshot = (_a = animState.snapshots[0]) === null || _a === void 0 ? void 0 : _a.snapshot;
        if (t.current >= animState.totalDuration) {
            if ((snapshot === null || snapshot === void 0 ? void 0 : snapshot.transition) && animState.isInitial) {
                const frameIndex = snapshot.transition.frames.length - 1;
                ctx.plugin.managers.snapshot.setSnapshotAnimationFrame(animState.totalDuration, false);
                await setPartialSnapshot(ctx.plugin, snapshot.transition.frames[frameIndex]);
            }
            return { kind: 'finished' };
        }
        if (!snapshot)
            return { kind: 'finished' };
        const { transition, camera, canvas3d } = snapshot;
        const frameIndex = PluginState.getStateTransitionFrameIndex(snapshot, t.current);
        if (!transition || frameIndex === undefined) {
            return { kind: 'finished' };
        }
        if (frameIndex === animState.currentTransitionFrame) {
            return { kind: 'skip' };
        }
        ctx.plugin.managers.snapshot.setSnapshotAnimationFrame(t.current, false);
        if (frameIndex === 0) {
            await setPartialSnapshot(ctx.plugin, {
                ...transition.frames[frameIndex],
                camera,
                canvas3d,
            });
        }
        else {
            await setPartialSnapshot(ctx.plugin, transition.frames[frameIndex]);
        }
        return { kind: 'next', state: { ...animState, currentAnimationFrame: frameIndex, isInitial: false } };
    }
});
