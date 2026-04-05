/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { getCellBoundingSphere } from '../../mol-plugin-state/manager/focus-camera/focus-object.js';
import { PluginStateObject } from '../../mol-plugin-state/objects.js';
import { StateSelection } from '../../mol-state/index.js';
import { Script } from '../../mol-script/script.js';
import { QueryContext, StructureElement, StructureSelection } from '../../mol-model/structure.js';
import { BehaviorSubject } from 'rxjs';
import { AnimateStateSnapshotTransition } from '../animation/built-in/state-snapshots.js';
export const BuiltInMarkdownExtension = [
    {
        name: 'center-camera',
        execute: ({ event, args, manager }) => {
            if (event !== 'click')
                return;
            if ('center-camera' in args) {
                manager.plugin.managers.camera.reset();
            }
        }
    },
    {
        name: 'apply-snapshot',
        execute: ({ event, args, manager }) => {
            if (event !== 'click')
                return;
            const key = args['apply-snapshot'];
            if (!key)
                return;
            manager.plugin.managers.snapshot.applyKey(key);
        }
    },
    {
        name: 'next-snapshot',
        execute: ({ event, args, manager }) => {
            if (event !== 'click' || !('next-snapshot' in args))
                return;
            let dir = (+args['next-snapshot'] || 1);
            if (!dir)
                return;
            if (dir < 0)
                dir = -1;
            else
                dir = 1;
            manager.plugin.managers.snapshot.applyNext(dir);
        }
    },
    {
        name: 'focus-refs',
        execute: ({ event, args, manager }) => {
            if (event !== 'click')
                return;
            const refs = parseArray(args['focus-refs']);
            if (!(refs === null || refs === void 0 ? void 0 : refs.length))
                return;
            const cells = manager.findCells(refs);
            if (!cells.length)
                return;
            const reprs = findRepresentations(manager.plugin, cells);
            if (!reprs.length)
                return;
            const spheres = reprs.map(c => getCellBoundingSphere(manager.plugin, c.transform.ref)).filter(s => !!s);
            if (!spheres.length)
                return;
            manager.plugin.managers.camera.focusSpheres(spheres, s => s, { extraRadius: 3 });
        }
    },
    {
        name: 'highlight-refs',
        execute: ({ event, args, manager }) => {
            var _a;
            const refs = parseArray(args['highlight-refs']);
            if (!(refs === null || refs === void 0 ? void 0 : refs.length))
                return;
            if (event === 'mouse-leave' && refs.length) {
                manager.plugin.managers.interactivity.lociHighlights.clearHighlights();
                return;
            }
            else if (event === 'mouse-enter') {
                const cells = manager.findCells(refs);
                for (const cell of findRepresentations(manager.plugin, cells)) {
                    if (!((_a = cell.obj) === null || _a === void 0 ? void 0 : _a.data))
                        continue;
                    const { repr } = cell.obj.data;
                    for (const loci of repr.getAllLoci()) {
                        manager.plugin.managers.interactivity.lociHighlights.highlight({ loci, repr }, false);
                    }
                }
            }
        }
    },
    {
        name: 'query',
        execute: ({ event, args, manager }) => {
            var _a;
            const expression = args['query'];
            if (!(expression === null || expression === void 0 ? void 0 : expression.length))
                return;
            // supported languages: mol-script, pymol, vmd, jmol
            const language = args['lang'] || 'mol-script';
            // supported actions: highlight, focus
            const action = parseArray(args['action'] || 'highlight');
            const focusRadius = parseFloat(args['focus-radius'] || '3');
            if (event === 'mouse-leave') {
                if (action.includes('highlight')) {
                    manager.plugin.managers.interactivity.lociHighlights.clearHighlights();
                }
                return;
            }
            let query;
            try {
                query = Script.toQuery({
                    language: language,
                    expression
                });
            }
            catch (e) {
                console.warn(`Failed to parse query '${expression}' (${language})`, e);
                return;
            }
            const structures = manager.plugin.state.data.selectQ(q => q.rootsOfType(PluginStateObject.Molecule.Structure));
            if (event === 'mouse-enter') {
                if (!action.includes('focus')) {
                    return;
                }
                manager.plugin.managers.interactivity.lociHighlights.clearHighlights();
                for (const structure of structures) {
                    if (!((_a = structure.obj) === null || _a === void 0 ? void 0 : _a.data))
                        continue;
                    const selection = query(new QueryContext(structure.obj.data));
                    const loci = StructureSelection.toLociWithSourceUnits(selection);
                    manager.plugin.managers.interactivity.lociHighlights.highlight({
                        loci,
                    }, false);
                }
            }
            if (event === 'click') {
                if (!action.includes('focus')) {
                    return;
                }
                const decorated = structures.map(s => StateSelection.getDecorated(manager.plugin.state.data, s.transform.ref));
                const spheres = decorated.map(s => {
                    var _a;
                    if (!((_a = s.obj) === null || _a === void 0 ? void 0 : _a.data))
                        return undefined;
                    const selection = query(new QueryContext(s.obj.data));
                    if (StructureSelection.isEmpty(selection))
                        return;
                    const loci = StructureSelection.toLociWithSourceUnits(selection);
                    return StructureElement.Loci.getBoundary(loci).sphere;
                }).filter(s => !!s);
                if (spheres.length) {
                    manager.plugin.managers.camera.focusSpheres(spheres, s => s, { extraRadius: focusRadius });
                }
            }
        },
    },
    {
        name: 'play-audio',
        execute: ({ event, args, manager }) => {
            if (event !== 'click')
                return;
            const src = args['play-audio'];
            if (!(src === null || src === void 0 ? void 0 : src.length))
                return;
            manager.audio.play(src);
        }
    },
    {
        name: 'toggle-audio',
        execute: ({ event, args, manager }) => {
            if (event !== 'click' || !('toggle-audio' in args))
                return;
            const src = args['toggle-audio'];
            manager.audio.play(src, { toggle: true });
        }
    },
    {
        name: 'pause-audio',
        execute: ({ event, args, manager }) => {
            if (event !== 'click' || !('pause-audio' in args))
                return;
            manager.audio.pause();
        }
    },
    {
        name: 'stop-audio',
        execute: ({ event, args, manager }) => {
            if (event !== 'click' || !('stop-audio' in args))
                return;
            manager.audio.stop();
        }
    },
    {
        name: 'dispose-audio',
        execute: ({ event, args, manager }) => {
            if (event !== 'click' || !('dispose-audio' in args))
                return;
            manager.audio.dispose();
        }
    },
    {
        name: 'play-transition',
        execute: ({ event, args, manager }) => {
            if (event !== 'click' || !('play-transition' in args))
                return;
            manager.plugin.managers.animation.play(AnimateStateSnapshotTransition, {});
        }
    },
    {
        name: 'play-snapshots',
        execute: ({ event, args, manager }) => {
            if (event !== 'click' || !('play-snapshots' in args))
                return;
            manager.plugin.managers.snapshot.play({ restart: true });
        }
    },
    {
        name: 'stop-animation',
        execute: ({ event, args, manager }) => {
            if (event !== 'click' || !('stop-animation' in args))
                return;
            manager.plugin.managers.snapshot.stop();
        }
    },
];
export class MarkdownExtensionManager {
    /**
     * Default parser has priority 100, parsers with higher priority
     * will be called first.
     */
    registerArgsParser(name, priority, parser) {
        this.removeArgsParser(name);
        this.argsParsers.push([name, priority, parser]);
        this.argsParsers.sort((a, b) => b[1] - a[1]); // Sort by priority, higher first
    }
    removeArgsParser(name) {
        const idx = this.argsParsers.findIndex(p => p[0] === name);
        if (idx >= 0) {
            this.argsParsers.splice(idx, 1);
        }
    }
    parseArgs(input) {
        for (const [, , parser] of this.argsParsers) {
            const ret = parser(input);
            if (ret)
                return ret;
        }
        return undefined;
    }
    registerRefResolver(name, resolver) {
        this.refResolvers[name] = resolver;
    }
    removeRefResolver(name) {
        delete this.refResolvers[name];
    }
    registerUriResolver(name, resolver) {
        this.uriResolvers[name] = resolver;
    }
    removeUriResolver(name) {
        delete this.uriResolvers[name];
    }
    registerExtension(command) {
        const existing = this.extension.findIndex(c => c.name === command.name);
        if (existing >= 0) {
            this.extension[existing] = command;
        }
        else {
            this.extension.push(command);
        }
    }
    removeExtension(name) {
        const idx = this.extension.findIndex(c => c.name === name);
        if (idx >= 0) {
            this.extension.splice(idx, 1);
        }
    }
    _tryRender(ext, options) {
        var _a;
        try {
            return (_a = ext.reactRenderFn) === null || _a === void 0 ? void 0 : _a.call(ext, options);
        }
        catch (e) {
            console.error(`Failed to render markdown extension '${ext.name}'`, e);
            return null;
        }
    }
    /**
     * Render a markdown extension with the given arguments.
     * Default renderers are defined separately because we
     * don't want to include `react` outside of mol-plugin-ui.
     */
    tryRender(args, defaultRenderers) {
        const options = { args, manager: this };
        for (const ext of this.extension) {
            const ret = this._tryRender(ext, options);
            if (ret) {
                return ret;
            }
        }
        for (const ext of defaultRenderers) {
            const ret = this._tryRender(ext, options);
            if (ret) {
                return ret;
            }
        }
        return null;
    }
    tryExecute(event, args) {
        var _a;
        const options = { event, args, manager: this };
        for (const ext of this.extension) {
            try {
                (_a = ext.execute) === null || _a === void 0 ? void 0 : _a.call(ext, options);
            }
            catch (e) {
                console.error(`Failed to execute markdown extension '${ext.name}'`, e);
            }
        }
    }
    tryResolveUri(uri) {
        for (const resolver of Object.values(this.uriResolvers)) {
            const resolved = resolver(this.plugin, uri);
            if (resolved) {
                return resolved;
            }
        }
    }
    findCells(refs) {
        const added = new Set();
        const ret = [];
        for (const resolver of Object.values(this.refResolvers)) {
            for (const cell of resolver(this.plugin, refs)) {
                if (cell && !added.has(cell.transform.ref)) {
                    added.add(cell.transform.ref);
                    ret.push(cell);
                }
            }
        }
        return ret;
    }
    resolveAudioPlayer() {
        if (this.state.audioPlayer.value) {
            return this.state.audioPlayer.value;
        }
        const audio = document.createElement('audio');
        audio.controls = true;
        audio.preload = 'auto';
        audio.style.width = '100%';
        audio.style.height = '32px';
        this.state.audioPlayer.next(audio);
        return audio;
    }
    get audioPlayer() {
        return this.state.audioPlayer.value;
    }
    constructor(plugin) {
        this.plugin = plugin;
        this.state = {
            audioPlayer: new BehaviorSubject(null),
        };
        this.extension = [];
        this.refResolvers = {
            default: (plugin, refs) => refs
                .map(ref => plugin.state.data.cells.get(ref))
                .filter(c => !!c),
        };
        this.uriResolvers = {};
        this.argsParsers = [
            ['default', 100, defaultParseMarkdownCommandArgs],
        ];
        this.audio = {
            play: async (src, options) => {
                try {
                    const audio = this.resolveAudioPlayer();
                    let newSource = false;
                    if (src === null || src === void 0 ? void 0 : src.trim()) {
                        const resolved = this.tryResolveUri(src);
                        let uri = src;
                        if (typeof (resolved === null || resolved === void 0 ? void 0 : resolved.then) === 'function') {
                            uri = (await resolved);
                        }
                        else if (resolved) {
                            uri = resolved;
                        }
                        newSource = audio.src !== uri;
                        if (newSource) {
                            audio.src = uri;
                            audio.load();
                        }
                    }
                    if (!newSource && (options === null || options === void 0 ? void 0 : options.toggle)) {
                        if (audio.paused) {
                            await audio.play();
                        }
                        else {
                            audio.pause();
                        }
                    }
                    else {
                        audio.currentTime = 0;
                        await audio.play();
                    }
                }
                catch (e) {
                    console.error('Failed to play audio', e);
                }
            },
            pause: () => {
                var _a;
                (_a = this.audioPlayer) === null || _a === void 0 ? void 0 : _a.pause();
            },
            stop: () => {
                if (!this.audioPlayer)
                    return;
                this.audioPlayer.pause();
                this.audioPlayer.currentTime = 0;
            },
            dispose: () => {
                if (this.audioPlayer) {
                    this.audioPlayer.pause();
                    this.audioPlayer.currentTime = 0;
                    this.state.audioPlayer.next(null);
                }
            }
        };
        for (const command of BuiltInMarkdownExtension) {
            this.registerExtension(command);
        }
    }
}
function parseArray(input) {
    var _a;
    return (_a = input === null || input === void 0 ? void 0 : input.split(',').map(s => s.trim()).filter(s => s.length > 0)) !== null && _a !== void 0 ? _a : [];
}
function findRepresentations(plugin, cells) {
    if (!cells.length)
        return [];
    return plugin.state.data.selectQ(q => q.byValue(...cells).subtree().filter(c => PluginStateObject.isRepresentation3D(c.obj)));
}
export function defaultParseMarkdownCommandArgs(input) {
    if (!(input === null || input === void 0 ? void 0 : input.startsWith('!')))
        return undefined;
    const entries = decodeURIComponent(input.substring(1))
        .split('&')
        .map(p => p.trim())
        .filter(p => p.length > 0)
        .map(p => p.split('=', 2).map(s => s.trim()));
    if (entries.length === 0)
        return undefined;
    return Object.fromEntries(entries);
}
