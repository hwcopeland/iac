/**
 * Copyright (c) 2018 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Interval } from '../int/interval.js';
import { OrderedSet } from '../int/ordered-set.js';
import { Segmentation } from '../int/segmentation.js';
import { Iterator } from '../iterator.js';
/** Emits a segment of length one for each element in the interval that is also in the set */
export declare class IntervalIterator<I extends number = number> implements Iterator<Segmentation.Segment<I>> {
    private interval;
    private set;
    private value;
    private curIndex;
    private maxIndex;
    hasNext: boolean;
    updateValue(): void;
    move(): Segmentation.Segment<I>;
    constructor(interval: Interval<I>, set: OrderedSet<I>);
}
