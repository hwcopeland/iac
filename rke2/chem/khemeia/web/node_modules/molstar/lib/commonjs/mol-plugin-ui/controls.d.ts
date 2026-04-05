/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import * as React from 'react';
import { LociLabel } from '../mol-plugin-state/manager/loci-label.js';
import { PluginUIComponent } from './base.js';
export declare class TrajectoryViewportControls extends PluginUIComponent<{}, {
    show: boolean;
    label: string;
}> {
    state: {
        show: boolean;
        label: string;
    };
    private update;
    componentDidMount(): void;
    reset: () => Promise<void>;
    prev: () => Promise<void>;
    next: () => Promise<void>;
    render(): import("react/jsx-runtime").JSX.Element | null;
}
export declare class StateSnapshotViewportControls extends PluginUIComponent<{}, {
    isBusy: boolean;
    show: boolean;
    showAnimation: boolean;
}> {
    state: {
        isBusy: boolean;
        show: boolean;
        showAnimation: boolean;
    };
    componentDidMount(): void;
    componentWillUnmount(): void;
    update(id: string): Promise<void>;
    change: (e: React.ChangeEvent<HTMLSelectElement>) => void;
    prev: () => void;
    next: () => void;
    togglePlay: () => void;
    toggleShowAnimation: () => void;
    toggleStateAnimation: () => void;
    get isStateTransitionPlaying(): boolean;
    render(): import("react/jsx-runtime").JSX.Element | null;
}
export declare function ViewportSnapshotDescription(): import("react/jsx-runtime").JSX.Element | null;
export declare class AnimationViewportControls extends PluginUIComponent<{}, {
    isEmpty: boolean;
    isExpanded: boolean;
    isBusy: boolean;
    isAnimating: boolean;
    isPlaying: boolean;
}> {
    state: {
        isEmpty: boolean;
        isExpanded: boolean;
        isBusy: boolean;
        isAnimating: boolean;
        isPlaying: boolean;
    };
    componentDidMount(): void;
    toggleExpanded: () => void;
    stop: () => void;
    render(): import("react/jsx-runtime").JSX.Element | null;
}
export declare class SelectionViewportControls extends PluginUIComponent {
    componentDidMount(): void;
    render(): import("react/jsx-runtime").JSX.Element | null;
}
export declare class LociLabels extends PluginUIComponent<{}, {
    labels: ReadonlyArray<LociLabel>;
}> {
    state: {
        labels: string[];
    };
    componentDidMount(): void;
    render(): import("react/jsx-runtime").JSX.Element | null;
}
export declare class CustomStructureControls extends PluginUIComponent<{
    initiallyCollapsed?: boolean;
}> {
    componentDidMount(): void;
    render(): import("react/jsx-runtime").JSX.Element | null;
}
export declare class DefaultStructureTools extends PluginUIComponent {
    render(): import("react/jsx-runtime").JSX.Element;
}
