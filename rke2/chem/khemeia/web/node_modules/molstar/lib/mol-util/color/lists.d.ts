/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author Adam Midlik <midlik@gmail.com>
 */
import { type ParamDefinition as PD } from '../param-definition.js';
import { ColorList } from './color.js';
export declare const ColorLists: {
    reds: ColorList;
    oranges: ColorList;
    greens: ColorList;
    blues: ColorList;
    purples: ColorList;
    greys: ColorList;
    'orange-red': ColorList;
    'blue-green': ColorList;
    'purple-blue-green': ColorList;
    'green-blue': ColorList;
    'purple-blue': ColorList;
    'blue-purple': ColorList;
    'red-purple': ColorList;
    'purple-red': ColorList;
    'yellow-orange-red': ColorList;
    'yellow-orange-brown': ColorList;
    'yellow-green': ColorList;
    'yellow-green-blue': ColorList;
    'red-blue': ColorList;
    'red-grey': ColorList;
    'pink-yellow-green': ColorList;
    'brown-white-green': ColorList;
    'purple-green': ColorList;
    'purple-orange': ColorList;
    'orange-purple': ColorList;
    'red-yellow-green': ColorList;
    'red-yellow-blue': ColorList;
    spectral: ColorList;
    magma: ColorList;
    'magma-no-black': ColorList;
    inferno: ColorList;
    'inferno-no-black': ColorList;
    plasma: ColorList;
    viridis: ColorList;
    cividis: ColorList;
    twilight: ColorList;
    'set-2': ColorList;
    accent: ColorList;
    'set-1': ColorList;
    'set-3': ColorList;
    'dark-2': ColorList;
    paired: ColorList;
    'pastel-2': ColorList;
    'pastel-1': ColorList;
    turbo: ColorList;
    'turbo-no-black': ColorList;
    warm: ColorList;
    cool: ColorList;
    'cubehelix-default': ColorList;
    rainbow: ColorList;
    sinebow: ColorList;
    'category-10': ColorList;
    'observable-10': ColorList;
    'tableau-10': ColorList;
    'simple-rainbow': ColorList;
    'red-white-blue': ColorList;
    'many-distinct': ColorList;
};
export type ColorListName = keyof typeof ColorLists;
export declare const ColorListNames: ColorListName[];
export declare const ColorListOptions: PD.SelectOption<ColorListName>[];
export declare const ColorListOptionsScale: PD.SelectOption<ColorListName>[];
export declare const ColorListOptionsSet: PD.SelectOption<ColorListName>[];
export declare function getColorListFromName(name: ColorListName): ColorList;
