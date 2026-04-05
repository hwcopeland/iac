/**
 * Copyright (c) 2022-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { WebGLContext } from '../../mol-gl/webgl/context.js';
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { ICamera } from '../camera.js';
import { Color } from '../../mol-util/color/index.js';
import { Asset, AssetManager } from '../../mol-util/assets.js';
import { PostprocessingProps } from './postprocessing.js';
export declare const BackgroundParams: {
    variant: PD.Mapped<PD.NamedParams<PD.Normalize<unknown>, "off"> | PD.NamedParams<PD.Normalize<{
        coverage: "canvas" | "viewport";
        opacity: number;
        saturation: number;
        lightness: number;
        source: PD.NamedParams<any, "url"> | PD.NamedParams<Asset.File | null, "file">;
        blur: number;
    }>, "image"> | PD.NamedParams<PD.Normalize<{
        centerColor: Color;
        edgeColor: Color;
        ratio: number;
        coverage: "canvas" | "viewport";
    }>, "radialGradient"> | PD.NamedParams<PD.Normalize<{
        opacity: number;
        saturation: number;
        lightness: number;
        faces: PD.NamedParams<PD.Normalize<{
            nx: /*elided*/ any;
            ny: /*elided*/ any;
            nz: /*elided*/ any;
            px: /*elided*/ any;
            py: /*elided*/ any;
            pz: /*elided*/ any;
        }>, "urls"> | PD.NamedParams<PD.Normalize<{
            nx: /*elided*/ any;
            ny: /*elided*/ any;
            nz: /*elided*/ any;
            px: /*elided*/ any;
            py: /*elided*/ any;
            pz: /*elided*/ any;
        }>, "files">;
        blur: number;
        rotation: PD.Normalize<{
            x: /*elided*/ any;
            y: /*elided*/ any;
            z: /*elided*/ any;
        }>;
    }>, "skybox"> | PD.NamedParams<PD.Normalize<{
        topColor: Color;
        bottomColor: Color;
        ratio: number;
        coverage: "canvas" | "viewport";
    }>, "horizontalGradient">>;
};
export type BackgroundProps = PD.Values<typeof BackgroundParams>;
export declare class BackgroundPass {
    private readonly webgl;
    private readonly assetManager;
    private renderable;
    private skybox;
    private image;
    private readonly camera;
    private readonly target;
    private readonly position;
    private readonly dir;
    constructor(webgl: WebGLContext, assetManager: AssetManager, width: number, height: number);
    setSize(width: number, height: number): void;
    private clearSkybox;
    private updateSkybox;
    private clearImage;
    private updateImage;
    private updateImageScaling;
    private updateGradient;
    update(camera: ICamera, props: BackgroundProps, onload?: (changed: boolean) => void): void;
    private _isEnabled;
    isEnabled(props: PostprocessingProps): boolean;
    private isReady;
    private readonly bgColor;
    clear(props: BackgroundProps, transparentBackground: boolean, backgroundColor: Color): void;
    render(props: BackgroundProps): void;
    dispose(): void;
}
