"use strict";
/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.ViewerAutoPreset = void 0;
const behavior_1 = require("../../extensions/model-archive/quality-assessment/behavior.js");
const prop_1 = require("../../extensions/model-archive/quality-assessment/prop.js");
const sb_ncbr_1 = require("../../extensions/sb-ncbr/index.js");
const representation_preset_1 = require("../../mol-plugin-state/builder/structure/representation-preset.js");
const mol_state_1 = require("../../mol-state/index.js");
exports.ViewerAutoPreset = (0, representation_preset_1.StructureRepresentationPresetProvider)({
    id: 'preset-structure-representation-viewer-auto',
    display: {
        name: 'Automatic (w/ Annotation)', group: 'Annotation',
        description: 'Show standard automatic representation but colored by quality assessment (if available in the model).'
    },
    isApplicable(a) {
        return (!!a.data.models.some(m => prop_1.QualityAssessment.isApplicable(m, 'pLDDT')) ||
            !!a.data.models.some(m => prop_1.QualityAssessment.isApplicable(m, 'qmean')));
    },
    params: () => representation_preset_1.StructureRepresentationPresetProvider.CommonParams,
    async apply(ref, params, plugin) {
        var _a;
        const structureCell = mol_state_1.StateObjectRef.resolveAndCheck(plugin.state.data, ref);
        const structure = (_a = structureCell === null || structureCell === void 0 ? void 0 : structureCell.obj) === null || _a === void 0 ? void 0 : _a.data;
        if (!structureCell || !structure)
            return {};
        if (!!structure.models.some(m => prop_1.QualityAssessment.isApplicable(m, 'pLDDT'))) {
            return await behavior_1.QualityAssessmentPLDDTPreset.apply(ref, params, plugin);
        }
        else if (!!structure.models.some(m => prop_1.QualityAssessment.isApplicable(m, 'qmean'))) {
            return await behavior_1.QualityAssessmentQmeanPreset.apply(ref, params, plugin);
        }
        else if (!!structure.models.some(m => sb_ncbr_1.SbNcbrPartialChargesPropertyProvider.isApplicable(m))) {
            return await sb_ncbr_1.SbNcbrPartialChargesPreset.apply(ref, params, plugin);
        }
        else {
            return await representation_preset_1.PresetStructureRepresentations.auto.apply(ref, params, plugin);
        }
    }
});
