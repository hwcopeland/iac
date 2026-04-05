/**
 * Copyright (c) 2017 Mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { MSymbol } from './symbol.js';
declare const MolScriptSymbolTable: {
    core: {
        '@header': string;
        type: {
            '@header': string;
            bool: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.AnyValue>;
            }>>, import("./type.js").Type.OneOf<boolean>>;
            num: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.AnyValue>;
            }>>, import("./type.js").Type.Value<number>>;
            str: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.AnyValue>;
            }>>, import("./type.js").Type.Value<string>>;
            regex: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<string>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Value<string>>;
            }>>, import("./type.js").Type.Value<RegExp>>;
            list: MSymbol<import("./symbol.js").Arguments<{
                [key: string]: any;
            }>, import("./type.js").Type.Container<import("./symbol-table/core.js").Types.List<any>>>;
            set: MSymbol<import("./symbol.js").Arguments<{
                [key: string]: any;
            }>, import("./type.js").Type.Container<import("./symbol-table/core.js").Types.Set<any>>>;
            bitflags: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Container<number>>;
            compositeKey: MSymbol<import("./symbol.js").Arguments<{
                [key: string]: any;
            }>, import("./type.js").Type.AnyValue>;
        };
        logic: {
            '@header': string;
            not: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.OneOf<boolean>>;
            }>>, import("./type.js").Type.OneOf<boolean>>;
            and: MSymbol<import("./symbol.js").Arguments<{
                [key: string]: boolean;
            }>, import("./type.js").Type.OneOf<boolean>>;
            or: MSymbol<import("./symbol.js").Arguments<{
                [key: string]: boolean;
            }>, import("./type.js").Type.OneOf<boolean>>;
        };
        ctrl: {
            '@header': string;
            eval: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => any>>;
            }>>, import("./type.js").Type.Variable<any>>;
            fn: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Variable<any>>;
            }>>, import("./type.js").Type.Container<(env: any) => any>>;
            if: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.OneOf<boolean>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Variable<any>>;
                2: import("./symbol.js").Argument<import("./type.js").Type.Variable<any>>;
            }>>, import("./type.js").Type.Union<any>>;
            assoc: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<string>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Variable<any>>;
            }>>, import("./type.js").Type.Variable<any>>;
        };
        rel: {
            '@header': string;
            eq: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Variable<any>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Variable<any>>;
            }>>, import("./type.js").Type.OneOf<boolean>>;
            neq: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Variable<any>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Variable<any>>;
            }>>, import("./type.js").Type.OneOf<boolean>>;
            lt: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.OneOf<boolean>>;
            lte: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.OneOf<boolean>>;
            gr: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.OneOf<boolean>>;
            gre: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.OneOf<boolean>>;
            inRange: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                2: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.OneOf<boolean>>;
        };
        math: {
            '@header': string;
            add: MSymbol<import("./symbol.js").Arguments<{
                [key: string]: number;
            }>, import("./type.js").Type.Value<number>>;
            sub: MSymbol<import("./symbol.js").Arguments<{
                [key: string]: number;
            }>, import("./type.js").Type.Value<number>>;
            mult: MSymbol<import("./symbol.js").Arguments<{
                [key: string]: number;
            }>, import("./type.js").Type.Value<number>>;
            div: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            pow: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            mod: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            min: MSymbol<import("./symbol.js").Arguments<{
                [key: string]: number;
            }>, import("./type.js").Type.Value<number>>;
            max: MSymbol<import("./symbol.js").Arguments<{
                [key: string]: number;
            }>, import("./type.js").Type.Value<number>>;
            cantorPairing: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            sortedCantorPairing: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            invertCantorPairing: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Container<import("./symbol-table/core.js").Types.List<number>>>;
            floor: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            ceil: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            roundInt: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            trunc: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            abs: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            sign: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            sqrt: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            cbrt: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            sin: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            cos: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            tan: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            asin: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            acos: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            atan: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            sinh: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            cosh: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            tanh: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            exp: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            log: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            log10: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
            atan2: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Value<number>>;
        };
        str: {
            '@header': string;
            concat: MSymbol<import("./symbol.js").Arguments<{
                [key: string]: string;
            }>, import("./type.js").Type.Value<string>>;
            match: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<RegExp>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Value<string>>;
            }>>, import("./type.js").Type.OneOf<boolean>>;
        };
        list: {
            '@header': string;
            getAt: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<import("./symbol-table/core.js").Types.List<any>>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Variable<any>>;
            equal: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<import("./symbol-table/core.js").Types.List<any>>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Container<import("./symbol-table/core.js").Types.List<any>>>;
            }>>, import("./type.js").Type.OneOf<boolean>>;
        };
        set: {
            '@header': string;
            has: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<import("./symbol-table/core.js").Types.Set<any>>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Variable<any>>;
            }>>, import("./type.js").Type.OneOf<boolean>>;
            isSubset: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<import("./symbol-table/core.js").Types.Set<any>>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Container<import("./symbol-table/core.js").Types.Set<any>>>;
            }>>, import("./type.js").Type.OneOf<boolean>>;
        };
        flags: {
            '@header': string;
            hasAny: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<number>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Container<number>>;
            }>>, import("./type.js").Type.OneOf<boolean>>;
            hasAll: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<number>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Container<number>>;
            }>>, import("./type.js").Type.OneOf<boolean>>;
        };
    };
    structureQuery: {
        '@header': string;
        type: {
            '@header': string;
            elementSymbol: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<string>>;
            }>>, import("./type.js").Type.Value<unknown>>;
            atomName: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.AnyValue>;
            }>>, import("./type.js").Type.Value<unknown>>;
            entityType: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.OneOf<string>>;
            }>>, import("./type.js").Type.OneOf<string>>;
            bondFlags: MSymbol<import("./symbol.js").Arguments<{
                [key: string]: string;
            }>, import("./type.js").Type.Container<number>>;
            ringFingerprint: MSymbol<import("./symbol.js").Arguments<{
                [key: string]: unknown;
            }>, import("./type.js").Type.Value<unknown>>;
            secondaryStructureFlags: MSymbol<import("./symbol.js").Arguments<{
                [key: string]: string;
            }>, import("./type.js").Type.Container<number>>;
            authResidueId: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<string>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                2: import("./symbol.js").Argument<import("./type.js").Type.Value<string>>;
            }>>, import("./type.js").Type.Value<unknown>>;
            labelResidueId: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Value<string>>;
                1: import("./symbol.js").Argument<import("./type.js").Type.Value<string>>;
                2: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                3: import("./symbol.js").Argument<import("./type.js").Type.Value<string>>;
            }>>, import("./type.js").Type.Value<unknown>>;
        };
        slot: {
            '@header': string;
            element: MSymbol<import("./symbol.js").Arguments<{}>, import("./type.js").Type.Value<unknown>>;
            elementSetReduce: MSymbol<import("./symbol.js").Arguments<{}>, import("./type.js").Type.Variable<any>>;
        };
        generator: {
            '@header': string;
            all: MSymbol<import("./symbol.js").Arguments<{}>, import("./type.js").Type.Container<(env: any) => unknown>>;
            atomGroups: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                'entity-test': import("./symbol.js").Argument<import("./type.js").Type.OneOf<boolean>>;
                'chain-test': import("./symbol.js").Argument<import("./type.js").Type.OneOf<boolean>>;
                'residue-test': import("./symbol.js").Argument<import("./type.js").Type.OneOf<boolean>>;
                'atom-test': import("./symbol.js").Argument<import("./type.js").Type.OneOf<boolean>>;
                'group-by': import("./symbol.js").Argument<import("./type.js").Type.Any>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
            bondedAtomicPairs: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.OneOf<boolean>>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
            rings: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                fingerprint: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                'only-aromatic': import("./symbol.js").Argument<import("./type.js").Type.OneOf<boolean>>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
            queryInSelection: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
                query: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
                'in-complement': import("./symbol.js").Argument<import("./type.js").Type.OneOf<boolean>>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
            empty: MSymbol<import("./symbol.js").Arguments<{}>, import("./type.js").Type.Container<(env: any) => unknown>>;
        };
        modifier: {
            '@header': string;
            queryEach: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
                query: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
            intersectBy: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
                by: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
            exceptBy: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
                by: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
            unionBy: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
                by: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
            union: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
            cluster: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
                'min-distance': import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                'max-distance': import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                'min-size': import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                'max-size': import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
            includeSurroundings: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
                radius: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                'atom-radius': import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                'as-whole-residues': import("./symbol.js").Argument<import("./type.js").Type.OneOf<boolean>>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
            surroundingLigands: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
                radius: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                'include-water': import("./symbol.js").Argument<import("./type.js").Type.OneOf<boolean>>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
            includeConnected: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
                'bond-test': import("./symbol.js").Argument<import("./type.js").Type.OneOf<boolean>>;
                'layer-count': import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                'fixed-point': import("./symbol.js").Argument<import("./type.js").Type.OneOf<boolean>>;
                'as-whole-residues': import("./symbol.js").Argument<import("./type.js").Type.OneOf<boolean>>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
            wholeResidues: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
            expandProperty: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
                property: import("./symbol.js").Argument<import("./type.js").Type.AnyValue>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
        };
        filter: {
            '@header': string;
            pick: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
                test: import("./symbol.js").Argument<import("./type.js").Type.OneOf<boolean>>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
            first: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
            withSameAtomProperties: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
                source: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
                property: import("./symbol.js").Argument<import("./type.js").Type.Any>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
            intersectedBy: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
                by: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
            within: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
                target: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
                'min-radius': import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                'max-radius': import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                'atom-radius': import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                invert: import("./symbol.js").Argument<import("./type.js").Type.OneOf<boolean>>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
            isConnectedTo: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
                target: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
                'bond-test': import("./symbol.js").Argument<import("./type.js").Type.OneOf<boolean>>;
                disjunct: import("./symbol.js").Argument<import("./type.js").Type.OneOf<boolean>>;
                invert: import("./symbol.js").Argument<import("./type.js").Type.OneOf<boolean>>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
        };
        combinator: {
            '@header': string;
            intersect: MSymbol<import("./symbol.js").Arguments<{
                [key: string]: (env: any) => unknown;
            }>, import("./type.js").Type.Container<(env: any) => unknown>>;
            merge: MSymbol<import("./symbol.js").Arguments<{
                [key: string]: (env: any) => unknown;
            }>, import("./type.js").Type.Container<(env: any) => unknown>>;
            distanceCluster: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                matrix: import("./symbol.js").Argument<import("./type.js").Type.Container<import("./symbol-table/core.js").Types.List<import("./symbol-table/core.js").Types.List<number>>>>;
                selections: import("./symbol.js").Argument<import("./type.js").Type.Container<import("./symbol-table/core.js").Types.List<(env: any) => unknown>>>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
        };
        atomSet: {
            '@header': string;
            atomCount: MSymbol<import("./symbol.js").Arguments<{}>, import("./type.js").Type.Value<number>>;
            countQuery: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Container<(env: any) => unknown>>;
            }>>, import("./type.js").Type.Value<number>>;
            reduce: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                initial: import("./symbol.js").Argument<import("./type.js").Type.Variable<any>>;
                value: import("./symbol.js").Argument<import("./type.js").Type.Variable<any>>;
            }>>, import("./type.js").Type.Variable<any>>;
            propertySet: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                0: import("./symbol.js").Argument<import("./type.js").Type.Variable<any>>;
            }>>, import("./type.js").Type.Container<import("./symbol-table/core.js").Types.Set<any>>>;
        };
        atomProperty: {
            '@header': string;
            core: {
                '@header': string;
                elementSymbol: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                vdw: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                mass: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                atomicNumber: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                x: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                y: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                z: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                atomKey: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                bondCount: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                    flags: import("./symbol.js").Argument<import("./type.js").Type.Container<number>>;
                }>>, import("./type.js").Type.Value<number>>;
                sourceIndex: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                operatorName: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                instanceId: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                operatorKey: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                modelIndex: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                modelLabel: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                modelEntryId: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
            };
            topology: {
                connectedComponentKey: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
            };
            macromolecular: {
                '@header': string;
                authResidueId: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                labelResidueId: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                residueKey: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                chainKey: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                entityKey: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                isHet: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                id: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                label_atom_id: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                label_alt_id: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                label_comp_id: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                label_asym_id: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                label_entity_id: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                label_seq_id: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                auth_atom_id: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                auth_comp_id: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                auth_asym_id: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                auth_seq_id: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                residueSourceIndex: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                pdbx_PDB_ins_code: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                pdbx_formal_charge: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                occupancy: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                B_iso_or_equiv: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                entityType: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                entitySubtype: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                entityPrdId: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                entityDescription: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                objectPrimitive: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                secondaryStructureKey: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                secondaryStructureFlags: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                isModified: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                modifiedParentName: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                isNonStandard: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
                chemCompType: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<unknown>>;
                }>>, import("./type.js").Type>;
            };
            ihm: {
                hasSeqId: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    0: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                }>>, import("./type.js").Type.OneOf<boolean>>;
                overlapsSeqIdRange: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                    beg: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                    end: import("./symbol.js").Argument<import("./type.js").Type.Value<number>>;
                }>>, import("./type.js").Type.OneOf<boolean>>;
            };
        };
        bondProperty: {
            '@header': string;
            flags: MSymbol<import("./symbol.js").Arguments<{}>, import("./type.js").Type>;
            order: MSymbol<import("./symbol.js").Arguments<{}>, import("./type.js").Type>;
            key: MSymbol<import("./symbol.js").Arguments<{}>, import("./type.js").Type>;
            length: MSymbol<import("./symbol.js").Arguments<{}>, import("./type.js").Type>;
            atomA: MSymbol<import("./symbol.js").Arguments<{}>, import("./type.js").Type>;
            atomB: MSymbol<import("./symbol.js").Arguments<{}>, import("./type.js").Type>;
        };
    };
    internal: {
        '@header': string;
        generator: {
            '@header': string;
            bundleElement: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                groupedUnits: import("./symbol.js").Argument<import("./type.js").Type.Any>;
                set: import("./symbol.js").Argument<import("./type.js").Type.Any>;
                ranges: import("./symbol.js").Argument<import("./type.js").Type.Any>;
            }>>, import("./type.js").Type.Any>;
            bundle: MSymbol<import("./symbol.js").Arguments<import("./symbol.js").Arguments.PropTypes<{
                elements: import("./symbol.js").Argument<import("./type.js").Type.Any>;
            }>>, import("./type.js").Type.Container<(env: any) => unknown>>;
            current: MSymbol<import("./symbol.js").Arguments<{}>, import("./type.js").Type.Container<(env: any) => unknown>>;
        };
    };
};
export declare const SymbolList: MSymbol<import("./symbol.js").Arguments<{}>, import("./type.js").Type>[];
export declare const SymbolMap: {
    [id: string]: MSymbol<import("./symbol.js").Arguments<{}>, import("./type.js").Type> | undefined;
};
export { MolScriptSymbolTable };
