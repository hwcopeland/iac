/**
 * Copyright (c) 2018 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { State, StateTransform, StateTransformer } from '../../mol-state/index.js';
import { TransformControlBase } from './common.js';
import { Observable } from 'rxjs';
import { PluginUIComponent } from '../base.js';
export { UpdateTransformControl, TransformUpdaterControl };
declare namespace UpdateTransformControl {
    interface Props {
        transform: StateTransform;
        state: State;
        toggleCollapsed?: Observable<any>;
        initiallyCollapsed?: boolean;
        customHeader?: StateTransformer.Definition['display'] | 'none';
        customUpdate?: (params: any) => Promise<any>;
    }
    interface ComponentState extends TransformControlBase.ComponentState {
    }
}
declare class UpdateTransformControl extends TransformControlBase<UpdateTransformControl.Props, UpdateTransformControl.ComponentState> {
    applyAction(): Promise<any>;
    getInfo(): {
        params: import("../../mol-util/param-definition.js").ParamDefinition.Params;
        initialValues: any;
        isEmpty: boolean;
    };
    getTransformerId(): StateTransformer.Id;
    getHeader(): "none" | {
        readonly name: string;
        readonly description?: string;
    };
    canApply(): boolean;
    applyText(): "Update" | "Nothing to Update";
    isUpdate(): boolean;
    getSourceAndTarget(): {
        a: import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>> | undefined;
        b: import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>> | undefined;
        bCell: import("../../mol-state/index.js").StateObjectCell<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, StateTransform<StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>> | undefined;
    };
    canAutoApply(newParams: any): boolean;
    componentDidMount(): void;
    private _getInfo;
    state: UpdateTransformControl.ComponentState;
    componentDidUpdate(prevProps: UpdateTransformControl.Props): void;
}
declare class TransformUpdaterControl extends PluginUIComponent<{
    nodeRef: string;
    initiallyCollapsed?: boolean;
    header?: StateTransformer.Definition['display'];
}> {
    componentDidMount(): void;
    render(): import("react/jsx-runtime").JSX.Element | null;
}
