"use strict";
/**
 * Copyright (c) 2023-2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.getParams = getParams;
exports.getCustomProps = getCustomProps;
exports.getChildren = getChildren;
exports.TreeSchema = TreeSchema;
exports.TreeSchemaWithAllRequired = TreeSchemaWithAllRequired;
const object_1 = require("../../../../mol-util/object.js");
const params_schema_1 = require("./params-schema.js");
/** Get params from a tree node */
function getParams(node) {
    var _a;
    return (_a = node.params) !== null && _a !== void 0 ? _a : {};
}
/** Get custom properties from a tree node */
function getCustomProps(node) {
    var _a;
    return ((_a = node.custom) !== null && _a !== void 0 ? _a : {});
}
/** Get children from a tree node */
function getChildren(tree) {
    var _a;
    return (_a = tree.children) !== null && _a !== void 0 ? _a : [];
}
function TreeSchema(schema) {
    return schema;
}
function TreeSchemaWithAllRequired(schema) {
    return {
        ...schema,
        nodes: (0, object_1.mapObjectMap)(schema.nodes, node => ({ ...node, params: (0, params_schema_1.AllRequired)(node.params) })),
    };
}
