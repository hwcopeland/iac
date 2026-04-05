/**
 * Copyright (c) 2019-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
export declare enum StructureFocusRepresentationTags {
    TargetSel = "structure-focus-target-sel",
    TargetRepr = "structure-focus-target-repr",
    SurrSel = "structure-focus-surr-sel",
    SurrRepr = "structure-focus-surr-repr",
    SurrNciRepr = "structure-focus-surr-nci-repr"
}
export declare const StructureFocusRepresentation: import("../../../../mol-state/index.js").StateTransformer<any, any, any>;
