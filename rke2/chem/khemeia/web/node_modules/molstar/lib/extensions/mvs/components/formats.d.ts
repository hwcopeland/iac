/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { StringLike } from '../../../mol-io/common/string-like.js';
import { DataFormatProvider } from '../../../mol-plugin-state/formats/provider.js';
import { PluginStateObject as SO } from '../../../mol-plugin-state/objects.js';
import { PluginContext } from '../../../mol-plugin/context.js';
import { StateAction, StateObjectRef } from '../../../mol-state/index.js';
import { RuntimeContext } from '../../../mol-task/index.js';
import { ParamDefinition as PD } from '../../../mol-util/param-definition.js';
import { MVSLoadOptions } from '../load.js';
import { MVSData } from '../mvs-data.js';
declare const Mvs_base: {
    new (data: {
        mvsData: MVSData;
        sourceUrl?: string;
    }, props?: {
        label: string;
        description?: string;
    } | undefined): {
        id: import("../../../mol-util/index.js").UUID;
        type: SO.TypeInfo;
        label: string;
        description?: string;
        data: {
            mvsData: MVSData;
            sourceUrl?: string;
        };
    };
    type: SO.TypeInfo;
    is(obj?: import("../../../mol-state/index.js").StateObject): obj is import("../../../mol-state/index.js").StateObject<{
        mvsData: MVSData;
        sourceUrl?: string;
    }, SO.TypeInfo>;
};
/** Plugin state object storing `MVSData` */
export declare class Mvs extends Mvs_base {
}
/** Transformer for parsing data in MVSJ format */
export declare const ParseMVSJ: import("../../../mol-state/index.js").StateTransformer<SO.Data.String, Mvs, PD.Normalize<{}>>;
/** Transformer for parsing data in MVSX format (= zipped MVSJ + referenced files like structures and annotations) */
export declare const ParseMVSX: import("../../../mol-state/index.js").StateTransformer<SO.Data.Binary, Mvs, PD.Normalize<{
    mainFilePath: string;
}>>;
/** Params for the `LoadMvsData` action */
export declare const LoadMvsDataParams: {
    appendSnapshots: PD.BooleanParam;
    keepCamera: PD.BooleanParam;
    keepCameraOrientation: PD.BooleanParam;
    applyExtensions: PD.BooleanParam;
};
/** State action which loads a MVS view into Mol* */
export declare const LoadMvsData: StateAction<Mvs, void, PD.Normalize<{
    appendSnapshots: boolean;
    keepCamera: boolean;
    keepCameraOrientation: boolean;
    applyExtensions: boolean;
}>>;
/** Data format provider for MVSJ format.
 * If Visuals:On, it will load the parsed MVS view;
 * otherwise it will just create a plugin state object with parsed data. */
export declare const MVSJFormatProvider: DataFormatProvider<{}, StateObjectRef<Mvs>, any>;
/** Data format provider for MVSX format.
 * If Visuals:On, it will load the parsed MVS view;
 * otherwise it will just create a plugin state object with parsed data. */
export declare const MVSXFormatProvider: DataFormatProvider<{}, StateObjectRef<Mvs>, any>;
/** Parse binary data `data` as MVSX archive,
 * add all contained files to `plugin`'s asset manager,
 * and parse the main file in the archive as MVSJ.
 * Return parsed MVS data and `sourceUrl` for resolution of relative URIs.  */
export declare function loadMVSX(plugin: PluginContext, runtimeCtx: RuntimeContext, data: Uint8Array<ArrayBuffer>, mainFilePathOrOptions?: string | {
    mainFilePath?: string;
    doNotClearAssets?: boolean;
}): Promise<{
    mvsData: MVSData;
    sourceUrl: string;
}>;
export declare function loadMVSData(plugin: PluginContext, data: MVSData | StringLike | Uint8Array<ArrayBuffer>, format: 'mvsj' | 'mvsx', options?: MVSLoadOptions): Promise<Uint8Array<ArrayBuffer> | StringLike | MVSData>;
export {};
