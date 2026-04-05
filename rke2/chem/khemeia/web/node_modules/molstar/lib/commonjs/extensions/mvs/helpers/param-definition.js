"use strict";
/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.MaybeIntegerParamDefinition = MaybeIntegerParamDefinition;
exports.MaybeFloatParamDefinition = MaybeFloatParamDefinition;
exports.MaybeStringParamDefinition = MaybeStringParamDefinition;
const param_definition_1 = require("../../../mol-util/param-definition.js");
/** Similar to `PD.Numeric` but allows leaving empty field in UI (treated as `undefined`), and can only have integer values */
function MaybeIntegerParamDefinition(info) {
    var _a;
    const defaultValue = null; // the default must be null, otherwise real nulls would be replaced by the default
    return param_definition_1.ParamDefinition.Converted(stringifyMaybeInt, parseMaybeInt, param_definition_1.ParamDefinition.Text(stringifyMaybeInt(defaultValue), { ...info, disableInteractiveUpdates: true, placeholder: (_a = info === null || info === void 0 ? void 0 : info.placeholder) !== null && _a !== void 0 ? _a : 'null' }));
}
function parseMaybeInt(input) {
    const num = parseInt(input);
    return isNaN(num) ? null : num;
}
function stringifyMaybeInt(num) {
    if (num === null)
        return '';
    return num.toString();
}
/** Similar to `PD.Numeric` but allows leaving empty field in UI (treated as `undefined`) */
function MaybeFloatParamDefinition(info) {
    var _a;
    const defaultValue = null; // the default must be null, otherwise real nulls would be replaced by the default
    return param_definition_1.ParamDefinition.Converted(stringifyMaybeFloat, parseMaybeFloat, param_definition_1.ParamDefinition.Text(stringifyMaybeFloat(defaultValue), { ...info, disableInteractiveUpdates: true, placeholder: (_a = info === null || info === void 0 ? void 0 : info.placeholder) !== null && _a !== void 0 ? _a : 'null' }));
}
function parseMaybeFloat(input) {
    const num = parseFloat(input);
    return isNaN(num) ? null : num;
}
function stringifyMaybeFloat(num) {
    if (num === null)
        return '';
    return num.toString();
}
/** Similar to `PD.Text` but leaving empty field in UI is treated as `undefined` */
function MaybeStringParamDefinition(info) {
    var _a;
    const defaultValue = null; // the default must be null, otherwise real nulls would be replaced by the default
    return param_definition_1.ParamDefinition.Converted(stringifyMaybeString, parseMaybeString, param_definition_1.ParamDefinition.Text(stringifyMaybeString(defaultValue), { ...info, placeholder: (_a = info === null || info === void 0 ? void 0 : info.placeholder) !== null && _a !== void 0 ? _a : 'null' }));
}
function parseMaybeString(input) {
    return input === '' ? null : input;
}
function stringifyMaybeString(str) {
    return str === null ? '' : str;
}
