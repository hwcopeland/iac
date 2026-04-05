"use strict";
/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author Lukáš Polák <admin@lukaspolak.cz>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.ResidueChargeColorThemeProvider = exports.ResidueChargeColorThemeParams = exports.ChargedResidueColors = void 0;
exports.getResidueChargeColorThemeParams = getResidueChargeColorThemeParams;
exports.residueChargeColor = residueChargeColor;
exports.ResidueChargeColorTheme = ResidueChargeColorTheme;
const color_1 = require("../../mol-util/color/index.js");
const structure_1 = require("../../mol-model/structure.js");
const param_definition_1 = require("../../mol-util/param-definition.js");
const legend_1 = require("../../mol-util/legend.js");
const color_2 = require("../../mol-util/color/color.js");
const params_1 = require("../../mol-util/color/params.js");
const categories_1 = require("./categories.js");
// Colors for charged residues (by-name)
exports.ChargedResidueColors = (0, color_1.ColorMap)({
    // standard amino acids (charged)
    'ARG': 0x0000FF,
    'ASP': 0xFF0000,
    'GLU': 0xFF0000,
    'HIS': 0x33C3F9,
    'LYS': 0x0000FF,
    // standard amino acids (uncharged)
    'ALA': 0xFFFFFF,
    'ASN': 0xFFFFFF,
    'CYS': 0xFFFFFF,
    'GLN': 0xFFFFFF,
    'GLY': 0xFFFFFF,
    'ILE': 0xFFFFFF,
    'LEU': 0xFFFFFF,
    'MET': 0xFFFFFF,
    'PHE': 0xFFFFFF,
    'PRO': 0xFFFFFF,
    'SER': 0xFFFFFF,
    'THR': 0xFFFFFF,
    'TRP': 0xFFFFFF,
    'TYR': 0xFFFFFF,
    'VAL': 0xFFFFFF,
    // common from CCD
    'MSE': 0xFFFFFF,
    'SEP': 0xFFFFFF,
    'TPO': 0xFFFFFF,
    'PTR': 0xFFFFFF,
    'PCA': 0xFFFFFF,
    'HYP': 0xFFFFFF,
    // charmm ff
    'HSD': 0xFFFFFF,
    'HSE': 0xFFFFFF,
    'HSP': 0x0000FF,
    'LSN': 0xFFFFFF,
    'ASPP': 0xFFFFFF,
    'GLUP': 0xFFFFFF,
    // amber ff
    'HID': 0xFFFFFF,
    'HIE': 0xFFFFFF,
    'HIP': 0x0000FF,
    'LYN': 0xFFFFFF,
    'ASH': 0xFFFFFF,
    'GLH': 0xFFFFFF,
    // rna bases
    'A': 0xFFFFFF,
    'G': 0xFFFFFF,
    'I': 0xFFFFFF,
    'C': 0xFFFFFF,
    'T': 0xFFFFFF,
    'U': 0xFFFFFF,
    // dna bases
    'DA': 0xFFFFFF,
    'DG': 0xFFFFFF,
    'DI': 0xFFFFFF,
    'DC': 0xFFFFFF,
    'DT': 0xFFFFFF,
    'DU': 0xFFFFFF,
    // peptide bases
    'APN': 0xFFFFFF,
    'GPN': 0xFFFFFF,
    'CPN': 0xFFFFFF,
    'TPN': 0xFFFFFF,
});
const DefaultResidueChargeColor = (0, color_1.Color)(0xFF00FF);
const Description = 'Assigns a color to every residue based on its charge state.';
exports.ResidueChargeColorThemeParams = {
    method: param_definition_1.ParamDefinition.MappedStatic('by-name', {
        'by-name': param_definition_1.ParamDefinition.Group({
            saturation: param_definition_1.ParamDefinition.Numeric(0, { min: -6, max: 6, step: 0.1 }),
            lightness: param_definition_1.ParamDefinition.Numeric(0, { min: -6, max: 6, step: 0.1 }),
            colors: param_definition_1.ParamDefinition.MappedStatic('default', {
                'default': param_definition_1.ParamDefinition.EmptyGroup(),
                'custom': param_definition_1.ParamDefinition.Group((0, params_1.getColorMapParams)(exports.ChargedResidueColors)),
            })
        }, { isFlat: true })
    })
};
function getResidueChargeColorThemeParams(ctx) {
    return param_definition_1.ParamDefinition.clone(exports.ResidueChargeColorThemeParams);
}
function getAtomicCompId(unit, element) {
    return unit.model.atomicHierarchy.atoms.label_comp_id.value(element);
}
function getCoarseCompId(unit, element) {
    const seqIdBegin = unit.coarseElements.seq_id_begin.value(element);
    const seqIdEnd = unit.coarseElements.seq_id_end.value(element);
    if (seqIdBegin === seqIdEnd) {
        const entityKey = unit.coarseElements.entityKey[element];
        const seq = unit.model.sequence.byEntityKey[entityKey].sequence;
        return seq.compId.value(seqIdBegin - 1); // 1-indexed
    }
}
function residueChargeColor(colorMap, residueName) {
    const c = colorMap[residueName];
    return c === undefined ? DefaultResidueChargeColor : c;
}
function ResidueChargeColorTheme(ctx, props) {
    const { saturation, lightness, colors } = props.method.params;
    const colorMap = (0, color_2.getAdjustedColorMap)(props.method.params.colors.name === 'default' ? exports.ChargedResidueColors : colors.params, saturation, lightness);
    function color(location) {
        if (structure_1.StructureElement.Location.is(location)) {
            if (structure_1.Unit.isAtomic(location.unit)) {
                const compId = getAtomicCompId(location.unit, location.element);
                return residueChargeColor(colorMap, compId);
            }
            else {
                const compId = getCoarseCompId(location.unit, location.element);
                if (compId)
                    return residueChargeColor(colorMap, compId);
            }
        }
        else if (structure_1.Bond.isLocation(location)) {
            if (structure_1.Unit.isAtomic(location.aUnit)) {
                const compId = getAtomicCompId(location.aUnit, location.aUnit.elements[location.aIndex]);
                return residueChargeColor(colorMap, compId);
            }
            else {
                const compId = getCoarseCompId(location.aUnit, location.aUnit.elements[location.aIndex]);
                if (compId)
                    return residueChargeColor(colorMap, compId);
            }
        }
        return DefaultResidueChargeColor;
    }
    return {
        factory: ResidueChargeColorTheme,
        granularity: 'group',
        preferSmoothing: true,
        color,
        props,
        description: Description,
        legend: (0, legend_1.TableLegend)(Object.keys(colorMap).map(name => {
            return [name, colorMap[name]];
        }).concat([['Unknown', DefaultResidueChargeColor]]))
    };
}
exports.ResidueChargeColorThemeProvider = {
    name: 'residue-charge',
    label: 'Residue Charge',
    category: categories_1.ColorThemeCategory.Residue,
    factory: ResidueChargeColorTheme,
    getParams: getResidueChargeColorThemeParams,
    defaultValues: param_definition_1.ParamDefinition.getDefaultValues(exports.ResidueChargeColorThemeParams),
    isApplicable: (ctx) => !!ctx.structure
};
