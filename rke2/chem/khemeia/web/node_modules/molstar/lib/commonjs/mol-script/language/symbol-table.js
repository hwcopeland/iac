"use strict";
/**
 * Copyright (c) 2017 Mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.MolScriptSymbolTable = exports.SymbolMap = exports.SymbolList = void 0;
const core_1 = require("./symbol-table/core.js");
const structure_query_1 = require("./symbol-table/structure-query.js");
const internal_1 = require("./symbol-table/internal.js");
const helpers_1 = require("./helpers.js");
const MolScriptSymbolTable = { core: core_1.core, structureQuery: structure_query_1.structureQuery, internal: internal_1.internal };
exports.MolScriptSymbolTable = MolScriptSymbolTable;
(0, helpers_1.normalizeTable)(MolScriptSymbolTable);
exports.SymbolList = (0, helpers_1.symbolList)(MolScriptSymbolTable);
exports.SymbolMap = (function () {
    const map = Object.create(null);
    for (const s of exports.SymbolList)
        map[s.id] = s;
    return map;
})();
