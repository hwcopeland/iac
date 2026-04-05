/**
 * Copyright (c) 2022 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { TopFile } from '../../mol-io/reader/top/parser.js';
import { Topology } from '../../mol-model/structure/topology/topology.js';
import { Task } from '../../mol-task/index.js';
import { ModelFormat } from '../format.js';
export { TopFormat };
type TopFormat = ModelFormat<TopFile>;
declare namespace TopFormat {
    function is(x?: ModelFormat): x is TopFormat;
    function fromTop(top: TopFile): TopFormat;
}
export declare function topologyFromTop(top: TopFile): Task<Topology>;
