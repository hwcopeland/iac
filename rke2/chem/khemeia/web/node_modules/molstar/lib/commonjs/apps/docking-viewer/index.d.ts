/**
 * Copyright (c) 2018-2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { BuiltInTrajectoryFormat } from '../../mol-plugin-state/formats/trajectory.js';
import { PluginUIContext } from '../../mol-plugin-ui/context.js';
import { Color } from '../../mol-util/color/index.js';
import '../../mol-util/polyfill.js';
import './index.html';
import '../../mol-plugin-ui/skin/light.scss';
export { PLUGIN_VERSION as version } from '../../mol-plugin/version.js';
export { setDebugMode, setProductionMode } from '../../mol-util/debug.js';
export { Viewer as DockingViewer };
declare class Viewer {
    plugin: PluginUIContext;
    constructor(plugin: PluginUIContext);
    static create(elementOrId: string | HTMLElement, colors?: Color[], showButtons?: boolean): Promise<Viewer>;
    loadStructuresFromUrlsAndMerge(sources: {
        url: string;
        format: BuiltInTrajectoryFormat;
        isBinary?: boolean;
    }[]): Promise<void>;
}
