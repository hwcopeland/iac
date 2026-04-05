/**
 * Copyright (c) 2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { BehaviorSubject } from 'rxjs';
import { Model } from '../../../../mol-model/structure.js';
import { CollapsableControls, CollapsableState } from '../../../../mol-plugin-ui/base.js';
import { PluginContext } from '../../../../mol-plugin/context.js';
import { StateTransform } from '../../../../mol-state/index.js';
import { ParamDefinition as PD } from '../../../../mol-util/param-definition.js';
import { SingleAsyncQueue } from '../../../../mol-util/single-async-queue.js';
import { QualityAssessment } from '../prop.js';
import { MAPairwiseMetricDrawing } from './plot.js';
type State = ReturnType<typeof getPropsAndValues>;
export declare class MAPairwiseScorePlotPanel extends CollapsableControls<{}, State> {
    protected defaultState(): State & CollapsableState;
    toggleCollapsed(): void;
    interactivity: BehaviorSubject<PlotInteractivityState>;
    queue: SingleAsyncQueue;
    componentDidMount(): void;
    protected renderControls(): JSX.Element | null;
}
export declare function MAPairwiseScorePlot({ plugin, model, pairwiseMetric }: {
    plugin: PluginContext;
    model: Model;
    pairwiseMetric: QualityAssessment.Pairwise;
}): import("react/jsx-runtime").JSX.Element;
declare function getPropsAndValues(plugin: PluginContext, current?: {
    model?: string;
    data?: string;
}): {
    params: {
        model: PD.Select<string>;
        data: PD.Select<string>;
    };
    values: {
        model: string;
        data: string;
    };
    dataSources: {
        id: string;
        label: string;
        metridId: number;
        dataRef: StateTransform.Ref;
        blockIndex: number;
    }[];
};
interface PlotInteractivityState {
    model?: Model;
    drawing?: MAPairwiseMetricDrawing;
    crosshairOffset?: [number, number];
    inside?: boolean;
    mouseDown?: boolean;
    boxStart?: [number, number];
    boxEnd?: [number, number];
}
export declare const MAPairwiseScorePlotBase: import("react").MemoExoticComponent<({ model, pairwiseMetric, interactivity }: {
    model: Model;
    pairwiseMetric: QualityAssessment.Pairwise;
    interactivity: BehaviorSubject<PlotInteractivityState>;
}) => import("react/jsx-runtime").JSX.Element>;
export {};
