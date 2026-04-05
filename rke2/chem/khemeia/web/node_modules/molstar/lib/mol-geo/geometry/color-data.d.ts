/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Gianluca Tomasello <giagitom@gmail.com>
 */
import { ValueCell } from '../../mol-util/index.js';
import { TextureImage } from '../../mol-gl/renderable/util.js';
import { Color } from '../../mol-util/color/index.js';
import { Vec2, Vec3, Vec4 } from '../../mol-math/linear-algebra.js';
import { LocationIterator } from '../util/location-iterator.js';
import { ColorTheme, ColorVolume } from '../../mol-theme/color.js';
import { Texture } from '../../mol-gl/webgl/texture.js';
export type ColorTypeLocation = 'uniform' | 'instance' | 'group' | 'groupInstance' | 'vertex' | 'vertexInstance';
export type ColorTypeGrid = 'volume' | 'volumeInstance';
export type ColorTypeDirect = 'direct';
export type ColorType = ColorTypeLocation | ColorTypeGrid | ColorTypeDirect;
export type ColorData = {
    uColor: ValueCell<Vec3>;
    tColor: ValueCell<TextureImage<Uint8Array>>;
    tColorGrid: ValueCell<Texture>;
    uPaletteDomain: ValueCell<Vec2>;
    uPaletteDefault: ValueCell<Vec3>;
    tPalette: ValueCell<TextureImage<Uint8Array>>;
    uColorTexDim: ValueCell<Vec2>;
    uColorGridDim: ValueCell<Vec3>;
    uColorGridTransform: ValueCell<Vec4>;
    dColorType: ValueCell<string>;
    dUsePalette: ValueCell<boolean>;
};
export declare function createColors(locationIt: LocationIterator, positionIt: LocationIterator, colorTheme: ColorTheme<any, any>, colorData?: ColorData): ColorData;
export declare function createValueColor(value: Color, colorData?: ColorData): ColorData;
export declare function createTextureColor(colors: TextureImage<Uint8Array>, type: ColorType, colorData?: ColorData): ColorData;
export declare function createGridColor(grid: ColorVolume, type: ColorType, colorData?: ColorData): ColorData;
