"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.MVSStoriesSnapshotMarkdownViewer = exports.MVSStoriesSnapshotMarkdownModel = void 0;
exports.MVSStoriesSnapshotMarkdownUI = MVSStoriesSnapshotMarkdownUI;
const jsx_runtime_1 = require("react/jsx-runtime");
/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
const rxjs_1 = require("rxjs");
const component_1 = require("../../../mol-plugin-state/component.js");
const context_1 = require("../context.js");
const use_behavior_1 = require("../../../mol-plugin-ui/hooks/use-behavior.js");
const client_1 = require("react-dom/client");
const base_1 = require("../../../mol-plugin-ui/base.js");
const react_1 = require("react");
const markdown_1 = require("../../../mol-plugin-ui/controls/markdown.js");
class MVSStoriesSnapshotMarkdownModel extends component_1.PluginComponent {
    get viewer() {
        var _a;
        return (_a = this.context.state.viewers.value) === null || _a === void 0 ? void 0 : _a.find(v => { var _a; return ((_a = this.options) === null || _a === void 0 ? void 0 : _a.viewerName) === v.name; });
    }
    sync() {
        var _a, _b, _c;
        const mng = (_b = (_a = this.viewer) === null || _a === void 0 ? void 0 : _a.model.plugin) === null || _b === void 0 ? void 0 : _b.managers.snapshot;
        this.state.next({
            entry: mng === null || mng === void 0 ? void 0 : mng.current,
            index: (mng === null || mng === void 0 ? void 0 : mng.current) ? mng === null || mng === void 0 ? void 0 : mng.getIndex(mng.current) : undefined,
            all: (_c = mng === null || mng === void 0 ? void 0 : mng.state.entries.toArray()) !== null && _c !== void 0 ? _c : [],
        });
    }
    async mount(root) {
        this.root = root;
        (0, client_1.createRoot)(root).render((0, jsx_runtime_1.jsx)(MVSStoriesSnapshotMarkdownUI, { model: this }));
        let currentViewer = undefined;
        let sub = undefined;
        this.subscribe(this.context.state.viewers.pipe((0, rxjs_1.map)(xs => xs.find(v => { var _a; return ((_a = this.options) === null || _a === void 0 ? void 0 : _a.viewerName) === v.name; })), (0, rxjs_1.distinctUntilChanged)((a, b) => (a === null || a === void 0 ? void 0 : a.model) === (b === null || b === void 0 ? void 0 : b.model))), viewer => {
            var _a;
            if (currentViewer !== viewer) {
                currentViewer = viewer === null || viewer === void 0 ? void 0 : viewer.model;
                sub === null || sub === void 0 ? void 0 : sub.unsubscribe();
            }
            if (!viewer)
                return;
            sub = this.subscribe((_a = viewer.model.plugin) === null || _a === void 0 ? void 0 : _a.managers.snapshot.events.changed, () => {
                this.sync();
            });
            this.sync();
        });
        this.sync();
    }
    constructor(options) {
        super();
        this.options = options;
        this.root = undefined;
        this.state = new rxjs_1.BehaviorSubject({ all: [] });
        this.context = (0, context_1.getMVSStoriesContext)(options === null || options === void 0 ? void 0 : options.context);
    }
}
exports.MVSStoriesSnapshotMarkdownModel = MVSStoriesSnapshotMarkdownModel;
function Loading() {
    return (0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsx)("div", { style: { marginBottom: 16 }, children: (0, jsx_runtime_1.jsx)("i", { children: "Loading times may vary depending on the story size, your internet connection, and device performance" }) }), (0, jsx_runtime_1.jsxs)("div", { children: ["Fetching data", (0, jsx_runtime_1.jsx)(Dots, {})] }), (0, jsx_runtime_1.jsxs)("div", { children: ["Generating animations", (0, jsx_runtime_1.jsx)(Dots, {})] }), (0, jsx_runtime_1.jsxs)("div", { children: ["Preparing visuals", (0, jsx_runtime_1.jsx)(Dots, {})] })] });
}
function Dots() {
    const [dots, setDots] = (0, react_1.useState)(0);
    (0, react_1.useEffect)(() => {
        const interval = setInterval(() => {
            setDots(d => (d + 1) % 4);
        }, Math.random() * 500 + 300);
        return () => clearInterval(interval);
    }, []);
    return (0, jsx_runtime_1.jsx)("span", { children: '.'.repeat(dots) });
}
function MVSStoriesSnapshotMarkdownUI({ model }) {
    var _a, _b, _c;
    const state = (0, use_behavior_1.useBehavior)(model.state);
    const isLoading = (0, use_behavior_1.useBehavior)(model.context.state.isLoading);
    const style = { display: 'flex', flexDirection: 'column', height: '100%' };
    const className = 'mvs-stories-markdown-explanation';
    if (isLoading) {
        return (0, jsx_runtime_1.jsxs)("div", { style: style, className: className, children: [(0, jsx_runtime_1.jsx)("h3", { children: "The story will be ready momentarily" }), (0, jsx_runtime_1.jsx)(Loading, {})] });
    }
    if (state.all.length === 0) {
        return (0, jsx_runtime_1.jsx)("div", { style: style, className: className, children: (0, jsx_runtime_1.jsx)("i", { children: "No snapshot loaded or no description available" }) });
    }
    return (0, jsx_runtime_1.jsxs)("div", { style: style, className: className, children: [(0, jsx_runtime_1.jsxs)("div", { style: { display: 'flex', flexDirection: 'row', width: '100%', gap: '8px' }, children: [(0, jsx_runtime_1.jsxs)("span", { style: { lineHeight: '38px', minWidth: 60, maxWidth: 60, flexShrink: 0 }, children: [typeof state.index === 'number' ? state.index + 1 : '-', "/", state.all.length] }), (0, jsx_runtime_1.jsx)("button", { onClick: () => { var _a, _b; return (_b = (_a = model.viewer) === null || _a === void 0 ? void 0 : _a.model.plugin) === null || _b === void 0 ? void 0 : _b.managers.snapshot.applyNext(-1); }, style: { flexGrow: 1, flexShrink: 0 }, children: "Prev" }), (0, jsx_runtime_1.jsx)("button", { onClick: () => { var _a, _b; return (_b = (_a = model.viewer) === null || _a === void 0 ? void 0 : _a.model.plugin) === null || _b === void 0 ? void 0 : _b.managers.snapshot.applyNext(1); }, style: { flexGrow: 1, flexShrink: 0 }, children: "Next" })] }), (0, jsx_runtime_1.jsx)("div", { style: { flexGrow: 1, overflow: 'hidden', overflowY: 'auto', position: 'relative' }, children: (0, jsx_runtime_1.jsx)("div", { style: { position: 'absolute', inset: 0 }, children: (0, jsx_runtime_1.jsx)(base_1.PluginReactContext.Provider, { value: (_a = model.viewer) === null || _a === void 0 ? void 0 : _a.model.plugin, children: (0, jsx_runtime_1.jsx)(markdown_1.Markdown, { children: (_c = (_b = state.entry) === null || _b === void 0 ? void 0 : _b.description) !== null && _c !== void 0 ? _c : 'Description not available' }) }) }) })] });
}
class MVSStoriesSnapshotMarkdownViewer extends HTMLElement {
    async connectedCallback() {
        var _a, _b;
        this.model = new MVSStoriesSnapshotMarkdownModel({
            context: { name: (_a = this.getAttribute('context-name')) !== null && _a !== void 0 ? _a : undefined },
            viewerName: (_b = this.getAttribute('viewer-name')) !== null && _b !== void 0 ? _b : undefined,
        });
        await this.model.mount(this);
    }
    disconnectedCallback() {
        var _a;
        (_a = this.model) === null || _a === void 0 ? void 0 : _a.dispose();
        this.model = undefined;
    }
    constructor() {
        super();
        this.model = undefined;
    }
}
exports.MVSStoriesSnapshotMarkdownViewer = MVSStoriesSnapshotMarkdownViewer;
window.customElements.define('mvs-stories-snapshot-markdown', MVSStoriesSnapshotMarkdownViewer);
