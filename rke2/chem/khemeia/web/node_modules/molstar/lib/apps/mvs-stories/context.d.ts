/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { BehaviorSubject } from 'rxjs';
import { MVSData } from '../../extensions/mvs/mvs-data.js';
import type { MVSStoriesViewerModel } from './elements/viewer.js';
export type MVSStoriesCommand = {
    kind: 'load-mvs';
    format?: 'mvsj' | 'mvsx';
    url?: string;
    data?: MVSData | string | Uint8Array<ArrayBuffer>;
};
export declare class MVSStoriesContext {
    name?: string | undefined;
    commands: BehaviorSubject<{
        kind: "load-mvs";
        format?: "mvsj" | "mvsx";
        url?: string;
        data?: MVSData | string | Uint8Array<ArrayBuffer>;
    } | undefined>;
    state: {
        viewers: BehaviorSubject<{
            name?: string;
            model: MVSStoriesViewerModel;
        }[]>;
        currentStoryData: BehaviorSubject<string | Uint8Array<ArrayBuffer> | undefined>;
        isLoading: BehaviorSubject<boolean>;
    };
    dispatch(command: MVSStoriesCommand): void;
    constructor(name?: string | undefined);
}
export declare function getMVSStoriesContext(options?: {
    name?: string;
    container?: object;
}): MVSStoriesContext;
