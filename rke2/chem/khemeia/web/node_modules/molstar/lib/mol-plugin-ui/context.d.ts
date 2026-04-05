/**
 * Copyright (c) 2021 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { PluginContext } from '../mol-plugin/context.js';
import { PluginUISpec } from './spec.js';
import { StateTransformParameters } from './state/common.js';
export declare class PluginUIContext extends PluginContext {
    spec: PluginUISpec;
    readonly customParamEditors: Map<string, StateTransformParameters.Class>;
    readonly customUIState: Record<string, any>;
    private initCustomParamEditors;
    dispose(options?: {
        doNotForceWebGLContextLoss?: boolean;
    }): void;
    constructor(spec: PluginUISpec);
}
