/**
 * Copyright (c) 2018-2022 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
import { Segment } from './volseg-api/data.js';
import { VolsegEntryData } from './entry-root.js';
export declare class VolsegMeshSegmentationData {
    private entryData;
    constructor(rootData: VolsegEntryData);
    loadSegmentation(): Promise<void>;
    updateOpacity(opacity: number): Promise<import("../../mol-state/index.js").StateObjectSelector<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>>;
    highlightSegment(segment: Segment): Promise<void>;
    selectSegment(segment?: number): Promise<void>;
    /** Make visible the specified set of mesh segments */
    showSegments(segments: number[]): Promise<void>;
}
