"use strict";
/**
 * Copyright (c) 2019-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.DataFormatProvider = DataFormatProvider;
exports.guessCifVariant = guessCifVariant;
const decode_1 = require("../../mol-io/common/msgpack/decode.js");
function DataFormatProvider(provider) { return provider; }
function guessCifVariant(info, data) {
    if (info.ext === 'bcif') {
        try {
            // TODO: find a way to run msgpackDecode only once
            //       now it is run twice, here and during file parsing
            const file = (0, decode_1.decodeMsgPack)(data);
            if (file.encoder.startsWith('VolumeServer'))
                return 'dscif';
            // Assumes volseg-volume-server only serves segments
            if (file.encoder.startsWith('volseg-volume-server'))
                return 'segcif';
            if (bcifHasCategory(file, 'volume_data_3d_info')) {
                if (bcifHasCategory(file, 'volume_data_3d'))
                    return 'dscif';
                if (bcifHasCategory(file, 'segmentation_data_3d'))
                    return 'segcif';
            }
        }
        catch (e) {
            console.error(e);
        }
    }
    else if (info.ext === 'cif') {
        const str = data;
        if (str.startsWith('data_SERVER\n#\n_density_server_result'))
            return 'dscif';
        if (str.startsWith('data_SERVER\n#\ndata_SEGMENTATION_DATA'))
            return 'segcif';
        if (cifHasCategory(str, 'volume_data_3d_info')) {
            if (cifHasCategory(str, 'volume_data_3d'))
                return 'dscif';
            if (cifHasCategory(str, 'segmentation_data_3d'))
                return 'segcif';
        }
        if (str.includes('atom_site_fract_x') || str.includes('atom_site.fract_x'))
            return 'coreCif';
    }
    return -1;
}
function cifHasCategory(file, categoryName) {
    return file.includes(`_${categoryName}.`);
}
function bcifHasCategory(file, categoryName) {
    for (const block of file.dataBlocks) {
        for (const category of block.categories) {
            if (category.name === categoryName)
                return true;
        }
    }
    return false;
}
