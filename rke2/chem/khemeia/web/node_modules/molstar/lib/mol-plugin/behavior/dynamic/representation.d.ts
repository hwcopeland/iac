/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author Jason Pattle <jpattle.exscientia.co.uk>
 * @author Adam Midlik <midlik@gmail.com>
 */
import { Binding } from '../../../mol-util/binding.js';
export declare const HighlightLoci: import("../../../mol-state/index.js").StateTransformer<any, any, any>;
export declare const DefaultSelectLociBindings: {
    clickSelect: Binding;
    clickSelectOnly: Binding;
    clickToggle: Binding;
    clickToggleExtend: Binding;
    clickDeselect: Binding;
    clickDeselectAllOnEmpty: Binding;
};
export declare const SelectLoci: import("../../../mol-state/index.js").StateTransformer<any, any, any>;
export declare const DefaultLociLabelProvider: import("../../../mol-state/index.js").StateTransformer<any, any, any>;
export declare const DefaultFocusLociBindings: {
    clickFocus: Binding;
    clickFocusAdd: Binding;
    clickFocusExtend: Binding;
    clickFocusSelectMode: Binding;
    clickFocusAddSelectMode: Binding;
    clickFocusExtendSelectMode: Binding;
};
export declare const FocusLoci: import("../../../mol-state/index.js").StateTransformer<any, any, any>;
