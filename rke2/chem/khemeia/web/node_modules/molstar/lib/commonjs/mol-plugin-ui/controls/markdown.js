"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.DefaultMarkdownExtensionRenderers = void 0;
exports.Markdown = Markdown;
exports.MarkdownAudioPlayer = MarkdownAudioPlayer;
exports.MarkdownImg = MarkdownImg;
exports.MarkdownAnchor = MarkdownAnchor;
const tslib_1 = require("tslib");
const jsx_runtime_1 = require("react/jsx-runtime");
/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
const react_1 = require("react");
const react_markdown_1 = tslib_1.__importDefault(require("react-markdown"));
const remark_gfm_1 = tslib_1.__importDefault(require("remark-gfm"));
const base_1 = require("../base.js");
const lists_1 = require("../../mol-util/color/lists.js");
const utils_1 = require("../../mol-util/color/utils.js");
const use_behavior_1 = require("../hooks/use-behavior.js");
function Markdown({ children, components }) {
    return (0, jsx_runtime_1.jsxs)("div", { className: 'msp-markdown', children: [(0, jsx_runtime_1.jsx)(MarkdownAudioPlayer, {}), (0, jsx_runtime_1.jsx)(react_markdown_1.default, { skipHtml: true, components: { a: MarkdownAnchor, img: MarkdownImg, ...components }, remarkPlugins: [remark_gfm_1.default], children: children })] });
}
function MarkdownAudioPlayer() {
    const parent = (0, react_1.useRef)(null);
    const plugin = (0, react_1.useContext)(base_1.PluginReactContext);
    const audio = (0, use_behavior_1.useBehavior)(plugin === null || plugin === void 0 ? void 0 : plugin.managers.markdownExtensions.state.audioPlayer);
    (0, react_1.useEffect)(() => {
        if (!parent.current)
            return;
        parent.current.appendChild(audio);
        return () => { audio === null || audio === void 0 ? void 0 : audio.remove(); };
    }, [audio]);
    if (!audio)
        return null;
    return (0, jsx_runtime_1.jsx)("div", { className: 'msp-markdown-audio-player', ref: parent });
}
function MarkdownImg({ src, element, alt }) {
    const plugin = (0, react_1.useContext)(base_1.PluginReactContext);
    if (!src)
        return element;
    warnMissingPlugin(plugin);
    const args = plugin === null || plugin === void 0 ? void 0 : plugin.managers.markdownExtensions.parseArgs(src);
    if (args) {
        const result = plugin === null || plugin === void 0 ? void 0 : plugin.managers.markdownExtensions.tryRender(args, exports.DefaultMarkdownExtensionRenderers);
        return result !== null && result !== void 0 ? result : element;
    }
    else {
        const data = plugin === null || plugin === void 0 ? void 0 : plugin.managers.markdownExtensions.tryResolveUri(src);
        if (typeof (data === null || data === void 0 ? void 0 : data.then) === 'function') {
            return (0, jsx_runtime_1.jsx)(LazyStaticImg, { alt: alt, data: data });
        }
        else if (typeof data === 'string' && data) {
            return (0, jsx_runtime_1.jsx)("img", { src: data, alt: alt });
        }
    }
    return (0, jsx_runtime_1.jsx)("img", { src: src, alt: alt });
}
function LazyStaticImg({ alt, data }) {
    const [src, setSrc] = (0, react_1.useState)(undefined);
    (0, react_1.useEffect)(() => {
        let mounted = true;
        data.then(d => {
            if (mounted)
                setSrc(d);
        }).catch(e => {
            console.error('Failed to load static image', e);
            if (mounted)
                setSrc(undefined);
        });
        return () => { mounted = false; };
    }, [data]);
    if (!src)
        return null;
    return (0, jsx_runtime_1.jsx)("img", { src: src, alt: alt });
}
exports.DefaultMarkdownExtensionRenderers = [
    {
        name: 'color-swatch',
        reactRenderFn: ({ args }) => {
            const color = args['color-swatch'];
            if (!color)
                return null;
            return (0, jsx_runtime_1.jsx)("span", { style: { display: 'inline-block', width: '0.75em', height: '0.75em', backgroundColor: color, borderRadius: '25%' } });
        }
    },
    {
        name: 'color-palette',
        reactRenderFn: ({ args }) => {
            var _a, _b, _c;
            const name = args['color-palette-name'];
            const colors = args['color-palette-colors'];
            const minWidth = (_a = args['color-palette-width']) !== null && _a !== void 0 ? _a : '150px';
            const height = (_b = args['color-palette-height']) !== null && _b !== void 0 ? _b : '0.5em';
            const discrete = 'color-palette-discrete' in args;
            if (!name && !colors)
                return null;
            const list = colors
                ? (0, utils_1.parseColorList)(colors)
                : (_c = lists_1.ColorLists[name.toLowerCase()]) === null || _c === void 0 ? void 0 : _c.list;
            if (!(list === null || list === void 0 ? void 0 : list.length)) {
                console.warn(`Color palette could not be resolved.`, args);
                return null;
            }
            return (0, jsx_runtime_1.jsx)("span", { style: {
                    display: 'inline-block',
                    minWidth,
                    height,
                    background: (discrete ? utils_1.getColorGradientBanded : utils_1.getColorGradient)(list),
                    borderRadius: '2px'
                } });
        }
    }
];
function MarkdownAnchor({ href, children, element }) {
    const plugin = (0, react_1.useContext)(base_1.PluginReactContext);
    if (!href)
        return element;
    warnMissingPlugin(plugin);
    const args = plugin === null || plugin === void 0 ? void 0 : plugin.managers.markdownExtensions.parseArgs(href);
    if (args) {
        return (0, jsx_runtime_1.jsx)("a", { href: '#', onClick: (e) => {
                e.preventDefault();
                plugin === null || plugin === void 0 ? void 0 : plugin.managers.markdownExtensions.tryExecute('click', args);
            }, onMouseEnter: () => plugin === null || plugin === void 0 ? void 0 : plugin.managers.markdownExtensions.tryExecute('mouse-enter', args), onMouseLeave: () => plugin === null || plugin === void 0 ? void 0 : plugin.managers.markdownExtensions.tryExecute('mouse-leave', args), children: children });
    }
    else if (href[0] === '#') {
        warnMissingPlugin(plugin);
        return (0, jsx_runtime_1.jsx)("a", { href: '#', onClick: (e) => {
                e.preventDefault();
                plugin === null || plugin === void 0 ? void 0 : plugin.managers.snapshot.applyKey(href.substring(1));
            }, children: children });
    }
    else if (href) {
        return (0, jsx_runtime_1.jsxs)("a", { href: href, target: '_blank', rel: 'noopener noreferrer', children: [children, "\u2934"] });
    }
    return children;
}
function warnMissingPlugin(plugin) {
    if (plugin)
        return;
    console.warn('Markdown component requires a PluginReactContext to be set.');
}
