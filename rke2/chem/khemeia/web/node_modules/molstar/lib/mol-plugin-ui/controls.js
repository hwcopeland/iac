import { jsx as _jsx, jsxs as _jsxs, Fragment as _Fragment } from "react/jsx-runtime";
/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import * as React from 'react';
import ReactMarkdown from 'react-markdown';
import { UpdateTrajectory } from '../mol-plugin-state/actions/structure.js';
import { StateTransforms } from '../mol-plugin-state/transforms.js';
import { PluginCommands } from '../mol-plugin/commands.js';
import { PluginReactContext, PluginUIComponent } from './base.js';
import { IconButton } from './controls/common.js';
import { Icon, NavigateBeforeSvg, NavigateNextSvg, SkipPreviousSvg, StopSvg, PlayArrowSvg, SubscriptionsOutlinedSvg, BuildSvg, AnimationSvg, RefreshSvg } from './controls/icons.js';
import { AnimationControls } from './state/animation.js';
import { StructureComponentControls } from './structure/components.js';
import { StructureMeasurementsControls } from './structure/measurements.js';
import { StructureSelectionActionsControls } from './structure/selection.js';
import { StructureSourceControls } from './structure/source.js';
import { VolumeStreamingControls, VolumeSourceControls } from './structure/volume.js';
import { PluginConfig } from '../mol-plugin/config.js';
import { StructureSuperpositionControls } from './structure/superposition.js';
import { StructureQuickStylesControls } from './structure/quick-styles.js';
import { Markdown } from './controls/markdown.js';
import { Slider } from './controls/slider.js';
import { AnimateStateSnapshotTransition } from '../mol-plugin-state/animation/built-in/state-snapshots.js';
import { PluginState } from '../mol-plugin/state.js';
export class TrajectoryViewportControls extends PluginUIComponent {
    constructor() {
        super(...arguments);
        this.state = { show: false, label: '' };
        this.update = () => {
            const state = this.plugin.state.data;
            const models = state.selectQ(q => q.ofTransformer(StateTransforms.Model.ModelFromTrajectory));
            if (models.length === 0) {
                this.setState({ show: false });
                return;
            }
            let label = '', count = 0;
            const parents = new Set();
            for (const m of models) {
                if (!m.sourceRef)
                    continue;
                const parent = state.cells.get(m.sourceRef).obj;
                if (!parent)
                    continue;
                if (parent.data.frameCount > 1) {
                    if (parents.has(m.sourceRef)) {
                        // do not show the controls if there are 2 models of the same trajectory present
                        this.setState({ show: false });
                        return;
                    }
                    parents.add(m.sourceRef);
                    count++;
                    if (!label) {
                        const idx = m.transform.params.modelIndex;
                        label = `Model ${Math.round(idx + 1)} / ${parent.data.frameCount}`;
                    }
                }
            }
            if (count > 1)
                label = '';
            this.setState({ show: count > 0, label });
        };
        this.reset = () => PluginCommands.State.ApplyAction(this.plugin, {
            state: this.plugin.state.data,
            action: UpdateTrajectory.create({ action: 'reset' })
        });
        this.prev = () => PluginCommands.State.ApplyAction(this.plugin, {
            state: this.plugin.state.data,
            action: UpdateTrajectory.create({ action: 'advance', by: -1 })
        });
        this.next = () => PluginCommands.State.ApplyAction(this.plugin, {
            state: this.plugin.state.data,
            action: UpdateTrajectory.create({ action: 'advance', by: 1 })
        });
    }
    componentDidMount() {
        this.subscribe(this.plugin.state.data.events.changed, this.update);
        this.subscribe(this.plugin.behaviors.state.isAnimating, this.update);
    }
    render() {
        const isAnimating = this.plugin.behaviors.state.isAnimating.value;
        if (!this.state.show || (isAnimating && !this.state.label) || !this.plugin.config.get(PluginConfig.Viewport.ShowTrajectoryControls))
            return null;
        return _jsxs("div", { className: 'msp-traj-controls', children: [!isAnimating && _jsx(IconButton, { svg: SkipPreviousSvg, title: 'First Model', onClick: this.reset, disabled: isAnimating }), !isAnimating && _jsx(IconButton, { svg: NavigateBeforeSvg, title: 'Previous Model', onClick: this.prev, disabled: isAnimating }), !isAnimating && _jsx(IconButton, { svg: NavigateNextSvg, title: 'Next Model', onClick: this.next, disabled: isAnimating }), !!this.state.label && _jsx("span", { children: this.state.label })] });
    }
}
export class StateSnapshotViewportControls extends PluginUIComponent {
    constructor() {
        super(...arguments);
        this.state = { isBusy: false, show: true, showAnimation: false };
        this.change = (e) => {
            if (e.target.value === 'none')
                return;
            this.update(e.target.value);
        };
        this.prev = () => {
            const s = this.plugin.managers.snapshot;
            const id = s.getNextId(s.state.current, -1);
            if (id)
                this.update(id);
        };
        this.next = () => {
            const s = this.plugin.managers.snapshot;
            const id = s.getNextId(s.state.current, 1);
            if (id)
                this.update(id);
        };
        this.togglePlay = () => {
            this.plugin.managers.snapshot.togglePlay();
        };
        this.toggleShowAnimation = () => {
            this.setState({ showAnimation: !this.state.showAnimation });
        };
        this.toggleStateAnimation = () => {
            if (this.state.isBusy) {
                this.plugin.managers.animation.stop();
                this.plugin.managers.markdownExtensions.audio.pause();
            }
            else {
                this.plugin.managers.animation.play(AnimateStateSnapshotTransition, {});
            }
        };
    }
    componentDidMount() {
        this.subscribe(this.plugin.managers.snapshot.events.changed, () => this.forceUpdate());
        this.subscribe(this.plugin.behaviors.state.isBusy, isBusy => this.setState({ isBusy }));
        this.subscribe(this.plugin.behaviors.state.isAnimating, isBusy => this.setState({ isBusy }));
    }
    componentWillUnmount() {
        super.componentWillUnmount();
    }
    async update(id) {
        this.setState({ isBusy: true });
        await PluginCommands.State.Snapshots.Apply(this.plugin, { id });
        this.setState({ isBusy: false });
    }
    get isStateTransitionPlaying() {
        return this.plugin.managers.animation.isAnimatingStateTransition;
    }
    render() {
        var _a, _b;
        const snapshots = this.plugin.managers.snapshot;
        const count = snapshots.state.entries.size;
        if (!count || !this.state.show) {
            return null;
        }
        const current = snapshots.state.current;
        const isPlaying = snapshots.state.isPlaying;
        const entry = snapshots.getEntry(current);
        const hasAnimation = !!(entry === null || entry === void 0 ? void 0 : entry.snapshot.transition);
        const disabled = isPlaying || (this.state.isBusy && !this.isStateTransitionPlaying);
        return _jsxs("div", { className: 'msp-state-snapshot-viewport-controls', children: [_jsxs("select", { className: 'msp-form-control', value: current || 'none', onChange: this.change, disabled: disabled, children: [!current && _jsx("option", { value: 'none' }, 'none'), snapshots.state.entries.valueSeq().map((e, i) => _jsxs("option", { value: e.snapshot.id, children: [`[${i + 1}/${count}]`, " ", e.name || new Date(e.timestamp).toLocaleString()] }, e.snapshot.id))] }), _jsx(IconButton, { svg: isPlaying ? StopSvg : PlayArrowSvg, title: isPlaying ? 'Pause' : 'Cycle States', onClick: this.togglePlay, disabled: isPlaying ? false : (this.state.isBusy && !this.isStateTransitionPlaying) }), !isPlaying && _jsxs(_Fragment, { children: [count > 1 && _jsx(IconButton, { svg: NavigateBeforeSvg, title: 'Previous State', onClick: this.prev, disabled: disabled }), count > 1 && _jsx(IconButton, { svg: NavigateNextSvg, title: 'Next State', onClick: this.next, disabled: disabled }), hasAnimation && _jsx(IconButton, { svg: AnimationSvg, className: 'msp-state-snapshot-animation-button', title: 'Snapshot Transition', onClick: this.toggleShowAnimation, disabled: !hasAnimation, toggleState: this.state.showAnimation })] }), hasAnimation && this.state.showAnimation && !isPlaying && _jsxs(_Fragment, { children: [_jsxs("div", { className: 'msp-state-snapshot-animation-slider msp-form-control', children: [_jsx(Slider, { value: Math.round(100 * ((_a = snapshots.state.currentAnimationTimeMs) !== null && _a !== void 0 ? _a : 0)) / 100, min: 0, step: PluginState.getMinFrameDuration(entry === null || entry === void 0 ? void 0 : entry.snapshot), max: (_b = PluginState.getStateTransitionDuration(entry === null || entry === void 0 ? void 0 : entry.snapshot)) !== null && _b !== void 0 ? _b : 1000, onChange: () => { }, onChangeImmediate: v => snapshots.setSnapshotAnimationFrame(v, true), hideInput: true, disabled: this.state.isBusy }), "\u00A0"] }), _jsx(IconButton, { svg: this.state.isBusy ? StopSvg : RefreshSvg, title: this.state.isBusy ? 'Stop' : 'Replay', onClick: this.toggleStateAnimation, toggleState: false })] })] });
    }
}
export function ViewportSnapshotDescription() {
    var _a;
    const plugin = React.useContext(PluginReactContext);
    const [_, setV] = React.useState(0);
    React.useEffect(() => {
        const sub = plugin.managers.snapshot.events.changed.subscribe(() => setV(v => v + 1));
        return () => sub.unsubscribe();
    }, [plugin]);
    const current = plugin.managers.snapshot.state.current;
    if (!current)
        return null;
    const e = plugin.managers.snapshot.getEntry(current);
    if (!((_a = e === null || e === void 0 ? void 0 : e.description) === null || _a === void 0 ? void 0 : _a.trim()))
        return null;
    return _jsx("div", { id: 'snapinfo', className: 'msp-snapshot-description-wrapper', children: e.descriptionFormat === 'plaintext'
            && e.description
            || _jsx(Markdown, { children: e.description }) });
}
export class AnimationViewportControls extends PluginUIComponent {
    constructor() {
        super(...arguments);
        this.state = { isEmpty: true, isExpanded: false, isBusy: false, isAnimating: false, isPlaying: false };
        this.toggleExpanded = () => this.setState({ isExpanded: !this.state.isExpanded });
        this.stop = () => {
            this.plugin.managers.animation.stop();
            this.plugin.managers.snapshot.stop();
        };
    }
    componentDidMount() {
        this.subscribe(this.plugin.managers.snapshot.events.changed, () => {
            if (this.plugin.managers.snapshot.state.isPlaying)
                this.setState({ isPlaying: true, isExpanded: false });
            else
                this.setState({ isPlaying: false });
        });
        this.subscribe(this.plugin.behaviors.state.isBusy, isBusy => {
            if (isBusy)
                this.setState({ isBusy: true, isExpanded: false, isEmpty: this.plugin.state.data.tree.transforms.size < 2 });
            else
                this.setState({ isBusy: false, isEmpty: this.plugin.state.data.tree.transforms.size < 2 });
        });
        this.subscribe(this.plugin.behaviors.state.isAnimating, isAnimating => {
            if (isAnimating)
                this.setState({ isAnimating: true, isExpanded: false });
            else
                this.setState({ isAnimating: false });
        });
    }
    render() {
        const isPlaying = this.plugin.managers.snapshot.state.isPlaying;
        if (isPlaying || this.state.isEmpty || this.plugin.managers.animation.isEmpty || !this.plugin.config.get(PluginConfig.Viewport.ShowAnimation))
            return null;
        const isAnimating = this.state.isAnimating;
        return _jsxs("div", { className: 'msp-animation-viewport-controls', children: [_jsxs("div", { children: [_jsx("div", { className: 'msp-semi-transparent-background' }), _jsx(IconButton, { svg: isAnimating || isPlaying ? StopSvg : SubscriptionsOutlinedSvg, transparent: true, title: isAnimating ? 'Stop' : 'Select Animation', onClick: isAnimating || isPlaying ? this.stop : this.toggleExpanded, toggleState: this.state.isExpanded, disabled: isAnimating || isPlaying ? false : this.state.isBusy || this.state.isPlaying || this.state.isEmpty })] }), (this.state.isExpanded && !this.state.isBusy) && _jsx("div", { className: 'msp-animation-viewport-controls-select', children: _jsx(AnimationControls, { onStart: this.toggleExpanded }) })] });
    }
}
export class SelectionViewportControls extends PluginUIComponent {
    componentDidMount() {
        this.subscribe(this.plugin.behaviors.interaction.selectionMode, () => this.forceUpdate());
    }
    render() {
        if (!this.plugin.selectionMode)
            return null;
        return _jsx("div", { className: 'msp-selection-viewport-controls', children: _jsx(StructureSelectionActionsControls, {}) });
    }
}
export class LociLabels extends PluginUIComponent {
    constructor() {
        super(...arguments);
        this.state = { labels: [] };
    }
    componentDidMount() {
        this.subscribe(this.plugin.behaviors.labels.highlight, e => this.setState({ labels: e.labels }));
    }
    render() {
        if (this.state.labels.length === 0) {
            return null;
        }
        return _jsx("div", { className: 'msp-highlight-info', children: this.state.labels.map((e, i) => {
                if (e.indexOf('\n') >= 0) {
                    return _jsx("div", { className: 'msp-highlight-markdown-row', children: _jsx(ReactMarkdown, { skipHtml: true, children: e }) }, '' + i);
                }
                return _jsx("div", { className: 'msp-highlight-simple-row', dangerouslySetInnerHTML: { __html: e } }, '' + i);
            }) });
    }
}
export class CustomStructureControls extends PluginUIComponent {
    componentDidMount() {
        this.subscribe(this.plugin.state.behaviors.events.changed, () => this.forceUpdate());
    }
    render() {
        const controls = [];
        this.plugin.customStructureControls.forEach((Controls, key) => {
            controls.push(_jsx(Controls, { initiallyCollapsed: this.props.initiallyCollapsed }, key));
        });
        return controls.length > 0 ? _jsx(_Fragment, { children: controls }) : null;
    }
}
export class DefaultStructureTools extends PluginUIComponent {
    render() {
        return _jsxs(_Fragment, { children: [_jsxs("div", { className: 'msp-section-header', children: [_jsx(Icon, { svg: BuildSvg }), "Structure Tools"] }), _jsx(StructureSourceControls, {}), _jsx(StructureMeasurementsControls, {}), _jsx(StructureSuperpositionControls, {}), _jsx(StructureQuickStylesControls, {}), _jsx(StructureComponentControls, {}), this.plugin.config.get(PluginConfig.VolumeStreaming.Enabled) && _jsx(VolumeStreamingControls, {}), _jsx(VolumeSourceControls, {}), _jsx(CustomStructureControls, {})] });
    }
}
