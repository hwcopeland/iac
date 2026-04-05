/**
 * Copyright (c) 2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Dušan Veľký <dvelky@mail.muni.cz>
 */
import { PluginStateObject } from '../../../mol-plugin-state/objects.js';
import { StateAction } from '../../../mol-state/index.js';
import { ParamDefinition as PD } from '../../../mol-util/param-definition.js';
export declare const TunnelDownloadServer: {
    channelsdb: PD.Group<PD.Normalize<unknown>>;
};
export declare const DownloadTunnels: StateAction<PluginStateObject.Root, void, PD.Normalize<{
    source: PD.NamedParams<PD.Normalize<{
        url: /*elided*/ any;
    }>, "url"> | PD.NamedParams<PD.Normalize<{
        provider: /*elided*/ any;
    }>, "alphafolddb"> | PD.NamedParams<PD.Normalize<{
        provider: /*elided*/ any;
    }>, "pdb">;
}>>;
