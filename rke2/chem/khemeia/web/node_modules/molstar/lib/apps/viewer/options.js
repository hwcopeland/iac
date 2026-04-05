/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { AssemblySymmetryConfig } from '../../extensions/assembly-symmetry/index.js';
import { G3dProvider } from '../../extensions/g3d/format.js';
import { PluginConfig } from '../../mol-plugin/config.js';
import '../../mol-util/polyfill.js';
import { ObjectKeys } from '../../mol-util/type-helpers.js';
import { ExtensionMap } from './extensions.js';
const CustomFormats = [
    ['g3d', G3dProvider]
];
export const DefaultViewerOptions = {
    customFormats: CustomFormats,
    extensions: ObjectKeys(ExtensionMap),
    disabledExtensions: [],
    layoutIsExpanded: true,
    layoutShowControls: true,
    layoutShowRemoteState: true,
    layoutControlsDisplay: 'reactive',
    layoutShowSequence: true,
    layoutShowLog: true,
    layoutShowLeftPanel: true,
    collapseLeftPanel: false,
    collapseRightPanel: false,
    disableAntialiasing: PluginConfig.General.DisableAntialiasing.defaultValue,
    pixelScale: PluginConfig.General.PixelScale.defaultValue,
    pickScale: PluginConfig.General.PickScale.defaultValue,
    transparency: PluginConfig.General.Transparency.defaultValue,
    preferWebgl1: PluginConfig.General.PreferWebGl1.defaultValue,
    allowMajorPerformanceCaveat: PluginConfig.General.AllowMajorPerformanceCaveat.defaultValue,
    powerPreference: PluginConfig.General.PowerPreference.defaultValue,
    resolutionMode: PluginConfig.General.ResolutionMode.defaultValue,
    illumination: false,
    viewportShowReset: PluginConfig.Viewport.ShowReset.defaultValue,
    viewportShowScreenshotControls: PluginConfig.Viewport.ShowScreenshotControls.defaultValue,
    viewportShowControls: PluginConfig.Viewport.ShowControls.defaultValue,
    viewportShowExpand: PluginConfig.Viewport.ShowExpand.defaultValue,
    viewportShowToggleFullscreen: PluginConfig.Viewport.ShowToggleFullscreen.defaultValue,
    viewportShowSettings: PluginConfig.Viewport.ShowSettings.defaultValue,
    viewportShowSelectionMode: PluginConfig.Viewport.ShowSelectionMode.defaultValue,
    viewportShowAnimation: PluginConfig.Viewport.ShowAnimation.defaultValue,
    viewportShowTrajectoryControls: PluginConfig.Viewport.ShowTrajectoryControls.defaultValue,
    // default: zoom & show structure interaction
    // secondary-zoom: zoom only, doesn't use primary mouse button
    // disabled: no automatic zoom or interaction on focus
    viewportFocusBehavior: 'default',
    viewportBackgroundColor: undefined,
    pluginStateServer: PluginConfig.State.DefaultServer.defaultValue,
    volumeStreamingServer: PluginConfig.VolumeStreaming.DefaultServer.defaultValue,
    volumeStreamingDisabled: !PluginConfig.VolumeStreaming.Enabled.defaultValue,
    pdbProvider: PluginConfig.Download.DefaultPdbProvider.defaultValue,
    emdbProvider: PluginConfig.Download.DefaultEmdbProvider.defaultValue,
    saccharideCompIdMapType: 'default',
    rcsbAssemblySymmetryDefaultServerType: AssemblySymmetryConfig.DefaultServerType.defaultValue,
    rcsbAssemblySymmetryDefaultServerUrl: AssemblySymmetryConfig.DefaultServerUrl.defaultValue,
    rcsbAssemblySymmetryApplyColors: AssemblySymmetryConfig.ApplyColors.defaultValue,
    config: [],
};
