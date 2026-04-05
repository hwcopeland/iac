/**
 * Copyright (c) 2017-2018 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { mmCIF_Database, mmCIF_Schema } from '../../../../mol-io/reader/cif/schema/mmcif.js';
import { Model } from '../../model.js';
import { Structure } from '../../structure.js';
import { EntityIndex } from '../../model/indexing.js';
import { CifWriter } from '../../../../mol-io/writer/cif.js';
import { CifExportContext } from '../mmcif.js';
import { CifCategory } from '../../../../mol-io/reader/cif.js';
export declare function getModelMmCifCategory<K extends keyof mmCIF_Schema>(model: Model, name: K): mmCIF_Database[K] | undefined;
export declare function getUniqueResidueNamesFromStructures(structures: Structure[]): Set<string>;
export declare function getUniqueEntityIdsFromStructures(structures: Structure[]): Set<string>;
export declare function getUniqueEntityIndicesFromStructures(structures: Structure[]): ReadonlyArray<EntityIndex>;
export declare function copy_mmCif_category(name: keyof mmCIF_Schema, condition?: (structure: Structure) => boolean): CifWriter.Category<CifExportContext>;
export declare function copy_source_mmCifCategory(encoder: CifWriter.Encoder, ctx: CifExportContext, category: CifCategory): CifWriter.Category<CifExportContext> | undefined;
