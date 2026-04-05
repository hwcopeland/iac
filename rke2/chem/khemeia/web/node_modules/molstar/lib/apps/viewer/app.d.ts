/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author Neli Fonseca <neli@ebi.ac.uk>
 * @author Adam Midlik <midlik@gmail.com>
 */
import { MolstarLoadingExtension } from '../../extensions/mvs/load.js';
import { MVSData } from '../../extensions/mvs/mvs-data.js';
import { StringLike } from '../../mol-io/common/string-like.js';
import { Structure, StructureElement } from '../../mol-model/structure.js';
import { Volume } from '../../mol-model/volume.js';
import { PresetTrajectoryHierarchy } from '../../mol-plugin-state/builder/structure/hierarchy-preset.js';
import { StructureRepresentationPresetProvider } from '../../mol-plugin-state/builder/structure/representation-preset.js';
import { BuiltInCoordinatesFormat } from '../../mol-plugin-state/formats/coordinates.js';
import { BuiltInTopologyFormat } from '../../mol-plugin-state/formats/topology.js';
import { BuiltInTrajectoryFormat } from '../../mol-plugin-state/formats/trajectory.js';
import { BuildInVolumeFormat } from '../../mol-plugin-state/formats/volume.js';
import { PluginStateObject } from '../../mol-plugin-state/objects.js';
import { PluginUIContext } from '../../mol-plugin-ui/context.js';
import { PluginState } from '../../mol-plugin/state.js';
import { MolScriptBuilder } from '../../mol-script/language/builder.js';
import { Expression } from '../../mol-script/language/expression.js';
import { StateObjectSelector } from '../../mol-state/index.js';
import { Color } from '../../mol-util/color/index.js';
import { ViewerOptions } from './options.js';
export { PLUGIN_VERSION as version } from '../../mol-plugin/version.js';
export { consoleStats, isDebugMode, isProductionMode, isTimingMode, setDebugMode, setProductionMode, setTimingMode } from '../../mol-util/debug.js';
import '../../mol-util/polyfill.js';
import { CameraFocusOptions } from '../../mol-plugin-state/manager/camera.js';
export declare class Viewer {
    private _events;
    readonly plugin: PluginUIContext;
    constructor(plugin: PluginUIContext);
    static create(elementOrId: string | HTMLElement, options?: Partial<ViewerOptions>): Promise<Viewer>;
    /**
     * Allows subscribing to rxjs observables in the context of the viewer.
     * All subscriptions will be disposed of when the viewer is destroyed.
     */
    subscribe: any;
    setRemoteSnapshot(id: string): Promise<void>;
    loadSnapshotFromUrl(url: string, type: PluginState.SnapshotType): Promise<void>;
    loadStructureFromUrl(url: string, format?: BuiltInTrajectoryFormat, isBinary?: boolean, options?: LoadStructureOptions & {
        label?: string;
    }): Promise<void>;
    loadAllModelsOrAssemblyFromUrl(url: string, format?: BuiltInTrajectoryFormat, isBinary?: boolean, options?: LoadStructureOptions): Promise<void>;
    loadStructureFromData(data: string | number[], format: BuiltInTrajectoryFormat, options?: {
        dataLabel?: string;
    }): Promise<void>;
    loadPdb(pdb: string, options?: LoadStructureOptions): Promise<void>;
    /**
     * @deprecated Scheduled for removal in v5. Use {@link loadPdbIhm | loadPdbIhm(pdbIhm: string)} instead.
     */
    loadPdbDev(pdbDev: string): Promise<void>;
    loadPdbIhm(pdbIhm: string): Promise<void>;
    loadEmdb(emdb: string, options?: {
        detail?: number;
    }): Promise<void>;
    loadAlphaFoldDb(afdb: string): Promise<void>;
    loadModelArchive(id: string): Promise<void>;
    /**
     * @example Load X-ray density from volume server
        viewer.loadVolumeFromUrl({
            url: 'https://www.ebi.ac.uk/pdbe/densities/x-ray/1tqn/cell?detail=3',
            format: 'dscif',
            isBinary: true
        }, [{
            type: 'relative',
            value: 1.5,
            color: 0x3362B2
        }, {
            type: 'relative',
            value: 3,
            color: 0x33BB33,
            volumeIndex: 1
        }, {
            type: 'relative',
            value: -3,
            color: 0xBB3333,
            volumeIndex: 1
        }], {
            entryId: ['2FO-FC', 'FO-FC'],
            isLazy: true
        });
     * *********************
     * @example Load EM density from volume server
        viewer.loadVolumeFromUrl({
            url: 'https://maps.rcsb.org/em/emd-30210/cell?detail=6',
            format: 'dscif',
            isBinary: true
        }, [{
            type: 'relative',
            value: 1,
            color: 0x3377aa
        }], {
            entryId: 'EMD-30210',
            isLazy: true
        });
     */
    loadVolumeFromUrl({ url, format, isBinary }: {
        url: string;
        format: BuildInVolumeFormat;
        isBinary: boolean;
    }, isovalues: VolumeIsovalueInfo[], options?: {
        entryId?: string | string[];
        isLazy?: boolean;
    }): Promise<void>;
    loadFullResolutionEMDBMap(emdbId: string, options: {
        isoValue: Volume.IsoValue;
        color?: Color;
    }): Promise<void>;
    /**
     * @example
     *  viewer.loadTrajectory({
     *      model: { kind: 'model-url', url: 'villin.gro', format: 'gro' },
     *      coordinates: { kind: 'coordinates-url', url: 'villin.xtc', format: 'xtc', isBinary: true },
     *      preset: 'all-models' // or 'default'
     *  });
     */
    loadTrajectory(params: LoadTrajectoryParams): Promise<{
        model: StateObjectSelector<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
        coords: any;
        preset: {
            model: StateObjectSelector<PluginStateObject.Molecule.Model, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
            modelProperties: StateObjectSelector<PluginStateObject.Molecule.Model, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
            unitcell: StateObjectSelector<PluginStateObject.Shape.Representation3D, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined;
            structure: StateObjectSelector<PluginStateObject.Molecule.Structure, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
            structureProperties: StateObjectSelector<PluginStateObject.Molecule.Structure, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
            representation: any;
        } | {
            models?: undefined;
            structures?: undefined;
        } | {
            models: StateObjectSelector<PluginStateObject.Molecule.Model, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>[];
            structures: StateObjectSelector<PluginStateObject.Molecule.Structure, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>[];
        } | {
            model: StateObjectSelector<PluginStateObject.Molecule.Model, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
            modelProperties: StateObjectSelector<PluginStateObject.Molecule.Model, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
            unitcell: StateObjectSelector<PluginStateObject.Shape.Representation3D, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined;
            structure: StateObjectSelector<PluginStateObject.Molecule.Structure, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
            structureProperties: StateObjectSelector<PluginStateObject.Molecule.Structure, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
            representation: any;
        } | {
            model: StateObjectSelector<PluginStateObject.Molecule.Model, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
            modelProperties: StateObjectSelector<PluginStateObject.Molecule.Model, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
            unitcell: StateObjectSelector<PluginStateObject.Shape.Representation3D, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined;
            structure: StateObjectSelector<PluginStateObject.Molecule.Structure, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
            structureProperties: StateObjectSelector<PluginStateObject.Molecule.Structure, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
            representation: any;
        } | undefined;
    }>;
    loadMvsFromUrl(url: string, format: 'mvsj' | 'mvsx', options?: {
        appendSnapshots?: boolean;
        keepCamera?: boolean;
        keepCameraOrientation?: boolean;
        extensions?: MolstarLoadingExtension<any>[];
    }): Promise<void>;
    /** Load MolViewSpec from `data`.
     * If `format` is 'mvsj', `data` must be a string or a Uint8Array containing a UTF8-encoded string.
     * If `format` is 'mvsx', `data` must be a Uint8Array or a string containing base64-encoded binary data prefixed with 'base64,'. */
    loadMvsData(data: string | Uint8Array<ArrayBuffer>, format: 'mvsj' | 'mvsx', options?: {
        appendSnapshots?: boolean;
        keepCamera?: boolean;
        keepCameraOrientation?: boolean;
        extensions?: MolstarLoadingExtension<any>[];
    }): Promise<Uint8Array<ArrayBuffer> | StringLike | MVSData>;
    loadFiles(files: File[]): Promise<void>;
    handleResize(): void;
    /**
     * Triggers structure element selection or highlighting based on the provided
     * MolScript expression or StructureElement schema. Focus action will only apply to the
     * first structure that matches the criteria.
     *
     * If neither `expression` nor `elements` are provided, all selections/highlights
     * will be cleared based on the specified `action`.
     */
    structureInteractivity({ expression, elements, action, applyGranularity, filterStructure, focusOptions }: {
        expression?: (queryBuilder: typeof MolScriptBuilder) => Expression;
        elements?: StructureElement.Schema;
        action: 'highlight' | 'select' | 'focus';
        applyGranularity?: boolean;
        filterStructure?: (structure: Structure) => boolean;
        focusOptions?: Partial<CameraFocusOptions>;
    }): void;
    dispose(): void;
}
export interface LoadStructureOptions {
    representationParams?: StructureRepresentationPresetProvider.CommonParams;
}
export interface VolumeIsovalueInfo {
    type: 'absolute' | 'relative';
    value: number;
    color: Color;
    alpha?: number;
    volumeIndex?: number;
}
export interface LoadTrajectoryParams {
    model: {
        kind: 'model-url';
        url: string;
        format?: BuiltInTrajectoryFormat;
        isBinary?: boolean;
    } | {
        kind: 'model-data';
        data: string | number[] | ArrayBuffer | Uint8Array<ArrayBuffer>;
        format?: BuiltInTrajectoryFormat;
    } | {
        kind: 'topology-url';
        url: string;
        format: BuiltInTopologyFormat;
        isBinary?: boolean;
    } | {
        kind: 'topology-data';
        data: string | number[] | ArrayBuffer | Uint8Array<ArrayBuffer>;
        format: BuiltInTopologyFormat;
    };
    modelLabel?: string;
    coordinates: {
        kind: 'coordinates-url';
        url: string;
        format: BuiltInCoordinatesFormat;
        isBinary?: boolean;
    } | {
        kind: 'coordinates-data';
        data: string | number[] | ArrayBuffer | Uint8Array<ArrayBuffer>;
        format: BuiltInCoordinatesFormat;
    };
    coordinatesLabel?: string;
    preset?: keyof PresetTrajectoryHierarchy;
}
