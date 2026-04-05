"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.DefaultStructureTools = exports.CustomStructureControls = exports.LociLabels = exports.SelectionViewportControls = exports.AnimationViewportControls = exports.StateSnapshotViewportControls = exports.TrajectoryViewportControls = void 0;
exports.ViewportSnapshotDescription = ViewportSnapshotDescription;
const tslib_1 = require("tslib");
const jsx_runtime_1 = require("react/jsx-runtime");
/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
const React = tslib_1.__importStar(require("react"));
const react_markdown_1 = tslib_1.__importDefault(require("react-markdown"));
const structure_1 = require("../mol-plugin-state/actions/structure.js");
const transforms_1 = require("../mol-plugin-state/transforms.js");
const commands_1 = require("../mol-plugin/commands.js");
const base_1 = require("./base.js");
const common_1 = require("./controls/common.js");
const icons_1 = require("./controls/icons.js");
const animation_1 = require("./state/animation.js");
const components_1 = require("./structure/components.js");
const measurements_1 = require("./structure/measurements.js");
const selection_1 = require("./structure/selection.js");
const source_1 = require("./structure/source.js");
const volume_1 = require("./structure/volume.js");
const config_1 = require("../mol-plugin/config.js");
const superposition_1 = require("./structure/superposition.js");
const quick_styles_1 = require("./structure/quick-styles.js");
const markdown_1 = require("./controls/markdown.js");
const slider_1 = require("./controls/slider.js");
const state_snapshots_1 = require("../mol-plugin-state/animation/built-in/state-snapshots.js");
const state_1 = require("../mol-plugin/state.js");
class TrajectoryViewportControls extends base_1.PluginUIComponent {
    constructor() {
        super(...arguments);
        this.state = { show: false, label: '' };
        this.update = () => {
            const state = this.plugin.state.data;
            const models = state.selectQ(q => q.ofTransformer(transforms_1.StateTransforms.Model.ModelFromTrajectory));
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
        this.reset = () => commands_1.PluginCommands.State.ApplyAction(this.plugin, {
            state: this.plugin.state.data,
            action: structure_1.UpdateTrajectory.create({ action: 'reset' })
        });
        this.prev = () => commands_1.PluginCommands.State.ApplyAction(this.plugin, {
            state: this.plugin.state.data,
            action: structure_1.UpdateTrajectory.create({ action: 'advance', by: -1 })
        });
        this.next = () => commands_1.PluginCommands.State.ApplyAction(this.plugin, {
            state: this.plugin.state.data,
            action: structure_1.UpdateTrajectory.create({ action: 'advance', by: 1 })
        });
    }
    componentDidMount() {
        this.subscribe(this.plugin.state.data.events.changed, this.update);
        this.subscribe(this.plugin.behaviors.state.isAnimating, this.update);
    }
    render() {
        const isAnimating = this.plugin.behaviors.state.isAnimating.value;
        if (!this.state.show || (isAnimating && !this.state.label) || !this.plugin.config.get(config_1.PluginConfig.Viewport.ShowTrajectoryControls))
            return null;
        return (0, jsx_runtime_1.jsxs)("div", { className: 'msp-traj-controls', children: [!isAnimating && (0, jsx_runtime_1.jsx)(common_1.IconButton, { svg: icons_1.SkipPreviousSvg, title: 'First Model', onClick: this.reset, disabled: isAnimating }), !isAnimating && (0, jsx_runtime_1.jsx)(common_1.IconButton, { svg: icons_1.NavigateBeforeSvg, title: 'Previous Model', onClick: this.prev, disabled: isAnimating }), !isAnimating && (0, jsx_runtime_1.jsx)(common_1.IconButton, { svg: icons_1.NavigateNextSvg, title: 'Next Model', onClick: this.next, disabled: isAnimating }), !!this.state.label && (0, jsx_runtime_1.jsx)("span", { children: this.state.label })] });
    }
}
exports.TrajectoryViewportControls = TrajectoryViewportControls;
class StateSnapshotViewportControls extends base_1.PluginUIComponent {
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
                this.plugin.managers.animation.play(state_snapshots_1.AnimateStateSnapshotTransition, {});
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
        await commands_1.PluginCommands.State.Snapshots.Apply(this.plugin, { id });
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
        return (0, jsx_runtime_1.jsxs)("div", { className: 'msp-state-snapshot-viewport-controls', children: [(0, jsx_runtime_1.jsxs)("select", { className: 'msp-form-control', value: current || 'none', onChange: this.change, disabled: disabled, children: [!current && (0, jsx_runtime_1.jsx)("option", { value: 'none' }, 'none'), snapshots.state.entries.valueSeq().map((e, i) => (0, jsx_runtime_1.jsxs)("option", { value: e.snapshot.id, children: [`[${i + 1}/${count}]`, " ", e.name || new Date(e.timestamp).toLocaleString()] }, e.snapshot.id))] }), (0, jsx_runtime_1.jsx)(common_1.IconButton, { svg: isPlaying ? icons_1.StopSvg : icons_1.PlayArrowSvg, title: isPlaying ? 'Pause' : 'Cycle States', onClick: this.togglePlay, disabled: isPlaying ? false : (this.state.isBusy && !this.isStateTransitionPlaying) }), !isPlaying && (0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [count > 1 && (0, jsx_runtime_1.jsx)(common_1.IconButton, { svg: icons_1.NavigateBeforeSvg, title: 'Previous State', onClick: this.prev, disabled: disabled }), count > 1 && (0, jsx_runtime_1.jsx)(common_1.IconButton, { svg: icons_1.NavigateNextSvg, title: 'Next State', onClick: this.next, disabled: disabled }), hasAnimation && (0, jsx_runtime_1.jsx)(common_1.IconButton, { svg: icons_1.AnimationSvg, className: 'msp-state-snapshot-animation-button', title: 'Snapshot Transition', onClick: this.toggleShowAnimation, disabled: !hasAnimation, toggleState: this.state.showAnimation })] }), hasAnimation && this.state.showAnimation && !isPlaying && (0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsxs)("div", { className: 'msp-state-snapshot-animation-slider msp-form-control', children: [(0, jsx_runtime_1.jsx)(slider_1.Slider, { value: Math.round(100 * ((_a = snapshots.state.currentAnimationTimeMs) !== null && _a !== void 0 ? _a : 0)) / 100, min: 0, step: state_1.PluginState.getMinFrameDuration(entry === null || entry === void 0 ? void 0 : entry.snapshot), max: (_b = state_1.PluginState.getStateTransitionDuration(entry === null || entry === void 0 ? void 0 : entry.snapshot)) !== null && _b !== void 0 ? _b : 1000, onChange: () => { }, onChangeImmediate: v => snapshots.setSnapshotAnimationFrame(v, true), hideInput: true, disabled: this.state.isBusy }), "\u00A0"] }), (0, jsx_runtime_1.jsx)(common_1.IconButton, { svg: this.state.isBusy ? icons_1.StopSvg : icons_1.RefreshSvg, title: this.state.isBusy ? 'Stop' : 'Replay', onClick: this.toggleStateAnimation, toggleState: false })] })] });
    }
}
exports.StateSnapshotViewportControls = StateSnapshotViewportControls;
function ViewportSnapshotDescription() {
    var _a;
    const plugin = React.useContext(base_1.PluginReactContext);
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
    return (0, jsx_runtime_1.jsx)("div", { id: 'snapinfo', className: 'msp-snapshot-description-wrapper', children: e.descriptionFormat === 'plaintext'
            && e.description
            || (0, jsx_runtime_1.jsx)(markdown_1.Markdown, { children: e.description }) });
}
class AnimationViewportControls extends base_1.PluginUIComponent {
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
        if (isPlaying || this.state.isEmpty || this.plugin.managers.animation.isEmpty || !this.plugin.config.get(config_1.PluginConfig.Viewport.ShowAnimation))
            return null;
        const isAnimating = this.state.isAnimating;
        return (0, jsx_runtime_1.jsxs)("div", { className: 'msp-animation-viewport-controls', children: [(0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsx)("div", { className: 'msp-semi-transparent-background' }), (0, jsx_runtime_1.jsx)(common_1.IconButton, { svg: isAnimating || isPlaying ? icons_1.StopSvg : icons_1.SubscriptionsOutlinedSvg, transparent: true, title: isAnimating ? 'Stop' : 'Select Animation', onClick: isAnimating || isPlaying ? this.stop : this.toggleExpanded, toggleState: this.state.isExpanded, disabled: isAnimating || isPlaying ? false : this.state.isBusy || this.state.isPlaying || this.state.isEmpty })] }), (this.state.isExpanded && !this.state.isBusy) && (0, jsx_runtime_1.jsx)("div", { className: 'msp-animation-viewport-controls-select', children: (0, jsx_runtime_1.jsx)(animation_1.AnimationControls, { onStart: this.toggleExpanded }) })] });
    }
}
exports.AnimationViewportControls = AnimationViewportControls;
class SelectionViewportControls extends base_1.PluginUIComponent {
    componentDidMount() {
        this.subscribe(this.plugin.behaviors.interaction.selectionMode, () => this.forceUpdate());
    }
    render() {
        if (!this.plugin.selectionMode)
            return null;
        return (0, jsx_runtime_1.jsx)("div", { className: 'msp-selection-viewport-controls', children: (0, jsx_runtime_1.jsx)(selection_1.StructureSelectionActionsControls, {}) });
    }
}
exports.SelectionViewportControls = SelectionViewportControls;
class LociLabels extends base_1.PluginUIComponent {
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
        return (0, jsx_runtime_1.jsx)("div", { className: 'msp-highlight-info', children: this.state.labels.map((e, i) => {
                if (e.indexOf('\n') >= 0) {
                    return (0, jsx_runtime_1.jsx)("div", { className: 'msp-highlight-markdown-row', children: (0, jsx_runtime_1.jsx)(react_markdown_1.default, { skipHtml: true, children: e }) }, '' + i);
                }
                return (0, jsx_runtime_1.jsx)("div", { className: 'msp-highlight-simple-row', dangerouslySetInnerHTML: { __html: e } }, '' + i);
            }) });
    }
}
exports.LociLabels = LociLabels;
class CustomStructureControls extends base_1.PluginUIComponent {
    componentDidMount() {
        this.subscribe(this.plugin.state.behaviors.events.changed, () => this.forceUpdate());
    }
    render() {
        const controls = [];
        this.plugin.customStructureControls.forEach((Controls, key) => {
            controls.push((0, jsx_runtime_1.jsx)(Controls, { initiallyCollapsed: this.props.initiallyCollapsed }, key));
        });
        return controls.length > 0 ? (0, jsx_runtime_1.jsx)(jsx_runtime_1.Fragment, { children: controls }) : null;
    }
}
exports.CustomStructureControls = CustomStructureControls;
class DefaultStructureTools extends base_1.PluginUIComponent {
    render() {
        return (0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsxs)("div", { className: 'msp-section-header', children: [(0, jsx_runtime_1.jsx)(icons_1.Icon, { svg: icons_1.BuildSvg }), "Structure Tools"] }), (0, jsx_runtime_1.jsx)(source_1.StructureSourceControls, {}), (0, jsx_runtime_1.jsx)(measurements_1.StructureMeasurementsControls, {}), (0, jsx_runtime_1.jsx)(superposition_1.StructureSuperpositionControls, {}), (0, jsx_runtime_1.jsx)(quick_styles_1.StructureQuickStylesControls, {}), (0, jsx_runtime_1.jsx)(components_1.StructureComponentControls, {}), this.plugin.config.get(config_1.PluginConfig.VolumeStreaming.Enabled) && (0, jsx_runtime_1.jsx)(volume_1.VolumeStreamingControls, {}), (0, jsx_runtime_1.jsx)(volume_1.VolumeSourceControls, {}), (0, jsx_runtime_1.jsx)(CustomStructureControls, {})] });
    }
}
exports.DefaultStructureTools = DefaultStructureTools;
