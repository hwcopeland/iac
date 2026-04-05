"use strict";
/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.dict = exports.list = exports.tuple = exports.bool = exports.float = exports.int = exports.str = void 0;
exports.object = object;
exports.partial = partial;
exports.union = union;
exports.nullable = nullable;
exports.literal = literal;
exports.RequiredField = RequiredField;
exports.OptionalField = OptionalField;
exports.fieldValidationIssues = fieldValidationIssues;
const tslib_1 = require("tslib");
const iots = tslib_1.__importStar(require("io-ts"));
const json_1 = require("../../../../mol-util/json.js");
/** Type definition for a string  */
exports.str = iots.string;
/** Type definition for an integer  */
exports.int = iots.Integer;
/** Type definition for a float or integer number  */
exports.float = iots.number;
/** Type definition for a boolean  */
exports.bool = iots.boolean;
/** Type definition for a tuple, e.g. `tuple([str, int, int])`  */
exports.tuple = iots.tuple;
/** Type definition for a list/array, e.g. `list(str)`  */
exports.list = iots.array;
/** Type definition for a dictionary/mapping/record, e.g. `dict(str, float)` means type `{ [K in string]: number }` */
exports.dict = iots.record;
function object(props, optionalProps, name) {
    if (!optionalProps) {
        return iots.type(props, name);
    }
    if (name === undefined) {
        const nameChunks = [];
        for (const key in props) {
            nameChunks.push(`${key}: ${props[key].name}`);
        }
        for (const key in optionalProps) {
            nameChunks.push(`${key}?: ${optionalProps[key].name}`);
        }
        name = `{ ${nameChunks.join(', ')} }`;
    }
    return iots.intersection([iots.type(props), iots.partial(optionalProps)], name);
}
/** Type definition used to create partial objects, e.g. `partial({ name: str, age: float })` means type `{ name?: string, age?: number }` */
function partial(props, name) {
    if (name === undefined) {
        const nameChunks = [];
        for (const key in props) {
            nameChunks.push(`${key}?: ${props[key].name}`);
        }
        name = `{ ${nameChunks.join(', ')} }`;
    }
    return iots.partial(props, name);
}
/** Type definition for union types, e.g. `union(str, int)` means string or integer */
function union(first, second, ...others) {
    const baseTypes = [];
    for (const type of [first, second, ...others]) {
        if (type instanceof iots.UnionType) {
            baseTypes.push(...type.types);
        }
        else {
            baseTypes.push(type);
        }
    }
    return iots.union(baseTypes);
}
/** Type definition for nullable types, e.g. `nullable(str)` means string or `null`  */
function nullable(type) {
    return union(type, iots.null);
}
/** Type definition for literal types, e.g. `literal('red', 'green', 'blue')` means 'red' or 'green' or 'blue'.
 *
 * Example usage:
 * ```
 * export type MyColor = 'red' | 'green' | 'blue';
 * export const MyColor = literal<MyColor>('red', 'green', 'blue');
 * ```
 *
 * (it looks stupid to repeat the list of values but it will result in nicer type bundle (for MolViewStories))
 */
function literal(...values) {
    if (values.length === 0) {
        throw new Error(`literal type must have at least one value`);
    }
    const typeName = values.length === 1 ? (0, json_1.onelinerJsonString)(values[0]) : `(${values.map(v => (0, json_1.onelinerJsonString)(v)).join(' | ')})`;
    const valueSet = new Set(values);
    return new iots.Type(typeName, ((value) => valueSet.has(value)), (value, ctx) => valueSet.has(value) ? { _tag: 'Right', right: value } : { _tag: 'Left', left: [{ value: value, context: ctx, message: `"${value}" is not a valid value for literal type ${typeName}` }] }, value => value);
}
function RequiredField(type, description) {
    return { type, required: true, description };
}
function OptionalField(type, defaultValue, description) {
    return { type, required: false, description, default: defaultValue };
}
/** Return `undefined` if `value` has correct type for `field`, regardsless of if required or optional.
 * Return description of validation issues, if `value` has wrong type. */
function fieldValidationIssues(field, value) {
    if (value === undefined && !field.required)
        return undefined; // Value undefined treated as if field not even present (unlike null)
    const validation = field.type.decode(value);
    if (validation._tag === 'Right') {
        return undefined;
    }
    else {
        return reportErrors(validation.left);
    }
}
// Inlining `reportErrors` instead of `import { PathReporter } from 'io-ts/PathReporter'`;
// because it breaks Deno usage.
function reportErrors(errors) {
    if (errors.length === 0)
        return undefined;
    return errors.map(getMessage);
}
function getMessage(e) {
    return e.message !== undefined
        ? e.message
        : `Invalid value ${stringifyError(e.value)} supplied to ${getContextPath(e.context)}`;
}
function getContextPath(context) {
    return context.map(a => `${a.key}: ${a.type.name}`).join('/');
}
function getFunctionName(f) {
    return f.displayName || f.name || `<function ${f.length}>`;
}
function stringifyError(v) {
    if (typeof v === 'function') {
        return getFunctionName(v);
    }
    if (typeof v === 'number' && !isFinite(v)) {
        if (isNaN(v)) {
            return 'NaN';
        }
        return v > 0 ? 'Infinity' : '-Infinity';
    }
    return JSON.stringify(v);
}
