/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { Structure } from '../../mol-model/structure.js';
export declare function molfileToJSONCif(molfile: string): Promise<{
    structure: Structure;
    molfile: import("../../mol-io/reader/mol/parser.js").MolFile;
    jsoncif: import("./model.js").JSONCifFile;
}>;
