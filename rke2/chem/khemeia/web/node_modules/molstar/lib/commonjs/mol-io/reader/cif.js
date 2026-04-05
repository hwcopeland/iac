"use strict";
/**
 * Copyright (c) 2017-2022 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.CIF = void 0;
const tslib_1 = require("tslib");
const parser_1 = require("./cif/text/parser.js");
const parser_2 = require("./cif/binary/parser.js");
const schema_1 = require("./cif/schema.js");
const mmcif_1 = require("./cif/schema/mmcif.js");
const ccd_1 = require("./cif/schema/ccd.js");
const bird_1 = require("./cif/schema/bird.js");
const dic_1 = require("./cif/schema/dic.js");
const density_server_1 = require("./cif/schema/density-server.js");
const cif_core_1 = require("./cif/schema/cif-core.js");
const segmentation_1 = require("./cif/schema/segmentation.js");
const string_like_1 = require("../common/string-like.js");
exports.CIF = {
    parse: (data) => string_like_1.StringLike.is(data) ? (0, parser_1.parseCifText)(data) : (0, parser_2.parseCifBinary)(data),
    parseText: parser_1.parseCifText,
    parseBinary: parser_2.parseCifBinary,
    toDatabaseCollection: schema_1.toDatabaseCollection,
    toDatabase: schema_1.toDatabase,
    schema: {
        mmCIF: (frame) => (0, schema_1.toDatabase)(mmcif_1.mmCIF_Schema, frame),
        CCD: (frame) => (0, schema_1.toDatabase)(ccd_1.CCD_Schema, frame),
        BIRD: (frame) => (0, schema_1.toDatabase)(bird_1.BIRD_Schema, frame),
        dic: (frame) => (0, schema_1.toDatabase)(dic_1.dic_Schema, frame),
        cifCore: (frame) => (0, schema_1.toDatabase)(cif_core_1.CifCore_Schema, frame, cif_core_1.CifCore_Aliases),
        densityServer: (frame) => (0, schema_1.toDatabase)(density_server_1.DensityServer_Data_Schema, frame),
        segmentation: (frame) => (0, schema_1.toDatabase)(segmentation_1.Segmentation_Data_Schema, frame),
    }
};
tslib_1.__exportStar(require("./cif/data-model.js"), exports);
