/**
 * Copyright (c) 2023-2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
import { mapObjectMap } from '../../../../mol-util/object.js';
import { AllRequired } from './params-schema.js';
/** Get params from a tree node */
export function getParams(node) {
    var _a;
    return (_a = node.params) !== null && _a !== void 0 ? _a : {};
}
/** Get custom properties from a tree node */
export function getCustomProps(node) {
    var _a;
    return ((_a = node.custom) !== null && _a !== void 0 ? _a : {});
}
/** Get children from a tree node */
export function getChildren(tree) {
    var _a;
    return (_a = tree.children) !== null && _a !== void 0 ? _a : [];
}
export function TreeSchema(schema) {
    return schema;
}
export function TreeSchemaWithAllRequired(schema) {
    return {
        ...schema,
        nodes: mapObjectMap(schema.nodes, node => ({ ...node, params: AllRequired(node.params) })),
    };
}
