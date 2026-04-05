/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author Jason Pattle <jpattle.exscientia.co.uk>
 */
import { Binding } from '../../../mol-util/binding.js';
export declare const DefaultClickResetCameraOnEmpty: Binding;
export declare const DefaultClickResetCameraOnEmptySelectMode: Binding;
type FocusLociBindings = {
    clickCenterFocus: Binding;
    clickCenterFocusSelectMode: Binding;
    clickResetCameraOnEmpty?: Binding;
    clickResetCameraOnEmptySelectMode?: Binding;
};
export declare const DefaultFocusLociBindings: FocusLociBindings;
export declare const NoPrimaryFocusLociBindings: FocusLociBindings;
export declare const FocusLoci: import("../../../mol-state/index.js").StateTransformer<any, any, any>;
export declare const CameraAxisHelper: import("../../../mol-state/index.js").StateTransformer<any, any, any>;
export declare const CameraControls: import("../../../mol-state/index.js").StateTransformer<any, any, any>;
export {};
