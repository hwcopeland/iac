"use strict";
/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.DefaultViewerOptions = void 0;
const assembly_symmetry_1 = require("../../extensions/assembly-symmetry/index.js");
const format_1 = require("../../extensions/g3d/format.js");
const config_1 = require("../../mol-plugin/config.js");
require("../../mol-util/polyfill.js");
const type_helpers_1 = require("../../mol-util/type-helpers.js");
const extensions_1 = require("./extensions.js");
const CustomFormats = [
    ['g3d', format_1.G3dProvider]
];
exports.DefaultViewerOptions = {
    customFormats: CustomFormats,
    extensions: (0, type_helpers_1.ObjectKeys)(extensions_1.ExtensionMap),
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
    disableAntialiasing: config_1.PluginConfig.General.DisableAntialiasing.defaultValue,
    pixelScale: config_1.PluginConfig.General.PixelScale.defaultValue,
    pickScale: config_1.PluginConfig.General.PickScale.defaultValue,
    transparency: config_1.PluginConfig.General.Transparency.defaultValue,
    preferWebgl1: config_1.PluginConfig.General.PreferWebGl1.defaultValue,
    allowMajorPerformanceCaveat: config_1.PluginConfig.General.AllowMajorPerformanceCaveat.defaultValue,
    powerPreference: config_1.PluginConfig.General.PowerPreference.defaultValue,
    resolutionMode: config_1.PluginConfig.General.ResolutionMode.defaultValue,
    illumination: false,
    viewportShowReset: config_1.PluginConfig.Viewport.ShowReset.defaultValue,
    viewportShowScreenshotControls: config_1.PluginConfig.Viewport.ShowScreenshotControls.defaultValue,
    viewportShowControls: config_1.PluginConfig.Viewport.ShowControls.defaultValue,
    viewportShowExpand: config_1.PluginConfig.Viewport.ShowExpand.defaultValue,
    viewportShowToggleFullscreen: config_1.PluginConfig.Viewport.ShowToggleFullscreen.defaultValue,
    viewportShowSettings: config_1.PluginConfig.Viewport.ShowSettings.defaultValue,
    viewportShowSelectionMode: config_1.PluginConfig.Viewport.ShowSelectionMode.defaultValue,
    viewportShowAnimation: config_1.PluginConfig.Viewport.ShowAnimation.defaultValue,
    viewportShowTrajectoryControls: config_1.PluginConfig.Viewport.ShowTrajectoryControls.defaultValue,
    // default: zoom & show structure interaction
    // secondary-zoom: zoom only, doesn't use primary mouse button
    // disabled: no automatic zoom or interaction on focus
    viewportFocusBehavior: 'default',
    viewportBackgroundColor: undefined,
    pluginStateServer: config_1.PluginConfig.State.DefaultServer.defaultValue,
    volumeStreamingServer: config_1.PluginConfig.VolumeStreaming.DefaultServer.defaultValue,
    volumeStreamingDisabled: !config_1.PluginConfig.VolumeStreaming.Enabled.defaultValue,
    pdbProvider: config_1.PluginConfig.Download.DefaultPdbProvider.defaultValue,
    emdbProvider: config_1.PluginConfig.Download.DefaultEmdbProvider.defaultValue,
    saccharideCompIdMapType: 'default',
    rcsbAssemblySymmetryDefaultServerType: assembly_symmetry_1.AssemblySymmetryConfig.DefaultServerType.defaultValue,
    rcsbAssemblySymmetryDefaultServerUrl: assembly_symmetry_1.AssemblySymmetryConfig.DefaultServerUrl.defaultValue,
    rcsbAssemblySymmetryApplyColors: assembly_symmetry_1.AssemblySymmetryConfig.ApplyColors.defaultValue,
    config: [],
};
