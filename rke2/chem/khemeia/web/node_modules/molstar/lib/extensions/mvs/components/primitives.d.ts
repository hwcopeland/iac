/**
 * Copyright (c) 2024-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Adam Midlik <midlik@gmail.com>
 */
import { Mesh } from '../../../mol-geo/geometry/mesh/mesh.js';
import { Text } from '../../../mol-geo/geometry/text/text.js';
import { Box3D, Sphere3D } from '../../../mol-math/geometry.js';
import { Mat4 } from '../../../mol-math/linear-algebra.js';
import { Shape } from '../../../mol-model/shape.js';
import { Structure } from '../../../mol-model/structure.js';
import { PluginStateObject as SO } from '../../../mol-plugin-state/objects.js';
import { StateObject, StateTransformer } from '../../../mol-state/index.js';
import { Asset } from '../../../mol-util/assets.js';
import { Clip } from '../../../mol-util/clip.js';
import { ParamDefinition as PD } from '../../../mol-util/param-definition.js';
import { MolstarNode, MolstarNodeParams, MolstarSubtree } from '../tree/molstar/molstar-tree.js';
import { MVSNode } from '../tree/mvs/mvs-tree.js';
type PrimitivesParams = MolstarNode<'primitives'>['params'];
export declare function getPrimitiveStructureRefs(primitives: MolstarSubtree<'primitives'>): Set<string>;
declare const MVSPrimitivesData_base: {
    new (data: PrimitiveBuilderContext, props?: {
        label: string;
        description?: string;
    } | undefined): {
        id: import("../../../mol-util/index.js").UUID;
        type: SO.TypeInfo;
        label: string;
        description?: string;
        data: PrimitiveBuilderContext;
    };
    type: SO.TypeInfo;
    is(obj?: StateObject): obj is StateObject<PrimitiveBuilderContext, SO.TypeInfo>;
};
export declare class MVSPrimitivesData extends MVSPrimitivesData_base {
}
declare const MVSPrimitiveShapes_base: {
    new (data: {
        mesh?: Shape<Mesh>;
        labels?: Shape<Text>;
    }, props?: {
        label: string;
        description?: string;
    } | undefined): {
        id: import("../../../mol-util/index.js").UUID;
        type: SO.TypeInfo;
        label: string;
        description?: string;
        data: {
            mesh?: Shape<Mesh>;
            labels?: Shape<Text>;
        };
    };
    type: SO.TypeInfo;
    is(obj?: StateObject): obj is StateObject<{
        mesh?: Shape<Mesh>;
        labels?: Shape<Text>;
    }, SO.TypeInfo>;
};
export declare class MVSPrimitiveShapes extends MVSPrimitiveShapes_base {
}
export interface MVSPrimitiveShapeSourceData {
    kind: 'mvs-primitives';
    node: MVSNode<'primitives'>;
    groupToNode: Map<number, MVSNode<'primitive'>>;
}
export type MVSDownloadPrimitiveData = typeof MVSDownloadPrimitiveData;
export declare const MVSDownloadPrimitiveData: StateTransformer<SO.Root, MVSPrimitivesData, PD.Normalize<{
    uri: string | Asset.Url;
    format: "mvs-node-json";
}>>;
interface PrimitivesSubtree {
    kind: 'primitives';
    params: MolstarNodeParams<'primitives'>;
    children?: {
        kind: 'primitive';
        params: MolstarNodeParams<'primitive'>;
    }[];
}
export type MVSInlinePrimitiveData = typeof MVSInlinePrimitiveData;
export declare const MVSInlinePrimitiveData: StateTransformer<SO.Root, MVSPrimitivesData, PD.Normalize<{
    node: PrimitivesSubtree;
}>>;
export type MVSBuildPrimitiveShape = typeof MVSBuildPrimitiveShape;
export declare const MVSBuildPrimitiveShape: StateTransformer<MVSPrimitivesData, SO.Shape.Provider, PD.Normalize<{
    kind: "lines" | "mesh" | "labels";
    clip: PD.Values<{
        variant: PD.Select<Clip.Variant>;
        objects: PD.ObjectList<PD.Normalize<{
            type: /*elided*/ any;
            invert: /*elided*/ any;
            position: /*elided*/ any;
            rotation: /*elided*/ any;
            scale: /*elided*/ any;
            transform: /*elided*/ any;
        }>>;
    }> | undefined;
}>>;
export declare const MVSShapeRepresentation3D: StateTransformer<SO.Shape.Provider, SO.Shape.Representation3D, PD.Normalize<{}>>;
interface PrimitiveBuilderContext {
    node: MolstarNode<'primitives'>;
    defaultStructure?: Structure;
    structureRefs: Record<string, Structure | undefined>;
    primitives: MolstarNode<'primitive'>[];
    options: PrimitivesParams;
    positionCache: Map<string, [isDefined: boolean, Sphere3D, Box3D]>;
    instances: Mat4[] | undefined;
    emptySelectionWarningPrinted?: boolean;
}
export {};
