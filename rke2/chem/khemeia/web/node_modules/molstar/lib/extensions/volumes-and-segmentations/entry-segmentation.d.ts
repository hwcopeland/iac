/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
import { Segment } from './volseg-api/data.js';
import { VolsegEntryData } from './entry-root.js';
export declare class VolsegLatticeSegmentationData {
    private entryData;
    constructor(rootData: VolsegEntryData);
    loadSegmentation(): Promise<void>;
    private createPalette;
    updateOpacity(opacity: number): Promise<import("../../mol-state/index.js").StateObjectSelector<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>>;
    private makeLoci;
    highlightSegment(segment: Segment): Promise<void>;
    selectSegment(segment?: number): Promise<void>;
    /** Make visible the specified set of lattice segments */
    showSegments(segments: number[]): Promise<void>;
    setTryUseGpu(tryUseGpu: boolean): Promise<void>;
}
