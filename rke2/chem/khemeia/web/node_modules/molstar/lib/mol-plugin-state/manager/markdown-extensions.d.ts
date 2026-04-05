/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { StateObjectCell } from '../../mol-state/index.js';
import { PluginContext } from '../../mol-plugin/context.js';
import { BehaviorSubject } from 'rxjs';
export type MarkdownExtensionEvent = 'click' | 'mouse-enter' | 'mouse-leave';
export interface MarkdownExtension {
    name: string;
    execute?: (options: {
        event: MarkdownExtensionEvent;
        args: Record<string, string>;
        manager: MarkdownExtensionManager;
    }) => void;
    reactRenderFn?: (options: {
        args: Record<string, string>;
        manager: MarkdownExtensionManager;
    }) => any;
}
export declare const BuiltInMarkdownExtension: MarkdownExtension[];
export declare class MarkdownExtensionManager {
    plugin: PluginContext;
    state: {
        audioPlayer: BehaviorSubject<HTMLAudioElement | null>;
    };
    private extension;
    private refResolvers;
    private uriResolvers;
    private argsParsers;
    /**
     * Default parser has priority 100, parsers with higher priority
     * will be called first.
     */
    registerArgsParser(name: string, priority: number, parser: (input: string | undefined) => Record<string, string> | undefined): void;
    removeArgsParser(name: string): void;
    parseArgs(input: string | undefined): Record<string, string> | undefined;
    registerRefResolver(name: string, resolver: (plugin: PluginContext, refs: string[]) => StateObjectCell[]): void;
    removeRefResolver(name: string): void;
    registerUriResolver(name: string, resolver: (plugin: PluginContext, uri: string) => Promise<string> | string | undefined): void;
    removeUriResolver(name: string): void;
    registerExtension(command: MarkdownExtension): void;
    removeExtension(name: string): void;
    private _tryRender;
    /**
     * Render a markdown extension with the given arguments.
     * Default renderers are defined separately because we
     * don't want to include `react` outside of mol-plugin-ui.
     */
    tryRender(args: Record<string, string>, defaultRenderers: MarkdownExtension[]): any;
    tryExecute(event: MarkdownExtensionEvent, args: Record<string, string>): void;
    tryResolveUri(uri: string): Promise<string> | string | undefined;
    findCells(refs: string[]): StateObjectCell[];
    private resolveAudioPlayer;
    get audioPlayer(): HTMLAudioElement | null;
    audio: {
        play: (src: string, options?: {
            toggle?: boolean;
        }) => Promise<void>;
        pause: () => void;
        stop: () => void;
        dispose: () => void;
    };
    constructor(plugin: PluginContext);
}
export declare function defaultParseMarkdownCommandArgs(input: string | undefined): Record<string, string> | undefined;
