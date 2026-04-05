/**
 * Copyright (c) 2017-2023 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Mat3 } from '../linear-algebra/3d/mat3.js';
import { Mat4 } from '../linear-algebra/3d/mat4.js';
import { Vec3 } from '../linear-algebra/3d/vec3.js';
interface SymmetryOperator {
    /** Operator name, e.g. 1_555, ASM_1 */
    readonly name: string;
    /** Canonical operator name, must follow symmetry instance naming rules (https://molstar.org/mol-view-spec-docs/selectors/#instance_id).
     * E.g. 1_555, ASM-X0-5 */
    readonly instanceId: string;
    readonly assembly?: {
        /** pointer to `pdbx_struct_assembly.id` or empty string */
        readonly id: string;
        /** pointers to `pdbx_struct_oper_list.id` or empty list */
        readonly operList: string[];
        /** (arbitrary) unique id of the operator to be used in suffix */
        readonly operId: number;
    };
    /** pointer to `struct_ncs_oper.id` or -1 */
    readonly ncsId: number;
    readonly hkl: Vec3;
    /** spacegroup symmetry operator index, -1 if not applicable */
    readonly spgrOp: number;
    /** unique (external) key, -1 if not available */
    readonly key: number;
    readonly matrix: Mat4;
    /** cache the inverse of the transform */
    readonly inverse: Mat4;
    /** optimize the identity case */
    readonly isIdentity: boolean;
    /**
     * Suffix based on operator type.
     * - Assembly: _assembly.operId
     * - Crystal: -op_ijk
     * - ncs: _ncsId
     */
    readonly suffix: string;
}
declare namespace SymmetryOperator {
    const DefaultName = "1_555";
    const Default: SymmetryOperator;
    const RotationTranslationEpsilon = 0.005;
    type CreateInfo = {
        assembly?: SymmetryOperator['assembly'];
        ncsId?: number;
        hkl?: Vec3;
        spgrOp?: number;
        key?: number;
        instanceId?: string;
    };
    function create(name: string, matrix: Mat4, info?: CreateInfo | SymmetryOperator): SymmetryOperator;
    /** Create spacegroup symmetry operator name, e.g. getSymmetryOperatorName(0, 0, 0, 0) -> '1_555' */
    function getSymmetryOperatorName(spgrOp: number, i: number, j: number, k: number): string;
    /** Create assembly symmetry operator name, e.g. getAssemblyOperatorName(['X0', '5']) -> 'X0-5' */
    function getAssemblyOperatorName(operatorIds: string[]): string;
    function checkIfRotationAndTranslation(rot: Mat3, offset: Vec3): boolean;
    function ofRotationAndOffset(name: string, rot: Mat3, offset: Vec3, ncsId?: number): SymmetryOperator;
    function lerpFromIdentity(out: Mat4, op: SymmetryOperator, t: number): Mat4;
    function slerp(out: Mat4, src: Mat4, tar: Mat4, t: number): Mat4;
    /**
     * Apply the 1st and then 2nd operator. ( = second.matrix * first.matrix).
     * Keep `name`, `assembly`, `ncsId`, `hkl` and `spgrOpId` properties from second.
     */
    function compose(first: SymmetryOperator, second: SymmetryOperator): SymmetryOperator;
    interface CoordinateMapper<T extends number> {
        (index: T, slot: Vec3): Vec3;
    }
    interface ArrayMapping<T extends number> {
        readonly coordinates: Coordinates;
        readonly operator: SymmetryOperator;
        invariantPosition(this: ArrayMapping<T>, i: T, s: Vec3): Vec3;
        position(this: ArrayMapping<T>, i: T, s: Vec3): Vec3;
        x(this: ArrayMapping<T>, index: T): number;
        y(this: ArrayMapping<T>, index: T): number;
        z(this: ArrayMapping<T>, index: T): number;
        readonly r: (this: ArrayMapping<T>, index: T) => number;
    }
    interface Coordinates {
        x: ArrayLike<number>;
        y: ArrayLike<number>;
        z: ArrayLike<number>;
    }
    function createMapping<T extends number>(operator: SymmetryOperator, coords: Coordinates, radius?: ((index: T) => number)): ArrayMapping<T>;
}
export { SymmetryOperator };
