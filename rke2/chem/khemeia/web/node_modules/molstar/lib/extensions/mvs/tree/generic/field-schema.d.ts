/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import * as iots from 'io-ts';
/** All types that can be used in tree node params.
 * Can be extended, this is just to list them all in one place and possibly catch some typing errors */
type AllowedValueTypes = string | number | boolean | null | [number, number, number] | string[] | number[] | {};
/** Type definition for a string  */
export declare const str: iots.StringC;
/** Type definition for an integer  */
export declare const int: iots.RefinementC<iots.NumberC, number>;
/** Type definition for a float or integer number  */
export declare const float: iots.NumberC;
/** Type definition for a boolean  */
export declare const bool: iots.BooleanC;
/** Type definition for a tuple, e.g. `tuple([str, int, int])`  */
export declare const tuple: typeof iots.tuple;
/** Type definition for a list/array, e.g. `list(str)`  */
export declare const list: typeof iots.array;
/** Type definition for a dictionary/mapping/record, e.g. `dict(str, float)` means type `{ [K in string]: number }` */
export declare const dict: typeof iots.record;
/** Type definition used to create objects, e.g. `object({ name: str, age: float }, { address: str })` means type `{ name: string, age: number, address?: string }` */
export declare function object<P extends iots.Props, Q extends iots.Props>(props: P, optionalProps: undefined, name?: string): iots.TypeC<P>;
export declare function object<P extends iots.Props, Q extends iots.Props>(props: P, optionalProps: Q, name?: string): iots.IntersectionC<[iots.TypeC<P>, iots.PartialC<Q>]>;
/** Type definition used to create partial objects, e.g. `partial({ name: str, age: float })` means type `{ name?: string, age?: number }` */
export declare function partial<P extends iots.Props>(props: P, name?: string): iots.PartialC<P>;
/** Type definition for union types, e.g. `union(str, int)` means string or integer */
export declare function union<T1 extends iots.Mixed, T2 extends iots.Mixed, TOthers extends iots.Mixed[]>(first: T1, second: T2, ...others: TOthers): iots.UnionC<[T1, T2, ...TOthers]>;
/** Type definition for nullable types, e.g. `nullable(str)` means string or `null`  */
export declare function nullable<V>(type: iots.Type<V>): iots.Type<V | null>;
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
export declare function literal<V extends string | number | boolean>(...values: V[]): iots.Type<V, V, unknown>;
interface FieldBase<V extends AllowedValueTypes = any, R extends boolean = boolean> {
    /** Definition of allowed types for the field */
    type: iots.Type<V>;
    /** If `required===true`, the value must always be defined in molviewspec format (can be `null` if `type` allows it).
     * If `required===false`, the value can be ommitted (meaning that a default should be used).
     * If `type` allows `null`, the default must be `null`. */
    required: R;
    /** Description of what the field value means */
    description: string;
}
/** Schema for param field which must always be provided (has no default value) */
export interface RequiredField<V extends AllowedValueTypes = any> extends FieldBase<V> {
    required: true;
}
export declare function RequiredField<V extends AllowedValueTypes>(type: iots.Type<V>, description: string): RequiredField<V>;
/** Schema for param field which can be dropped (meaning that a default value will be used) */
export interface OptionalField<V extends AllowedValueTypes = any> extends FieldBase<V> {
    required: false;
    /** Default value for optional field.
     * If field type allows `null`, default must be `null` (this is to avoid issues in languages that do not distinguish `null` and `undefined`). */
    default: DefaultValue<V>;
}
export declare function OptionalField<V extends AllowedValueTypes>(type: iots.Type<V>, defaultValue: DefaultValue<V>, description: string): OptionalField<V>;
/** Schema for one field in params (i.e. a value in a top-level key-value pair) */
export type Field<V extends AllowedValueTypes = any> = RequiredField<V> | OptionalField<V>;
/** Type of valid default value for value type `V` (if the type allows `null`, the default must be `null`) */
type DefaultValue<V extends AllowedValueTypes> = null extends V ? null : V;
/** Type of valid value for field of type `F` (never includes `undefined`, even if field is optional) */
export type ValueFor<F extends Field | iots.Any> = F extends Field<infer V> ? V : F extends iots.Any ? iots.TypeOf<F> : never;
/** Return `undefined` if `value` has correct type for `field`, regardsless of if required or optional.
 * Return description of validation issues, if `value` has wrong type. */
export declare function fieldValidationIssues<F extends Field>(field: F, value: any): string[] | undefined;
export {};
