/**
 * Copyright (c) 2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Dušan Veľký <dvelky@mail.muni.cz>
 */
import { PluginStateObject } from '../../../mol-plugin-state/objects.js';
import { StateTransformer } from '../../../mol-state/index.js';
import { TunnelStateObject, Tunnel, TunnelsStateObject } from './data-model.js';
import { ParamDefinition as PD } from '../../../mol-util/param-definition.js';
export declare const TunnelsFromRawData: StateTransformer<PluginStateObject.Root, TunnelsStateObject, PD.Normalize<{
    data: Tunnel[];
}>>;
export declare const SelectTunnel: StateTransformer<TunnelsStateObject, TunnelStateObject, PD.Normalize<{
    index: number;
}>>;
export declare const TunnelFromRawData: StateTransformer<PluginStateObject.Root, TunnelStateObject, PD.Normalize<{
    data: Tunnel;
}>>;
export declare const TunnelShapeProvider: StateTransformer<TunnelStateObject, PluginStateObject.Shape.Provider, PD.Normalize<{
    webgl: import("../../../mol-gl/webgl/context.js").WebGLContext | null;
    colorTheme: import("../../../mol-util/color/index.js").Color;
    visual: PD.NamedParams<PD.Normalize<{
        resolution: /*elided*/ any;
    }>, "spheres"> | PD.NamedParams<PD.Normalize<{
        resolution: /*elided*/ any;
    }>, "mesh">;
    samplingRate: number;
    showRadii: boolean;
}>>;
