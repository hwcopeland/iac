/**
 * Copyright (c) 2020-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { DrawPass } from './draw.js';
import { PickPass } from './pick.js';
import { MultiSamplePass } from './multi-sample.js';
import { WebGLContext } from '../../mol-gl/webgl/context.js';
import { AssetManager } from '../../mol-util/assets.js';
import { IlluminationPass } from './illumination.js';
export declare class Passes {
    private webgl;
    readonly draw: DrawPass;
    readonly pick: PickPass;
    readonly multiSample: MultiSamplePass;
    readonly illumination: IlluminationPass;
    constructor(webgl: WebGLContext, assetManager: AssetManager, attribs?: Partial<{
        pickScale: number;
        transparency: 'wboit' | 'dpoit' | 'blended';
    }>);
    getByteCount(): number;
    setPickScale(pickScale: number): void;
    setTransparency(transparency: 'wboit' | 'dpoit' | 'blended'): void;
    updateSize(): void;
}
