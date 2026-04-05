import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { useContext, useEffect, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { PluginReactContext } from '../base.js';
import { ColorLists } from '../../mol-util/color/lists.js';
import { getColorGradient, getColorGradientBanded, parseColorList } from '../../mol-util/color/utils.js';
import { useBehavior } from '../hooks/use-behavior.js';
export function Markdown({ children, components }) {
    return _jsxs("div", { className: 'msp-markdown', children: [_jsx(MarkdownAudioPlayer, {}), _jsx(ReactMarkdown, { skipHtml: true, components: { a: MarkdownAnchor, img: MarkdownImg, ...components }, remarkPlugins: [remarkGfm], children: children })] });
}
export function MarkdownAudioPlayer() {
    const parent = useRef(null);
    const plugin = useContext(PluginReactContext);
    const audio = useBehavior(plugin === null || plugin === void 0 ? void 0 : plugin.managers.markdownExtensions.state.audioPlayer);
    useEffect(() => {
        if (!parent.current)
            return;
        parent.current.appendChild(audio);
        return () => { audio === null || audio === void 0 ? void 0 : audio.remove(); };
    }, [audio]);
    if (!audio)
        return null;
    return _jsx("div", { className: 'msp-markdown-audio-player', ref: parent });
}
export function MarkdownImg({ src, element, alt }) {
    const plugin = useContext(PluginReactContext);
    if (!src)
        return element;
    warnMissingPlugin(plugin);
    const args = plugin === null || plugin === void 0 ? void 0 : plugin.managers.markdownExtensions.parseArgs(src);
    if (args) {
        const result = plugin === null || plugin === void 0 ? void 0 : plugin.managers.markdownExtensions.tryRender(args, DefaultMarkdownExtensionRenderers);
        return result !== null && result !== void 0 ? result : element;
    }
    else {
        const data = plugin === null || plugin === void 0 ? void 0 : plugin.managers.markdownExtensions.tryResolveUri(src);
        if (typeof (data === null || data === void 0 ? void 0 : data.then) === 'function') {
            return _jsx(LazyStaticImg, { alt: alt, data: data });
        }
        else if (typeof data === 'string' && data) {
            return _jsx("img", { src: data, alt: alt });
        }
    }
    return _jsx("img", { src: src, alt: alt });
}
function LazyStaticImg({ alt, data }) {
    const [src, setSrc] = useState(undefined);
    useEffect(() => {
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
    return _jsx("img", { src: src, alt: alt });
}
export const DefaultMarkdownExtensionRenderers = [
    {
        name: 'color-swatch',
        reactRenderFn: ({ args }) => {
            const color = args['color-swatch'];
            if (!color)
                return null;
            return _jsx("span", { style: { display: 'inline-block', width: '0.75em', height: '0.75em', backgroundColor: color, borderRadius: '25%' } });
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
                ? parseColorList(colors)
                : (_c = ColorLists[name.toLowerCase()]) === null || _c === void 0 ? void 0 : _c.list;
            if (!(list === null || list === void 0 ? void 0 : list.length)) {
                console.warn(`Color palette could not be resolved.`, args);
                return null;
            }
            return _jsx("span", { style: {
                    display: 'inline-block',
                    minWidth,
                    height,
                    background: (discrete ? getColorGradientBanded : getColorGradient)(list),
                    borderRadius: '2px'
                } });
        }
    }
];
export function MarkdownAnchor({ href, children, element }) {
    const plugin = useContext(PluginReactContext);
    if (!href)
        return element;
    warnMissingPlugin(plugin);
    const args = plugin === null || plugin === void 0 ? void 0 : plugin.managers.markdownExtensions.parseArgs(href);
    if (args) {
        return _jsx("a", { href: '#', onClick: (e) => {
                e.preventDefault();
                plugin === null || plugin === void 0 ? void 0 : plugin.managers.markdownExtensions.tryExecute('click', args);
            }, onMouseEnter: () => plugin === null || plugin === void 0 ? void 0 : plugin.managers.markdownExtensions.tryExecute('mouse-enter', args), onMouseLeave: () => plugin === null || plugin === void 0 ? void 0 : plugin.managers.markdownExtensions.tryExecute('mouse-leave', args), children: children });
    }
    else if (href[0] === '#') {
        warnMissingPlugin(plugin);
        return _jsx("a", { href: '#', onClick: (e) => {
                e.preventDefault();
                plugin === null || plugin === void 0 ? void 0 : plugin.managers.snapshot.applyKey(href.substring(1));
            }, children: children });
    }
    else if (href) {
        return _jsxs("a", { href: href, target: '_blank', rel: 'noopener noreferrer', children: [children, "\u2934"] });
    }
    return children;
}
function warnMissingPlugin(plugin) {
    if (plugin)
        return;
    console.warn('Markdown component requires a PluginReactContext to be set.');
}
