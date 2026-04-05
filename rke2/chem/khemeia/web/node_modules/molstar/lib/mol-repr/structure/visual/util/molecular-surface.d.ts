/**
 * Copyright (c) 2019-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Unit, Structure } from '../../../../mol-model/structure.js';
import { Task } from '../../../../mol-task/index.js';
import { CommonSurfaceProps } from './common.js';
import { DensityData } from '../../../../mol-math/geometry.js';
import { MolecularSurfaceCalculationProps } from '../../../../mol-math/geometry/molecular-surface.js';
import { SizeTheme } from '../../../../mol-theme/size.js';
import { ParamDefinition as PD } from '../../../../mol-util/param-definition.js';
export declare const CommonMolecularSurfaceCalculationParams: {
    resolution: {
        label?: string;
        description?: string;
        legend?: import("../../../../mol-util/legend.js").Legend;
        fieldLabels?: {
            [name: string]: string;
        };
        isHidden?: boolean;
        shortLabel?: boolean;
        twoColumns?: boolean;
        isEssential?: boolean;
        category?: string;
        hideIf?: (currentGroup: any) => boolean;
        help?: (value: any) => {
            description?: string;
            legend?: import("../../../../mol-util/legend.js").Legend;
        };
        type: "number";
        immediateUpdate?: boolean;
        isOptional?: boolean;
        defaultValue: number;
        min?: number;
        max?: number;
        step?: number;
    };
    probePositions: {
        label?: string;
        description?: string;
        legend?: import("../../../../mol-util/legend.js").Legend;
        fieldLabels?: {
            [name: string]: string;
        };
        isHidden?: boolean;
        shortLabel?: boolean;
        twoColumns?: boolean;
        isEssential?: boolean;
        category?: string;
        hideIf?: (currentGroup: any) => boolean;
        help?: (value: any) => {
            description?: string;
            legend?: import("../../../../mol-util/legend.js").Legend;
        };
        type: "number";
        immediateUpdate?: boolean;
        isOptional?: boolean;
        defaultValue: number;
        min?: number;
        max?: number;
        step?: number;
    };
    floodfill: PD.Select<"inside" | "outside" | "off">;
    probeRadius: PD.Numeric;
};
export type MolecularSurfaceProps = MolecularSurfaceCalculationProps & CommonSurfaceProps;
export declare function computeUnitMolecularSurface(structure: Structure, unit: Unit, sizeTheme: SizeTheme<any>, props: MolecularSurfaceProps): Task<DensityData>;
export declare function computeStructureMolecularSurface(structure: Structure, sizeTheme: SizeTheme<any>, props: MolecularSurfaceProps): Task<DensityData>;
