/**
 * Copyright (c) 2019-2022 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Adam Midlik <midlik@gmail.com>
 */
import { PluginUIComponent } from '../base.js';
import { StateTransformParameters } from '../state/common.js';
import { ParamOnChange } from '../controls/parameters.js';
import { Grid } from '../../mol-model/volume.js';
export declare class VolumeStreamingCustomControls extends PluginUIComponent<StateTransformParameters.Props> {
    private areInitial;
    private newParams;
    changeIso: (name: string, value: number, isRelative: boolean) => void;
    changeParams: (name: string, param: string, value: any) => void;
    convert(channel: any, stats: Grid['stats'], isRelative: boolean): any;
    changeOption: ParamOnChange;
    render(): import("react/jsx-runtime").JSX.Element | null;
}
