import { Draft, Immutable } from '../interface';
/**
 * Cast a value to an Draft type value.
 */
export declare function castDraft<T>(value: T): Draft<T>;
/**
 * Cast a value to an Immutable type value.
 */
export declare function castImmutable<T>(value: T): Immutable<T>;
/**
 * Cast a value to an Mutable type value.
 */
export declare function castMutable<T>(draft: Draft<T>): T;
