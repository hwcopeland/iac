/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { UniformSizeThemeProvider } from './size/uniform.js';
import { ThemeRegistry } from '../mol-theme/theme.js';
import { PhysicalSizeThemeProvider } from './size/physical.js';
import { deepEqual } from '../mol-util/index.js';
import { ShapeGroupSizeThemeProvider } from './size/shape-group.js';
import { UncertaintySizeThemeProvider } from './size/uncertainty.js';
import { VolumeValueSizeThemeProvider } from './size/volume-value.js';
export { SizeTheme };
var SizeTheme;
(function (SizeTheme) {
    SizeTheme.EmptyFactory = () => SizeTheme.Empty;
    SizeTheme.Empty = { factory: SizeTheme.EmptyFactory, granularity: 'uniform', size: () => 1, props: {} };
    function areEqual(themeA, themeB) {
        return themeA.contextHash === themeB.contextHash && themeA.factory === themeB.factory && deepEqual(themeA.props, themeB.props);
    }
    SizeTheme.areEqual = areEqual;
    SizeTheme.EmptyProvider = { name: '', label: '', category: '', factory: SizeTheme.EmptyFactory, getParams: () => ({}), defaultValues: {}, isApplicable: () => true };
    function createRegistry() {
        return new ThemeRegistry(SizeTheme.BuiltIn, SizeTheme.EmptyProvider);
    }
    SizeTheme.createRegistry = createRegistry;
    SizeTheme.BuiltIn = {
        'physical': PhysicalSizeThemeProvider,
        'shape-group': ShapeGroupSizeThemeProvider,
        'uncertainty': UncertaintySizeThemeProvider,
        'uniform': UniformSizeThemeProvider,
        'volume-value': VolumeValueSizeThemeProvider,
    };
})(SizeTheme || (SizeTheme = {}));
