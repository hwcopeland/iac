/**
 * Copyright (c) 2018-2025 Mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Type } from '../type.js';
import * as Core from './core.js';
import { Arguments, Argument } from '../symbol.js';
export declare namespace Types {
    const ElementSymbol: Type.Value<unknown>;
    const AtomName: Type.Value<unknown>;
    const BondFlag: Type.OneOf<string>;
    const BondFlags: Type.Container<number>;
    const SecondaryStructureFlag: Type.OneOf<string>;
    const SecondaryStructureFlags: Type.Container<number>;
    const RingFingerprint: Type.Value<unknown>;
    const EntityType: Type.OneOf<string>;
    const EntitySubtype: Type.OneOf<string>;
    const ObjectPrimitive: Type.OneOf<string>;
    const ResidueId: Type.Value<unknown>;
    const ElementSet: Type.Value<unknown>;
    const ElementSelection: Type.Value<unknown>;
    const ElementReference: Type.Value<unknown>;
    const ElementSelectionQuery: Type.Container<(env: any) => unknown>;
}
export declare const structureQuery: {
    '@header': string;
    type: {
        '@header': string;
        elementSymbol: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Value<string>>;
        }>>, Type.Value<unknown>>;
        atomName: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.AnyValue>;
        }>>, Type.Value<unknown>>;
        entityType: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.OneOf<string>>;
        }>>, Type.OneOf<string>>;
        bondFlags: import("../symbol.js").MSymbol<Arguments<{
            [key: string]: string;
        }>, Type.Container<number>>;
        ringFingerprint: import("../symbol.js").MSymbol<Arguments<{
            [key: string]: unknown;
        }>, Type.Value<unknown>>;
        secondaryStructureFlags: import("../symbol.js").MSymbol<Arguments<{
            [key: string]: string;
        }>, Type.Container<number>>;
        authResidueId: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Value<string>>;
            1: Argument<Type.Value<number>>;
            2: Argument<Type.Value<string>>;
        }>>, Type.Value<unknown>>;
        labelResidueId: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Value<string>>;
            1: Argument<Type.Value<string>>;
            2: Argument<Type.Value<number>>;
            3: Argument<Type.Value<string>>;
        }>>, Type.Value<unknown>>;
    };
    slot: {
        '@header': string;
        element: import("../symbol.js").MSymbol<Arguments<{}>, Type.Value<unknown>>;
        elementSetReduce: import("../symbol.js").MSymbol<Arguments<{}>, Type.Variable<any>>;
    };
    generator: {
        '@header': string;
        all: import("../symbol.js").MSymbol<Arguments<{}>, Type.Container<(env: any) => unknown>>;
        atomGroups: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            'entity-test': Argument<Type.OneOf<boolean>>;
            'chain-test': Argument<Type.OneOf<boolean>>;
            'residue-test': Argument<Type.OneOf<boolean>>;
            'atom-test': Argument<Type.OneOf<boolean>>;
            'group-by': Argument<Type.Any>;
        }>>, Type.Container<(env: any) => unknown>>;
        bondedAtomicPairs: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.OneOf<boolean>>;
        }>>, Type.Container<(env: any) => unknown>>;
        rings: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            fingerprint: Argument<Type.Value<unknown>>;
            'only-aromatic': Argument<Type.OneOf<boolean>>;
        }>>, Type.Container<(env: any) => unknown>>;
        queryInSelection: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Container<(env: any) => unknown>>;
            query: Argument<Type.Container<(env: any) => unknown>>;
            'in-complement': Argument<Type.OneOf<boolean>>;
        }>>, Type.Container<(env: any) => unknown>>;
        empty: import("../symbol.js").MSymbol<Arguments<{}>, Type.Container<(env: any) => unknown>>;
    };
    modifier: {
        '@header': string;
        queryEach: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Container<(env: any) => unknown>>;
            query: Argument<Type.Container<(env: any) => unknown>>;
        }>>, Type.Container<(env: any) => unknown>>;
        intersectBy: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Container<(env: any) => unknown>>;
            by: Argument<Type.Container<(env: any) => unknown>>;
        }>>, Type.Container<(env: any) => unknown>>;
        exceptBy: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Container<(env: any) => unknown>>;
            by: Argument<Type.Container<(env: any) => unknown>>;
        }>>, Type.Container<(env: any) => unknown>>;
        unionBy: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Container<(env: any) => unknown>>;
            by: Argument<Type.Container<(env: any) => unknown>>;
        }>>, Type.Container<(env: any) => unknown>>;
        union: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Container<(env: any) => unknown>>;
        }>>, Type.Container<(env: any) => unknown>>;
        cluster: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Container<(env: any) => unknown>>;
            'min-distance': Argument<Type.Value<number>>;
            'max-distance': Argument<Type.Value<number>>;
            'min-size': Argument<Type.Value<number>>;
            'max-size': Argument<Type.Value<number>>;
        }>>, Type.Container<(env: any) => unknown>>;
        includeSurroundings: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Container<(env: any) => unknown>>;
            radius: Argument<Type.Value<number>>;
            'atom-radius': Argument<Type.Value<number>>;
            'as-whole-residues': Argument<Type.OneOf<boolean>>;
        }>>, Type.Container<(env: any) => unknown>>;
        surroundingLigands: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Container<(env: any) => unknown>>;
            radius: Argument<Type.Value<number>>;
            'include-water': Argument<Type.OneOf<boolean>>;
        }>>, Type.Container<(env: any) => unknown>>;
        includeConnected: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Container<(env: any) => unknown>>;
            'bond-test': Argument<Type.OneOf<boolean>>;
            'layer-count': Argument<Type.Value<number>>;
            'fixed-point': Argument<Type.OneOf<boolean>>;
            'as-whole-residues': Argument<Type.OneOf<boolean>>;
        }>>, Type.Container<(env: any) => unknown>>;
        wholeResidues: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Container<(env: any) => unknown>>;
        }>>, Type.Container<(env: any) => unknown>>;
        expandProperty: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Container<(env: any) => unknown>>;
            property: Argument<Type.AnyValue>;
        }>>, Type.Container<(env: any) => unknown>>;
    };
    filter: {
        '@header': string;
        pick: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Container<(env: any) => unknown>>;
            test: Argument<Type.OneOf<boolean>>;
        }>>, Type.Container<(env: any) => unknown>>;
        first: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Container<(env: any) => unknown>>;
        }>>, Type.Container<(env: any) => unknown>>;
        withSameAtomProperties: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Container<(env: any) => unknown>>;
            source: Argument<Type.Container<(env: any) => unknown>>;
            property: Argument<Type.Any>;
        }>>, Type.Container<(env: any) => unknown>>;
        intersectedBy: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Container<(env: any) => unknown>>;
            by: Argument<Type.Container<(env: any) => unknown>>;
        }>>, Type.Container<(env: any) => unknown>>;
        within: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Container<(env: any) => unknown>>;
            target: Argument<Type.Container<(env: any) => unknown>>;
            'min-radius': Argument<Type.Value<number>>;
            'max-radius': Argument<Type.Value<number>>;
            'atom-radius': Argument<Type.Value<number>>;
            invert: Argument<Type.OneOf<boolean>>;
        }>>, Type.Container<(env: any) => unknown>>;
        isConnectedTo: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Container<(env: any) => unknown>>;
            target: Argument<Type.Container<(env: any) => unknown>>;
            'bond-test': Argument<Type.OneOf<boolean>>;
            disjunct: Argument<Type.OneOf<boolean>>;
            invert: Argument<Type.OneOf<boolean>>;
        }>>, Type.Container<(env: any) => unknown>>;
    };
    combinator: {
        '@header': string;
        intersect: import("../symbol.js").MSymbol<Arguments<{
            [key: string]: (env: any) => unknown;
        }>, Type.Container<(env: any) => unknown>>;
        merge: import("../symbol.js").MSymbol<Arguments<{
            [key: string]: (env: any) => unknown;
        }>, Type.Container<(env: any) => unknown>>;
        distanceCluster: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            matrix: Argument<Type.Container<Core.Types.List<Core.Types.List<number>>>>;
            selections: Argument<Type.Container<Core.Types.List<(env: any) => unknown>>>;
        }>>, Type.Container<(env: any) => unknown>>;
    };
    atomSet: {
        '@header': string;
        atomCount: import("../symbol.js").MSymbol<Arguments<{}>, Type.Value<number>>;
        countQuery: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Container<(env: any) => unknown>>;
        }>>, Type.Value<number>>;
        reduce: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            initial: Argument<Type.Variable<any>>;
            value: Argument<Type.Variable<any>>;
        }>>, Type.Variable<any>>;
        propertySet: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
            0: Argument<Type.Variable<any>>;
        }>>, Type.Container<Core.Types.Set<any>>>;
    };
    atomProperty: {
        '@header': string;
        core: {
            '@header': string;
            elementSymbol: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            vdw: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            mass: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            atomicNumber: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            x: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            y: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            z: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            atomKey: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            bondCount: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
                flags: Argument<Type.Container<number>>;
            }>>, Type.Value<number>>;
            sourceIndex: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            operatorName: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            instanceId: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            operatorKey: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            modelIndex: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            modelLabel: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            modelEntryId: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
        };
        topology: {
            connectedComponentKey: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
        };
        macromolecular: {
            '@header': string;
            authResidueId: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            labelResidueId: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            residueKey: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            chainKey: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            entityKey: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            isHet: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            id: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            label_atom_id: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            label_alt_id: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            label_comp_id: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            label_asym_id: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            label_entity_id: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            label_seq_id: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            auth_atom_id: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            auth_comp_id: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            auth_asym_id: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            auth_seq_id: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            residueSourceIndex: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            pdbx_PDB_ins_code: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            pdbx_formal_charge: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            occupancy: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            B_iso_or_equiv: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            entityType: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            entitySubtype: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            entityPrdId: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            entityDescription: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            objectPrimitive: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            secondaryStructureKey: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            secondaryStructureFlags: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            isModified: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            modifiedParentName: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            isNonStandard: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
            chemCompType: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<unknown>>;
            }>>, Type>;
        };
        ihm: {
            hasSeqId: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                0: Argument<Type.Value<number>>;
            }>>, Type.OneOf<boolean>>;
            overlapsSeqIdRange: import("../symbol.js").MSymbol<Arguments<Arguments.PropTypes<{
                beg: Argument<Type.Value<number>>;
                end: Argument<Type.Value<number>>;
            }>>, Type.OneOf<boolean>>;
        };
    };
    bondProperty: {
        '@header': string;
        flags: import("../symbol.js").MSymbol<Arguments<{}>, Type>;
        order: import("../symbol.js").MSymbol<Arguments<{}>, Type>;
        key: import("../symbol.js").MSymbol<Arguments<{}>, Type>;
        length: import("../symbol.js").MSymbol<Arguments<{}>, Type>;
        atomA: import("../symbol.js").MSymbol<Arguments<{}>, Type>;
        atomB: import("../symbol.js").MSymbol<Arguments<{}>, Type>;
    };
};
