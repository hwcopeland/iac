/**
 * Copyright (c) 2024-25 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
export declare class PluginContainer {
    options?: {
        checkeredCanvasBackground?: boolean;
        canvas?: HTMLCanvasElement;
    } | undefined;
    readonly parent: HTMLDivElement;
    readonly canvas: HTMLCanvasElement;
    mount(target: HTMLElement): void;
    unmount(): void;
    /**
     * options.checkeredCanvasBackground has no effect. Use canvas3d.checkeredTransparentBackground instead.
     * TODO: remove in v6
     */
    constructor(options?: {
        checkeredCanvasBackground?: boolean;
        canvas?: HTMLCanvasElement;
    } | undefined);
}
