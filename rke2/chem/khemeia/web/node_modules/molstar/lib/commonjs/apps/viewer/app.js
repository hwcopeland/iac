"use strict";
/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author Neli Fonseca <neli@ebi.ac.uk>
 * @author Adam Midlik <midlik@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.Viewer = exports.setTimingMode = exports.setProductionMode = exports.setDebugMode = exports.isTimingMode = exports.isProductionMode = exports.isDebugMode = exports.consoleStats = exports.version = void 0;
const assembly_symmetry_1 = require("../../extensions/assembly-symmetry/index.js");
const formats_1 = require("../../extensions/mvs/components/formats.js");
const load_1 = require("../../extensions/mvs/load.js");
const mvs_data_1 = require("../../extensions/mvs/mvs-data.js");
const string_like_1 = require("../../mol-io/common/string-like.js");
const structure_1 = require("../../mol-model/structure.js");
const volume_1 = require("../../mol-model/volume.js");
const file_1 = require("../../mol-plugin-state/actions/file.js");
const structure_2 = require("../../mol-plugin-state/actions/structure.js");
const volume_2 = require("../../mol-plugin-state/actions/volume.js");
const component_1 = require("../../mol-plugin-state/component.js");
const volume_representation_params_1 = require("../../mol-plugin-state/helpers/volume-representation-params.js");
const objects_1 = require("../../mol-plugin-state/objects.js");
const transforms_1 = require("../../mol-plugin-state/transforms.js");
const model_1 = require("../../mol-plugin-state/transforms/model.js");
const mol_plugin_ui_1 = require("../../mol-plugin-ui/index.js");
const react18_1 = require("../../mol-plugin-ui/react18.js");
const spec_1 = require("../../mol-plugin-ui/spec.js");
const behavior_1 = require("../../mol-plugin/behavior.js");
const commands_1 = require("../../mol-plugin/commands.js");
const config_1 = require("../../mol-plugin/config.js");
const mol_task_1 = require("../../mol-task/index.js");
const assets_1 = require("../../mol-util/assets.js");
const color_1 = require("../../mol-util/color/index.js");
const extensions_1 = require("./extensions.js");
const options_1 = require("./options.js");
var version_1 = require("../../mol-plugin/version.js");
Object.defineProperty(exports, "version", { enumerable: true, get: function () { return version_1.PLUGIN_VERSION; } });
var debug_1 = require("../../mol-util/debug.js");
Object.defineProperty(exports, "consoleStats", { enumerable: true, get: function () { return debug_1.consoleStats; } });
Object.defineProperty(exports, "isDebugMode", { enumerable: true, get: function () { return debug_1.isDebugMode; } });
Object.defineProperty(exports, "isProductionMode", { enumerable: true, get: function () { return debug_1.isProductionMode; } });
Object.defineProperty(exports, "isTimingMode", { enumerable: true, get: function () { return debug_1.isTimingMode; } });
Object.defineProperty(exports, "setDebugMode", { enumerable: true, get: function () { return debug_1.setDebugMode; } });
Object.defineProperty(exports, "setProductionMode", { enumerable: true, get: function () { return debug_1.setProductionMode; } });
Object.defineProperty(exports, "setTimingMode", { enumerable: true, get: function () { return debug_1.setTimingMode; } });
const utils_1 = require("../../mol-util/color/utils.js");
require("../../mol-util/polyfill.js");
const presets_1 = require("./presets.js");
const spec_2 = require("../../mol-plugin/spec.js");
const camera_1 = require("../../mol-plugin/behavior/dynamic/camera.js");
class Viewer {
    constructor(plugin) {
        this._events = new component_1.PluginComponent();
        /**
         * Allows subscribing to rxjs observables in the context of the viewer.
         * All subscriptions will be disposed of when the viewer is destroyed.
         */
        this.subscribe = this._events.subscribe.bind(this._events);
        this.plugin = plugin;
    }
    static async create(elementOrId, options = {}) {
        var _a, _b, _c, _d, _e;
        const definedOptions = {};
        // filter for defined properies only so the default values
        // are property applied
        for (const p of Object.keys(options)) {
            if (options[p] !== void 0)
                definedOptions[p] = options[p];
        }
        const o = { ...options_1.DefaultViewerOptions, ...definedOptions };
        const defaultSpec = (0, spec_1.DefaultPluginUISpec)();
        const disabledExtension = new Set((_a = o.disabledExtensions) !== null && _a !== void 0 ? _a : []);
        let baseBehaviors = defaultSpec.behaviors;
        if (o.viewportFocusBehavior === 'disabled') {
            baseBehaviors = baseBehaviors.filter(b => b.transformer !== behavior_1.PluginBehaviors.Camera.FocusLoci
                && b.transformer !== behavior_1.PluginBehaviors.Representation.FocusLoci);
        }
        else if (o.viewportFocusBehavior === 'secondary-zoom') {
            baseBehaviors = baseBehaviors.filter(b => b.transformer !== behavior_1.PluginBehaviors.Camera.FocusLoci
                && b.transformer !== behavior_1.PluginBehaviors.Representation.FocusLoci);
            baseBehaviors.push(spec_2.PluginSpec.Behavior(behavior_1.PluginBehaviors.Camera.FocusLoci, {
                bindings: camera_1.NoPrimaryFocusLociBindings
            }));
        }
        const spec = {
            canvas3d: {
                ...defaultSpec.canvas3d,
            },
            actions: defaultSpec.actions,
            behaviors: [
                ...baseBehaviors,
                ...o.extensions.filter(e => !disabledExtension.has(e)).map(e => extensions_1.ExtensionMap[e]),
            ],
            animations: [...defaultSpec.animations || []],
            customParamEditors: defaultSpec.customParamEditors,
            customFormats: o === null || o === void 0 ? void 0 : o.customFormats,
            layout: {
                initial: {
                    isExpanded: o.layoutIsExpanded,
                    showControls: o.layoutShowControls,
                    controlsDisplay: o.layoutControlsDisplay,
                    regionState: {
                        bottom: 'full',
                        left: o.collapseLeftPanel ? 'collapsed' : 'full',
                        right: o.collapseRightPanel ? 'hidden' : 'full',
                        top: 'full',
                    }
                },
            },
            components: {
                ...defaultSpec.components,
                controls: {
                    ...(_b = defaultSpec.components) === null || _b === void 0 ? void 0 : _b.controls,
                    top: o.layoutShowSequence ? undefined : 'none',
                    bottom: o.layoutShowLog ? undefined : 'none',
                    left: o.layoutShowLeftPanel ? undefined : 'none',
                },
                remoteState: o.layoutShowRemoteState ? 'default' : 'none',
            },
            config: [
                [config_1.PluginConfig.General.DisableAntialiasing, o.disableAntialiasing],
                [config_1.PluginConfig.General.PixelScale, o.pixelScale],
                [config_1.PluginConfig.General.PickScale, o.pickScale],
                [config_1.PluginConfig.General.Transparency, o.transparency],
                [config_1.PluginConfig.General.PreferWebGl1, o.preferWebgl1],
                [config_1.PluginConfig.General.AllowMajorPerformanceCaveat, o.allowMajorPerformanceCaveat],
                [config_1.PluginConfig.General.PowerPreference, o.powerPreference],
                [config_1.PluginConfig.General.ResolutionMode, o.resolutionMode],
                [config_1.PluginConfig.Viewport.ShowReset, o.viewportShowReset],
                [config_1.PluginConfig.Viewport.ShowScreenshotControls, o.viewportShowScreenshotControls],
                [config_1.PluginConfig.Viewport.ShowControls, o.viewportShowControls],
                [config_1.PluginConfig.Viewport.ShowExpand, o.viewportShowExpand],
                [config_1.PluginConfig.Viewport.ShowToggleFullscreen, o.viewportShowToggleFullscreen],
                [config_1.PluginConfig.Viewport.ShowSettings, o.viewportShowSettings],
                [config_1.PluginConfig.Viewport.ShowSelectionMode, o.viewportShowSelectionMode],
                [config_1.PluginConfig.Viewport.ShowAnimation, o.viewportShowAnimation],
                [config_1.PluginConfig.Viewport.ShowTrajectoryControls, o.viewportShowTrajectoryControls],
                [config_1.PluginConfig.State.DefaultServer, o.pluginStateServer],
                [config_1.PluginConfig.State.CurrentServer, o.pluginStateServer],
                [config_1.PluginConfig.VolumeStreaming.DefaultServer, o.volumeStreamingServer],
                [config_1.PluginConfig.VolumeStreaming.Enabled, !o.volumeStreamingDisabled],
                [config_1.PluginConfig.Download.DefaultPdbProvider, o.pdbProvider],
                [config_1.PluginConfig.Download.DefaultEmdbProvider, o.emdbProvider],
                [config_1.PluginConfig.Structure.DefaultRepresentationPreset, presets_1.ViewerAutoPreset.id],
                [config_1.PluginConfig.Structure.SaccharideCompIdMapType, o.saccharideCompIdMapType],
                [assembly_symmetry_1.AssemblySymmetryConfig.DefaultServerType, o.rcsbAssemblySymmetryDefaultServerType],
                [assembly_symmetry_1.AssemblySymmetryConfig.DefaultServerUrl, o.rcsbAssemblySymmetryDefaultServerUrl],
                [assembly_symmetry_1.AssemblySymmetryConfig.ApplyColors, o.rcsbAssemblySymmetryApplyColors],
                ...((_c = o.config) !== null && _c !== void 0 ? _c : []),
            ]
        };
        const element = typeof elementOrId === 'string'
            ? document.getElementById(elementOrId)
            : elementOrId;
        if (!element)
            throw new Error(`Could not get element with id '${elementOrId}'`);
        const plugin = await (0, mol_plugin_ui_1.createPluginUI)({
            target: element,
            spec,
            render: react18_1.renderReact18,
            onBeforeUIRender: plugin => {
                // the preset needs to be added before the UI renders otherwise
                // "Download Structure" wont be able to pick it up
                plugin.builders.structure.representation.registerPreset(presets_1.ViewerAutoPreset);
            }
        });
        (_d = plugin.canvas3d) === null || _d === void 0 ? void 0 : _d.setProps({ illumination: { enabled: o.illumination } });
        if (o.viewportBackgroundColor) {
            const backgroundColor = (0, utils_1.decodeColor)(o.viewportBackgroundColor);
            if (typeof backgroundColor === 'number') {
                (_e = plugin.canvas3d) === null || _e === void 0 ? void 0 : _e.setProps({ renderer: { backgroundColor } });
            }
        }
        return new Viewer(plugin);
    }
    setRemoteSnapshot(id) {
        const url = `${this.plugin.config.get(config_1.PluginConfig.State.CurrentServer)}/get/${id}`;
        return commands_1.PluginCommands.State.Snapshots.Fetch(this.plugin, { url });
    }
    loadSnapshotFromUrl(url, type) {
        return commands_1.PluginCommands.State.Snapshots.OpenUrl(this.plugin, { url, type });
    }
    loadStructureFromUrl(url, format = 'mmcif', isBinary = false, options) {
        const params = structure_2.DownloadStructure.createDefaultParams(this.plugin.state.data.root.obj, this.plugin);
        return this.plugin.runTask(this.plugin.state.data.applyAction(structure_2.DownloadStructure, {
            source: {
                name: 'url',
                params: {
                    url: assets_1.Asset.Url(url),
                    format: format,
                    isBinary,
                    label: options === null || options === void 0 ? void 0 : options.label,
                    options: { ...params.source.params.options, representationParams: options === null || options === void 0 ? void 0 : options.representationParams },
                }
            }
        }));
    }
    async loadAllModelsOrAssemblyFromUrl(url, format = 'mmcif', isBinary = false, options) {
        const plugin = this.plugin;
        const data = await plugin.builders.data.download({ url, isBinary }, { state: { isGhost: true } });
        const trajectory = await plugin.builders.structure.parseTrajectory(data, format);
        await this.plugin.builders.structure.hierarchy.applyPreset(trajectory, 'all-models', { useDefaultIfSingleModel: true, representationPresetParams: options === null || options === void 0 ? void 0 : options.representationParams });
    }
    async loadStructureFromData(data, format, options) {
        const _data = await this.plugin.builders.data.rawData({ data, label: options === null || options === void 0 ? void 0 : options.dataLabel });
        const trajectory = await this.plugin.builders.structure.parseTrajectory(_data, format);
        await this.plugin.builders.structure.hierarchy.applyPreset(trajectory, 'default');
    }
    loadPdb(pdb, options) {
        const params = structure_2.DownloadStructure.createDefaultParams(this.plugin.state.data.root.obj, this.plugin);
        const provider = this.plugin.config.get(config_1.PluginConfig.Download.DefaultPdbProvider);
        return this.plugin.runTask(this.plugin.state.data.applyAction(structure_2.DownloadStructure, {
            source: {
                name: 'pdb',
                params: {
                    provider: {
                        id: pdb,
                        server: {
                            name: provider,
                            params: structure_2.PdbDownloadProvider[provider].defaultValue
                        }
                    },
                    options: { ...params.source.params.options, representationParams: options === null || options === void 0 ? void 0 : options.representationParams },
                }
            }
        }));
    }
    /**
     * @deprecated Scheduled for removal in v5. Use {@link loadPdbIhm | loadPdbIhm(pdbIhm: string)} instead.
     */
    loadPdbDev(pdbDev) {
        return this.loadPdbIhm(pdbDev);
    }
    loadPdbIhm(pdbIhm) {
        const params = structure_2.DownloadStructure.createDefaultParams(this.plugin.state.data.root.obj, this.plugin);
        return this.plugin.runTask(this.plugin.state.data.applyAction(structure_2.DownloadStructure, {
            source: {
                name: 'pdb-ihm',
                params: {
                    provider: {
                        id: pdbIhm,
                        encoding: 'bcif',
                    },
                    options: params.source.params.options,
                }
            }
        }));
    }
    loadEmdb(emdb, options) {
        var _a;
        const provider = this.plugin.config.get(config_1.PluginConfig.Download.DefaultEmdbProvider);
        return this.plugin.runTask(this.plugin.state.data.applyAction(volume_2.DownloadDensity, {
            source: {
                name: 'pdb-emd-ds',
                params: {
                    provider: {
                        id: emdb,
                        server: provider,
                    },
                    detail: (_a = options === null || options === void 0 ? void 0 : options.detail) !== null && _a !== void 0 ? _a : 3,
                }
            }
        }));
    }
    loadAlphaFoldDb(afdb) {
        const params = structure_2.DownloadStructure.createDefaultParams(this.plugin.state.data.root.obj, this.plugin);
        return this.plugin.runTask(this.plugin.state.data.applyAction(structure_2.DownloadStructure, {
            source: {
                name: 'alphafolddb',
                params: {
                    provider: {
                        id: afdb,
                        encoding: 'bcif'
                    },
                    options: {
                        ...params.source.params.options,
                        representation: 'preset-structure-representation-ma-quality-assessment-plddt'
                    },
                }
            }
        }));
    }
    loadModelArchive(id) {
        const params = structure_2.DownloadStructure.createDefaultParams(this.plugin.state.data.root.obj, this.plugin);
        return this.plugin.runTask(this.plugin.state.data.applyAction(structure_2.DownloadStructure, {
            source: {
                name: 'modelarchive',
                params: {
                    id,
                    options: params.source.params.options,
                }
            }
        }));
    }
    /**
     * @example Load X-ray density from volume server
        viewer.loadVolumeFromUrl({
            url: 'https://www.ebi.ac.uk/pdbe/densities/x-ray/1tqn/cell?detail=3',
            format: 'dscif',
            isBinary: true
        }, [{
            type: 'relative',
            value: 1.5,
            color: 0x3362B2
        }, {
            type: 'relative',
            value: 3,
            color: 0x33BB33,
            volumeIndex: 1
        }, {
            type: 'relative',
            value: -3,
            color: 0xBB3333,
            volumeIndex: 1
        }], {
            entryId: ['2FO-FC', 'FO-FC'],
            isLazy: true
        });
     * *********************
     * @example Load EM density from volume server
        viewer.loadVolumeFromUrl({
            url: 'https://maps.rcsb.org/em/emd-30210/cell?detail=6',
            format: 'dscif',
            isBinary: true
        }, [{
            type: 'relative',
            value: 1,
            color: 0x3377aa
        }], {
            entryId: 'EMD-30210',
            isLazy: true
        });
     */
    async loadVolumeFromUrl({ url, format, isBinary }, isovalues, options) {
        const plugin = this.plugin;
        if (!plugin.dataFormats.get(format)) {
            throw new Error(`Unknown density format: ${format}`);
        }
        if (options === null || options === void 0 ? void 0 : options.isLazy) {
            const update = this.plugin.build();
            update.toRoot().apply(transforms_1.StateTransforms.Data.LazyVolume, {
                url,
                format,
                entryId: options === null || options === void 0 ? void 0 : options.entryId,
                isBinary,
                isovalues: isovalues.map(v => ({ alpha: 1, volumeIndex: 0, ...v }))
            });
            return update.commit();
        }
        return plugin.dataTransaction(async () => {
            var _a, _b, _c, _d;
            const data = await plugin.builders.data.download({ url, isBinary }, { state: { isGhost: true } });
            const parsed = await plugin.dataFormats.get(format).parse(plugin, data, { entryId: options === null || options === void 0 ? void 0 : options.entryId });
            const firstVolume = (parsed.volume || parsed.volumes[0]);
            if (!(firstVolume === null || firstVolume === void 0 ? void 0 : firstVolume.isOk))
                throw new Error('Failed to parse any volume.');
            const repr = plugin.build();
            for (const iso of isovalues) {
                const volume = (_c = (_a = parsed.volumes) === null || _a === void 0 ? void 0 : _a[(_b = iso.volumeIndex) !== null && _b !== void 0 ? _b : 0]) !== null && _c !== void 0 ? _c : parsed.volume;
                const volumeData = volume.cell.obj.data;
                repr
                    .to(volume)
                    .apply(transforms_1.StateTransforms.Representation.VolumeRepresentation3D, (0, volume_representation_params_1.createVolumeRepresentationParams)(this.plugin, firstVolume.data, {
                    type: 'isosurface',
                    typeParams: { alpha: (_d = iso.alpha) !== null && _d !== void 0 ? _d : 1, isoValue: volume_1.Volume.adjustedIsoValue(volumeData, iso.value, iso.type) },
                    color: 'uniform',
                    colorParams: { value: iso.color }
                }));
            }
            await repr.commit();
        });
    }
    loadFullResolutionEMDBMap(emdbId, options) {
        const plugin = this.plugin;
        const numericId = parseInt(emdbId.toUpperCase().replace('EMD-', ''));
        const url = `https://ftp.ebi.ac.uk/pub/databases/emdb/structures/EMD-${numericId}/map/emd_${numericId}.map.gz`;
        return plugin.dataTransaction(async () => {
            var _a, _b, _c;
            const data = await plugin.build().toRoot()
                .apply(transforms_1.StateTransforms.Data.Download, { url, isBinary: true, label: emdbId }, { state: { isGhost: true } })
                .apply(transforms_1.StateTransforms.Data.DeflateData)
                .commit();
            const parsed = await plugin.dataFormats.get('ccp4').parse(plugin, data, { entryId: emdbId });
            const firstVolume = (parsed.volume || parsed.volumes[0]);
            if (!(firstVolume === null || firstVolume === void 0 ? void 0 : firstVolume.isOk))
                throw new Error('Failed to parse any volume.');
            const volume = (_b = (_a = parsed.volumes) === null || _a === void 0 ? void 0 : _a[0]) !== null && _b !== void 0 ? _b : parsed.volume;
            await plugin.build()
                .to(volume)
                .apply(transforms_1.StateTransforms.Representation.VolumeRepresentation3D, (0, volume_representation_params_1.createVolumeRepresentationParams)(this.plugin, firstVolume.data, {
                type: 'isosurface',
                typeParams: { alpha: 1, isoValue: options.isoValue },
                color: 'uniform',
                colorParams: { value: (_c = options.color) !== null && _c !== void 0 ? _c : (0, color_1.Color)(0x33BB33) }
            }))
                .commit();
        });
    }
    /**
     * @example
     *  viewer.loadTrajectory({
     *      model: { kind: 'model-url', url: 'villin.gro', format: 'gro' },
     *      coordinates: { kind: 'coordinates-url', url: 'villin.xtc', format: 'xtc', isBinary: true },
     *      preset: 'all-models' // or 'default'
     *  });
     */
    async loadTrajectory(params) {
        var _a, _b;
        const plugin = this.plugin;
        let model;
        if (params.model.kind === 'model-data' || params.model.kind === 'model-url') {
            const data = params.model.kind === 'model-data'
                ? await plugin.builders.data.rawData({ data: params.model.data, label: params.modelLabel })
                : await plugin.builders.data.download({ url: params.model.url, isBinary: params.model.isBinary, label: params.modelLabel });
            const trajectory = await plugin.builders.structure.parseTrajectory(data, (_a = params.model.format) !== null && _a !== void 0 ? _a : 'mmcif');
            model = await plugin.builders.structure.createModel(trajectory);
        }
        else {
            const data = params.model.kind === 'topology-data'
                ? await plugin.builders.data.rawData({ data: params.model.data, label: params.modelLabel })
                : await plugin.builders.data.download({ url: params.model.url, isBinary: params.model.isBinary, label: params.modelLabel });
            const provider = plugin.dataFormats.get(params.model.format);
            const parsed = await provider.parse(plugin, data);
            model = parsed.topology;
        }
        const data = params.coordinates.kind === 'coordinates-data'
            ? await plugin.builders.data.rawData({ data: params.coordinates.data, label: params.coordinatesLabel })
            : await plugin.builders.data.download({ url: params.coordinates.url, isBinary: params.coordinates.isBinary, label: params.coordinatesLabel });
        const provider = plugin.dataFormats.get(params.coordinates.format);
        const coords = await provider.parse(plugin, data);
        const trajectory = await plugin.build().toRoot()
            .apply(model_1.TrajectoryFromModelAndCoordinates, {
            modelRef: model.ref,
            coordinatesRef: coords.ref
        }, { dependsOn: [model.ref, coords.ref] })
            .commit();
        const preset = await plugin.builders.structure.hierarchy.applyPreset(trajectory, (_b = params.preset) !== null && _b !== void 0 ? _b : 'default');
        return { model, coords, preset };
    }
    async loadMvsFromUrl(url, format, options) {
        if (format === 'mvsj') {
            const data = await this.plugin.runTask(this.plugin.fetch({ url, type: 'string' }));
            const mvsData = mvs_data_1.MVSData.fromMVSJ(string_like_1.StringLike.toString(data));
            await (0, load_1.loadMVS)(this.plugin, mvsData, { sanityChecks: true, sourceUrl: url, ...options });
        }
        else if (format === 'mvsx') {
            const data = await this.plugin.runTask(this.plugin.fetch({ url, type: 'binary' }));
            await this.plugin.runTask(mol_task_1.Task.create('Load MVSX file', async (ctx) => {
                const parsed = await (0, formats_1.loadMVSX)(this.plugin, ctx, data, { doNotClearAssets: options === null || options === void 0 ? void 0 : options.appendSnapshots });
                await (0, load_1.loadMVS)(this.plugin, parsed.mvsData, { sanityChecks: true, sourceUrl: parsed.sourceUrl, ...options });
            }));
        }
        else {
            throw new Error(`Unknown MolViewSpec format: ${format}`);
        }
    }
    /** Load MolViewSpec from `data`.
     * If `format` is 'mvsj', `data` must be a string or a Uint8Array containing a UTF8-encoded string.
     * If `format` is 'mvsx', `data` must be a Uint8Array or a string containing base64-encoded binary data prefixed with 'base64,'. */
    loadMvsData(data, format, options) {
        return (0, formats_1.loadMVSData)(this.plugin, data, format, options);
    }
    loadFiles(files) {
        const sessions = files.filter(f => {
            const fn = f.name.toLowerCase();
            return fn.endsWith('.molx') || fn.endsWith('.molj');
        });
        if (sessions.length > 0) {
            return commands_1.PluginCommands.State.Snapshots.OpenFile(this.plugin, { file: sessions[0] });
        }
        else {
            return this.plugin.runTask(this.plugin.state.data.applyAction(file_1.OpenFiles, {
                files: files.map(f => assets_1.Asset.File(f)),
                format: { name: 'auto', params: {} },
                visuals: true
            }));
        }
    }
    handleResize() {
        this.plugin.layout.events.updated.next(void 0);
    }
    /**
     * Triggers structure element selection or highlighting based on the provided
     * MolScript expression or StructureElement schema. Focus action will only apply to the
     * first structure that matches the criteria.
     *
     * If neither `expression` nor `elements` are provided, all selections/highlights
     * will be cleared based on the specified `action`.
     */
    structureInteractivity({ expression, elements, action, applyGranularity = false, filterStructure, focusOptions }) {
        var _a;
        const plugin = this.plugin;
        if (!expression && !elements) {
            if (action === 'select') {
                plugin.managers.interactivity.lociSelects.deselectAll();
            }
            else if (action === 'highlight') {
                plugin.managers.interactivity.lociHighlights.clearHighlights();
            }
            return;
        }
        const structures = this.plugin.state.data.selectQ(Q => Q.rootsOfType(objects_1.PluginStateObject.Molecule.Structure));
        for (const s of structures) {
            if (!((_a = s.obj) === null || _a === void 0 ? void 0 : _a.data))
                continue;
            if (filterStructure && !filterStructure(s.obj.data))
                continue;
            const loci = expression
                ? structure_1.StructureElement.Loci.fromExpression(s.obj.data, expression)
                : structure_1.StructureElement.Loci.fromSchema(s.obj.data, elements);
            if (action === 'select') {
                plugin.managers.interactivity.lociSelects.select({ loci }, applyGranularity);
            }
            else if (action === 'highlight') {
                plugin.managers.interactivity.lociHighlights.highlight({ loci }, applyGranularity);
            }
            else if (action === 'focus' && !structure_1.StructureElement.Loci.isEmpty(loci)) {
                plugin.managers.camera.focusLoci(loci, focusOptions);
                return;
            }
        }
    }
    dispose() {
        this._events.dispose();
        this.plugin.dispose();
    }
}
exports.Viewer = Viewer;
