/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
import { ParamDefinition as PD } from '../../../mol-util/param-definition.js';
/** Similar to `PD.Numeric` but allows leaving empty field in UI (treated as `undefined`), and can only have integer values */
export declare function MaybeIntegerParamDefinition(info?: PD.Info & {
    placeholder?: string;
}): PD.Converted<number | null, string>;
/** Similar to `PD.Numeric` but allows leaving empty field in UI (treated as `undefined`) */
export declare function MaybeFloatParamDefinition(info?: PD.Info & {
    placeholder?: string;
}): PD.Converted<number | null, string>;
/** Similar to `PD.Text` but leaving empty field in UI is treated as `undefined` */
export declare function MaybeStringParamDefinition(info?: PD.Info & {
    placeholder?: string;
}): PD.Converted<string | null, string>;
