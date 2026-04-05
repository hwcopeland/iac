/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { QualityAssessmentPLDDTPreset, QualityAssessmentQmeanPreset } from '../../extensions/model-archive/quality-assessment/behavior.js';
import { QualityAssessment } from '../../extensions/model-archive/quality-assessment/prop.js';
import { SbNcbrPartialChargesPreset, SbNcbrPartialChargesPropertyProvider } from '../../extensions/sb-ncbr/index.js';
import { PresetStructureRepresentations, StructureRepresentationPresetProvider } from '../../mol-plugin-state/builder/structure/representation-preset.js';
import { StateObjectRef } from '../../mol-state/index.js';
export const ViewerAutoPreset = StructureRepresentationPresetProvider({
    id: 'preset-structure-representation-viewer-auto',
    display: {
        name: 'Automatic (w/ Annotation)', group: 'Annotation',
        description: 'Show standard automatic representation but colored by quality assessment (if available in the model).'
    },
    isApplicable(a) {
        return (!!a.data.models.some(m => QualityAssessment.isApplicable(m, 'pLDDT')) ||
            !!a.data.models.some(m => QualityAssessment.isApplicable(m, 'qmean')));
    },
    params: () => StructureRepresentationPresetProvider.CommonParams,
    async apply(ref, params, plugin) {
        var _a;
        const structureCell = StateObjectRef.resolveAndCheck(plugin.state.data, ref);
        const structure = (_a = structureCell === null || structureCell === void 0 ? void 0 : structureCell.obj) === null || _a === void 0 ? void 0 : _a.data;
        if (!structureCell || !structure)
            return {};
        if (!!structure.models.some(m => QualityAssessment.isApplicable(m, 'pLDDT'))) {
            return await QualityAssessmentPLDDTPreset.apply(ref, params, plugin);
        }
        else if (!!structure.models.some(m => QualityAssessment.isApplicable(m, 'qmean'))) {
            return await QualityAssessmentQmeanPreset.apply(ref, params, plugin);
        }
        else if (!!structure.models.some(m => SbNcbrPartialChargesPropertyProvider.isApplicable(m))) {
            return await SbNcbrPartialChargesPreset.apply(ref, params, plugin);
        }
        else {
            return await PresetStructureRepresentations.auto.apply(ref, params, plugin);
        }
    }
});
