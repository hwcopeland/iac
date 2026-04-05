/**
 * Copyright (c) 2017-2022 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { parseCifText } from './cif/text/parser.js';
import { parseCifBinary } from './cif/binary/parser.js';
import { CifFrame } from './cif/data-model.js';
import { toDatabaseCollection, toDatabase } from './cif/schema.js';
import { mmCIF_Database } from './cif/schema/mmcif.js';
import { CCD_Database } from './cif/schema/ccd.js';
import { BIRD_Database } from './cif/schema/bird.js';
import { dic_Database } from './cif/schema/dic.js';
import { DensityServer_Data_Database } from './cif/schema/density-server.js';
import { CifCore_Database } from './cif/schema/cif-core.js';
import { Segmentation_Data_Database } from './cif/schema/segmentation.js';
import { StringLike } from '../common/string-like.js';
export declare const CIF: {
    parse: (data: StringLike | Uint8Array) => import("../../mol-task/index.js").Task<import("./result.js").ReaderResult<import("./cif/data-model.js").CifFile>>;
    parseText: typeof parseCifText;
    parseBinary: typeof parseCifBinary;
    toDatabaseCollection: typeof toDatabaseCollection;
    toDatabase: typeof toDatabase;
    schema: {
        mmCIF: (frame: CifFrame) => mmCIF_Database;
        CCD: (frame: CifFrame) => CCD_Database;
        BIRD: (frame: CifFrame) => BIRD_Database;
        dic: (frame: CifFrame) => dic_Database;
        cifCore: (frame: CifFrame) => CifCore_Database;
        densityServer: (frame: CifFrame) => DensityServer_Data_Database;
        segmentation: (frame: CifFrame) => Segmentation_Data_Database;
    };
};
export * from './cif/data-model.js';
