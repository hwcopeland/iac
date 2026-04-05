/**
 * Copyright (c) 2019-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author Kim Juho <juho_kim@outlook.com>
 */
import { CifField } from '../../../mol-io/reader/cif.js';
import { mmCIF_Schema } from '../../../mol-io/reader/cif/schema/mmcif.js';
import { Tokenizer } from '../../../mol-io/reader/common/text/tokenizer.js';
import { StringLike } from '../../../mol-io/common/string-like.js';
type AnisotropicTemplate = typeof getAnisotropicTemplate extends (...args: any) => infer T ? T : never;
export declare function getAnisotropicTemplate(data: StringLike, count: number): {
    index: number;
    count: number;
    id: string[];
    type_symbol: import("../../../mol-io/reader/common/text/tokenizer.js").Tokens;
    pdbx_label_atom_id: import("../../../mol-io/reader/common/text/tokenizer.js").Tokens;
    pdbx_label_alt_id: import("../../../mol-io/reader/common/text/tokenizer.js").Tokens;
    pdbx_label_comp_id: import("../../../mol-io/reader/common/text/tokenizer.js").Tokens;
    pdbx_label_asym_id: import("../../../mol-io/reader/common/text/tokenizer.js").Tokens;
    pdbx_label_seq_id: import("../../../mol-io/reader/common/text/tokenizer.js").Tokens;
    pdbx_PDB_ins_code: import("../../../mol-io/reader/common/text/tokenizer.js").Tokens;
    'U[1][1]': Float32Array<ArrayBuffer>;
    'U[2][2]': Float32Array<ArrayBuffer>;
    'U[3][3]': Float32Array<ArrayBuffer>;
    'U[1][2]': Float32Array<ArrayBuffer>;
    'U[1][3]': Float32Array<ArrayBuffer>;
    'U[2][3]': Float32Array<ArrayBuffer>;
    pdbx_auth_seq_id: import("../../../mol-io/reader/common/text/tokenizer.js").Tokens;
    pdbx_auth_comp_id: import("../../../mol-io/reader/common/text/tokenizer.js").Tokens;
    pdbx_auth_asym_id: import("../../../mol-io/reader/common/text/tokenizer.js").Tokens;
    pdbx_auth_atom_id: import("../../../mol-io/reader/common/text/tokenizer.js").Tokens;
};
export declare function getAnisotropic(sites: AnisotropicTemplate): {
    [K in keyof mmCIF_Schema['atom_site_anisotrop']]?: CifField;
};
export declare function addAnisotropic(sites: AnisotropicTemplate, model: string, data: Tokenizer, s: number, e: number): void;
export {};
