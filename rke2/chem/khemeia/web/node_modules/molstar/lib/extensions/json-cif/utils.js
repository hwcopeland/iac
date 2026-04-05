/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { parseMol } from '../../mol-io/reader/mol/parser.js';
import { trajectoryFromMol } from '../../mol-model-formats/structure/mol.js';
import { Structure, to_mmCIF } from '../../mol-model/structure.js';
import { Task } from '../../mol-task/index.js';
import { JSONCifEncoder } from './encoder.js';
export async function molfileToJSONCif(molfile) {
    const parsed = await parseMol(molfile).run();
    if (parsed.isError)
        throw new Error(parsed.message);
    const models = await trajectoryFromMol(parsed.result).run();
    const model = await Task.resolveInContext(models.getFrameAtIndex(0));
    const structure = Structure.ofModel(model);
    const encoder = new JSONCifEncoder('Mol*', { formatJSON: true });
    to_mmCIF('mol', structure, false, {
        encoder,
        includedCategoryNames: new Set(['atom_site']),
        extensions: {
            molstar_bond_site: true,
        }
    });
    return {
        structure,
        molfile: parsed.result,
        jsoncif: encoder.getFile()
    };
}
