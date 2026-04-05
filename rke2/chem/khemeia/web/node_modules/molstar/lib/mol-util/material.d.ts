/**
 * Copyright (c) 2021-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { NumberArray } from './type-helpers.js';
import { ParamDefinition as PD } from './param-definition.js';
export interface Material {
    /** Normalized to [0, 1] range */
    metalness: number;
    /** Normalized to [0, 1] range */
    roughness: number;
    /** Normalized to [0, 1] range */
    bumpiness: number;
}
export declare function Material(values?: Partial<Material>): {
    metalness: number;
    roughness: number;
    bumpiness: number;
};
export declare namespace Material {
    const Zero: Material;
    function toArray<T extends NumberArray>(material: Material, array: T, offset: number): T;
    function toArrayNormalized<T extends NumberArray>(material: Material, array: T, offset: number): T;
    function areEqual(a: Material, b: Material): boolean;
    function toString({ metalness, roughness, bumpiness }: Material): string;
    function getParam(info?: {
        isExpanded?: boolean;
        isFlat?: boolean;
    }): PD.Group<PD.Normalize<{
        metalness: number;
        roughness: number;
        bumpiness: number;
    }>>;
}
