/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Color } from '../mol-util/color/index.js';
import { CarbohydrateSymbolColorThemeProvider } from './color/carbohydrate-symbol.js';
import { UniformColorThemeProvider } from './color/uniform.js';
import { deepEqual } from '../mol-util/index.js';
import { ThemeRegistry } from './theme.js';
import { ChainIdColorThemeProvider } from './color/chain-id.js';
import { ElementIndexColorThemeProvider } from './color/element-index.js';
import { ElementSymbolColorThemeProvider } from './color/element-symbol.js';
import { MoleculeTypeColorThemeProvider } from './color/molecule-type.js';
import { PolymerIdColorThemeProvider } from './color/polymer-id.js';
import { PolymerIndexColorThemeProvider } from './color/polymer-index.js';
import { ResidueNameColorThemeProvider } from './color/residue-name.js';
import { ResidueChargeColorThemeProvider } from './color/residue-charge.js';
import { SecondaryStructureColorThemeProvider } from './color/secondary-structure.js';
import { SequenceIdColorThemeProvider } from './color/sequence-id.js';
import { ShapeGroupColorThemeProvider } from './color/shape-group.js';
import { UnitIndexColorThemeProvider } from './color/unit-index.js';
import { UncertaintyColorThemeProvider } from './color/uncertainty.js';
import { EntitySourceColorThemeProvider } from './color/entity-source.js';
import { IllustrativeColorThemeProvider } from './color/illustrative.js';
import { HydrophobicityColorThemeProvider } from './color/hydrophobicity.js';
import { TrajectoryIndexColorThemeProvider } from './color/trajectory-index.js';
import { OccupancyColorThemeProvider } from './color/occupancy.js';
import { OperatorNameColorThemeProvider } from './color/operator-name.js';
import { OperatorHklColorThemeProvider } from './color/operator-hkl.js';
import { PartialChargeColorThemeProvider } from './color/partial-charge.js';
import { AtomIdColorThemeProvider } from './color/atom-id.js';
import { EntityIdColorThemeProvider } from './color/entity-id.js';
import { VolumeValueColorThemeProvider } from './color/volume-value.js';
import { ModelIndexColorThemeProvider } from './color/model-index.js';
import { StructureIndexColorThemeProvider } from './color/structure-index.js';
import { VolumeSegmentColorThemeProvider } from './color/volume-segment.js';
import { ExternalVolumeColorThemeProvider } from './color/external-volume.js';
import { ColorThemeCategory } from './color/categories.js';
import { CartoonColorThemeProvider } from './color/cartoon.js';
import { FormalChargeColorThemeProvider } from './color/formal-charge.js';
import { ExternalStructureColorThemeProvider } from './color/external-structure.js';
import { getPrecision } from '../mol-util/number.js';
import { SortedArray } from '../mol-data/int/sorted-array.js';
import { normalize } from '../mol-math/interpolate.js';
import { VolumeInstanceColorThemeProvider } from './color/volume-instance.js';
export { ColorTheme };
var ColorTheme;
(function (ColorTheme) {
    ColorTheme.Category = ColorThemeCategory;
    function Palette(list, kind, domain, defaultColor) {
        const colors = [];
        const hasOffsets = list.every(c => Array.isArray(c));
        if (hasOffsets) {
            let maxPrecision = 0;
            for (const e of list) {
                if (Array.isArray(e)) {
                    const p = getPrecision(e[1]);
                    if (p > maxPrecision)
                        maxPrecision = p;
                }
            }
            const count = Math.pow(10, maxPrecision);
            const sorted = [...list];
            sorted.sort((a, b) => a[1] - b[1]);
            const src = sorted.map(c => c[0]);
            const values = SortedArray.ofSortedArray(sorted.map(c => c[1]));
            const _off = [];
            for (let i = 0, il = values.length - 1; i < il; ++i) {
                _off.push(values[i] + (values[i + 1] - values[i]) / 2);
            }
            _off.push(values[values.length - 1]);
            const off = SortedArray.ofSortedArray(_off);
            for (let i = 0, il = Math.max(count, list.length); i < il; ++i) {
                const t = normalize(i, 0, count - 1);
                const j = SortedArray.findPredecessorIndex(off, t);
                colors[i] = src[j];
            }
        }
        else {
            for (const e of list) {
                if (Array.isArray(e))
                    colors.push(e[0]);
                else
                    colors.push(e);
            }
        }
        return {
            colors,
            filter: kind === 'set' ? 'nearest' : 'linear',
            domain,
            defaultColor,
        };
    }
    ColorTheme.Palette = Palette;
    ColorTheme.PaletteScale = (1 << 24) - 2; // reserve (1 << 24) - 1 for undefiend values
    ColorTheme.EmptyFactory = () => ColorTheme.Empty;
    const EmptyColor = Color(0xCCCCCC);
    ColorTheme.Empty = {
        factory: ColorTheme.EmptyFactory,
        granularity: 'uniform',
        color: () => EmptyColor,
        props: {}
    };
    function areEqual(themeA, themeB) {
        return themeA.contextHash === themeB.contextHash && themeA.factory === themeB.factory && deepEqual(themeA.props, themeB.props);
    }
    ColorTheme.areEqual = areEqual;
    ColorTheme.EmptyProvider = { name: '', label: '', category: '', factory: ColorTheme.EmptyFactory, getParams: () => ({}), defaultValues: {}, isApplicable: () => true };
    function createRegistry() {
        return new ThemeRegistry(ColorTheme.BuiltIn, ColorTheme.EmptyProvider);
    }
    ColorTheme.createRegistry = createRegistry;
    ColorTheme.BuiltIn = {
        'atom-id': AtomIdColorThemeProvider,
        'carbohydrate-symbol': CarbohydrateSymbolColorThemeProvider,
        'cartoon': CartoonColorThemeProvider,
        'chain-id': ChainIdColorThemeProvider,
        'element-index': ElementIndexColorThemeProvider,
        'element-symbol': ElementSymbolColorThemeProvider,
        'entity-id': EntityIdColorThemeProvider,
        'entity-source': EntitySourceColorThemeProvider,
        'external-structure': ExternalStructureColorThemeProvider,
        'external-volume': ExternalVolumeColorThemeProvider,
        'formal-charge': FormalChargeColorThemeProvider,
        'hydrophobicity': HydrophobicityColorThemeProvider,
        'illustrative': IllustrativeColorThemeProvider,
        'model-index': ModelIndexColorThemeProvider,
        'molecule-type': MoleculeTypeColorThemeProvider,
        'occupancy': OccupancyColorThemeProvider,
        'operator-hkl': OperatorHklColorThemeProvider,
        'operator-name': OperatorNameColorThemeProvider,
        'partial-charge': PartialChargeColorThemeProvider,
        'polymer-id': PolymerIdColorThemeProvider,
        'polymer-index': PolymerIndexColorThemeProvider,
        'residue-charge': ResidueChargeColorThemeProvider,
        'residue-name': ResidueNameColorThemeProvider,
        'secondary-structure': SecondaryStructureColorThemeProvider,
        'sequence-id': SequenceIdColorThemeProvider,
        'shape-group': ShapeGroupColorThemeProvider,
        'structure-index': StructureIndexColorThemeProvider,
        'trajectory-index': TrajectoryIndexColorThemeProvider,
        'uncertainty': UncertaintyColorThemeProvider,
        'unit-index': UnitIndexColorThemeProvider,
        'uniform': UniformColorThemeProvider,
        'volume-instance': VolumeInstanceColorThemeProvider,
        'volume-segment': VolumeSegmentColorThemeProvider,
        'volume-value': VolumeValueColorThemeProvider,
    };
})(ColorTheme || (ColorTheme = {}));
export function ColorThemeProvider(p) { return p; }
