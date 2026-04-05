"use strict";
/**
 * Copyright (c) 2019 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.StateActions = void 0;
const tslib_1 = require("tslib");
const Structure = tslib_1.__importStar(require("./actions/structure.js"));
const Volume = tslib_1.__importStar(require("./actions/volume.js"));
const DataFormat = tslib_1.__importStar(require("./actions/file.js"));
exports.StateActions = {
    Structure,
    Volume,
    DataFormat
};
