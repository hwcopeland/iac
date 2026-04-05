/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import './elements/index.js';
import { MVSData } from '../../extensions/mvs/mvs-data.js';
import './favicon.ico';
import '../../mol-plugin-ui/skin/light.scss';
import './styles.scss';
import './index.html';
export declare function getContext(name?: string): import("./context.js").MVSStoriesContext;
export declare function loadFromURL(url: string, options?: {
    format: 'mvsx' | 'mvsj';
    contextName?: string;
}): void;
export declare function loadFromData(data: MVSData | string | Uint8Array<ArrayBuffer>, options?: {
    format: 'mvsx' | 'mvsj';
    contextName?: string;
}): void;
export declare function loadFromID(id: string, options?: {
    format?: 'mvsx' | 'mvsj';
    contextName?: string;
}): void;
export declare function downloadCurrentStory(options?: {
    contextName?: string;
    filename?: string;
}): void;
export { MVSData };
