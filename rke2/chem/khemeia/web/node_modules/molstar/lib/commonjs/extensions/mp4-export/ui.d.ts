/**
 * Copyright (c) 2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { CollapsableControls, CollapsableState } from '../../mol-plugin-ui/base.js';
import { Mp4Controls } from './controls.js';
interface State {
    busy?: boolean;
    data?: {
        movie: Uint8Array<ArrayBuffer>;
        filename: string;
    };
}
export declare class Mp4EncoderUI extends CollapsableControls<{}, State> {
    private _controls;
    get controls(): Mp4Controls;
    protected defaultState(): State & CollapsableState;
    private downloadControls;
    protected renderControls(): JSX.Element | null;
    componentDidMount(): void;
    componentWillUnmount(): void;
    save: () => void;
    generate: () => Promise<void>;
}
export {};
