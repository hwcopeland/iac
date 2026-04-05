/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { Components } from 'react-markdown';
import { MarkdownExtension } from '../../mol-plugin-state/manager/markdown-extensions.js';
export declare function Markdown({ children, components }: {
    children?: string;
    components?: Components;
}): import("react/jsx-runtime").JSX.Element;
export declare function MarkdownAudioPlayer(): import("react/jsx-runtime").JSX.Element | null;
export declare function MarkdownImg({ src, element, alt }: {
    src?: string;
    element?: any;
    alt?: string;
}): any;
export declare const DefaultMarkdownExtensionRenderers: MarkdownExtension[];
export declare function MarkdownAnchor({ href, children, element }: {
    href?: string;
    children?: any;
    element?: any;
}): any;
