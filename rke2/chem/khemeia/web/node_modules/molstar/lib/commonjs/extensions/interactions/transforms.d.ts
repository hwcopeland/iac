/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { PluginStateObject as SO } from '../../mol-plugin-state/objects.js';
import { StateTransformer } from '../../mol-state/index.js';
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { InteractionElementSchema, StructureInteractions } from './model.js';
declare const InteractionData_base: {
    new (data: {
        interactions: StructureInteractions;
    }, props?: {
        label: string;
        description?: string;
    } | undefined): {
        id: import("../../mol-util/index.js").UUID;
        type: SO.TypeInfo;
        label: string;
        description?: string;
        data: {
            interactions: StructureInteractions;
        };
    };
    type: SO.TypeInfo;
    is(obj?: import("../../mol-state/index.js").StateObject): obj is import("../../mol-state/index.js").StateObject<{
        interactions: StructureInteractions;
    }, SO.TypeInfo>;
};
export declare class InteractionData extends InteractionData_base {
}
export declare const ComputeContacts: StateTransformer<SO.Molecule.Structure.Selections, InteractionData, PD.Normalize<{
    interactions: PD.Normalize<{
        providers: /*elided*/ any;
        contacts: /*elided*/ any;
    }>;
}>>;
export declare const CustomInteractions: StateTransformer<SO.Root, InteractionData, PD.Normalize<{
    interactions: InteractionElementSchema[];
}>>;
export declare const InteractionsShape: StateTransformer<InteractionData, SO.Shape.Provider, PD.Normalize<{
    kinds: ("unknown" | "covalent" | "metal-coordination" | "hydrogen-bond" | "ionic" | "hydrophobic" | "pi-stacking" | "cation-pi" | "halogen-bond" | "weak-hydrogen-bond")[];
    styles: PD.Normalize<{
        unknown: /*elided*/ any;
        ionic: /*elided*/ any;
        'pi-stacking': /*elided*/ any;
        'cation-pi': /*elided*/ any;
        'halogen-bond': /*elided*/ any;
        'hydrogen-bond': /*elided*/ any;
        'weak-hydrogen-bond': /*elided*/ any;
        hydrophobic: /*elided*/ any;
        'metal-coordination': /*elided*/ any;
        covalent: /*elided*/ any;
    }>;
}>>;
export {};
