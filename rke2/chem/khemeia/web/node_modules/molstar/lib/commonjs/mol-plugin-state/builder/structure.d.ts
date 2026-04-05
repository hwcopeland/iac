/**
 * Copyright (c) 2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { PluginContext } from '../../mol-plugin/context.js';
import { StateObjectRef, StateObjectSelector, StateTransformer, StateTransform, StateObjectCell } from '../../mol-state/index.js';
import { PluginStateObject as SO } from '../objects.js';
import { StateTransforms } from '../transforms.js';
import { RootStructureDefinition } from '../helpers/root-structure.js';
import { StructureComponentParams, StaticStructureComponentType } from '../helpers/structure-component.js';
import { BuiltInTrajectoryFormat, TrajectoryFormatProvider } from '../formats/trajectory.js';
import { StructureRepresentationBuilder } from './structure/representation.js';
import { StructureSelectionQuery } from '../helpers/structure-selection-query.js';
import { Expression } from '../../mol-script/language/expression.js';
import { TrajectoryHierarchyBuilder } from './structure/hierarchy.js';
export declare class StructureBuilder {
    plugin: PluginContext;
    private get dataState();
    private parseTrajectoryData;
    private parseTrajectoryBlob;
    readonly hierarchy: TrajectoryHierarchyBuilder;
    readonly representation: StructureRepresentationBuilder;
    parseTrajectory(data: StateObjectRef<SO.Data.Binary | SO.Data.String>, format: BuiltInTrajectoryFormat | TrajectoryFormatProvider): Promise<StateObjectSelector<SO.Molecule.Trajectory>>;
    parseTrajectory(blob: StateObjectRef<SO.Data.Blob>, params: StateTransformer.Params<StateTransforms['Data']['ParseBlob']>): Promise<StateObjectSelector<SO.Molecule.Trajectory>>;
    createModel(trajectory: StateObjectRef<SO.Molecule.Trajectory>, params?: StateTransformer.Params<StateTransforms['Model']['ModelFromTrajectory']>, initialState?: Partial<StateTransform.State>): Promise<StateObjectSelector<SO.Molecule.Model, StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>>;
    insertModelProperties(model: StateObjectRef<SO.Molecule.Model>, params?: StateTransformer.Params<StateTransforms['Model']['CustomModelProperties']>, initialState?: Partial<StateTransform.State>): Promise<StateObjectSelector<SO.Molecule.Model, StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>>;
    tryCreateUnitcell(model: StateObjectRef<SO.Molecule.Model>, params?: StateTransformer.Params<StateTransforms['Representation']['ModelUnitcell3D']>, initialState?: Partial<StateTransform.State>): Promise<StateObjectSelector<SO.Shape.Representation3D, StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>> | undefined;
    createStructure(modelRef: StateObjectRef<SO.Molecule.Model>, params?: RootStructureDefinition.Params, initialState?: Partial<StateTransform.State>, tags?: string | string[]): Promise<StateObjectSelector<SO.Molecule.Structure, StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>>;
    insertStructureProperties(structure: StateObjectRef<SO.Molecule.Structure>, params?: StateTransformer.Params<StateTransforms['Model']['CustomStructureProperties']>): Promise<StateObjectSelector<SO.Molecule.Structure, StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>>;
    isComponentTransform(cell: StateObjectCell): boolean;
    /** returns undefined if the component is empty/null */
    tryCreateComponent(structure: StateObjectRef<SO.Molecule.Structure>, params: StructureComponentParams, key: string, tags?: string[]): Promise<StateObjectSelector<SO.Molecule.Structure> | undefined>;
    tryCreateComponentFromExpression(structure: StateObjectRef<SO.Molecule.Structure>, expression: Expression, key: string, params?: {
        label?: string;
        tags?: string[];
    }): Promise<StateObjectSelector<SO.Molecule.Structure, StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined>;
    tryCreateComponentStatic(structure: StateObjectRef<SO.Molecule.Structure>, type: StaticStructureComponentType, params?: {
        label?: string;
        tags?: string[];
    }): Promise<StateObjectSelector<SO.Molecule.Structure, StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined>;
    tryCreateComponentFromSelection(structure: StateObjectRef<SO.Molecule.Structure>, selection: StructureSelectionQuery, key: string, params?: {
        label?: string;
        tags?: string[];
    }): Promise<StateObjectSelector<SO.Molecule.Structure> | undefined>;
    constructor(plugin: PluginContext);
}
