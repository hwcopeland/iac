/**
 * Copyright (c) 2018-2023 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author Christian Dominguez <christian.99dominguez@gmail.com>
 * @author Ventura Rivera <venturaxrivera@gmail.com>
 */
import { List } from 'immutable';
import * as React from 'react';
import { LogEntry } from '../mol-util/log-entry.js';
import { PluginUIComponent } from './base.js';
import { PluginUIContext } from './context.js';
export declare function Plugin({ plugin }: {
    plugin: PluginUIContext;
}): import("react/jsx-runtime").JSX.Element;
export declare class PluginContextContainer extends React.Component<{
    plugin: PluginUIContext;
    children?: any;
}> {
    render(): import("react/jsx-runtime").JSX.Element;
}
type RegionKind = 'top' | 'left' | 'right' | 'bottom' | 'main';
export declare class Layout extends PluginUIComponent {
    componentDidMount(): void;
    region(kind: RegionKind, Element?: React.ComponentClass | React.FC): import("react/jsx-runtime").JSX.Element;
    get layoutVisibilityClassName(): string;
    get layoutClassName(): string;
    onDrop: (ev: React.DragEvent<HTMLDivElement>) => void;
    onDragOver: (ev: React.DragEvent<HTMLDivElement>) => void;
    private showDragOverlay;
    onDragEnter: (ev: React.DragEvent<HTMLDivElement>) => void;
    render(): import("react/jsx-runtime").JSX.Element;
}
export declare class ControlsWrapper extends PluginUIComponent {
    render(): import("react/jsx-runtime").JSX.Element;
}
export declare class DefaultViewport extends PluginUIComponent {
    render(): import("react/jsx-runtime").JSX.Element;
}
export declare class Log extends PluginUIComponent<{}, {
    entries: List<LogEntry>;
}> {
    private wrapper;
    componentDidMount(): void;
    componentDidUpdate(): void;
    state: {
        entries: List<LogEntry>;
    };
    private scrollToBottom;
    render(): import("react/jsx-runtime").JSX.Element;
}
export {};
