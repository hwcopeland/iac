/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { CameraFogParams, DefaultCanvas3DParams } from '../../mol-canvas3d/canvas3d.js';
import { TrackballControlsParams } from '../../mol-canvas3d/controls/trackball.js';
import { BackgroundParams } from '../../mol-canvas3d/passes/background.js';
import { BloomParams } from '../../mol-canvas3d/passes/bloom.js';
import { DofParams } from '../../mol-canvas3d/passes/dof.js';
import { OutlineParams } from '../../mol-canvas3d/passes/outline.js';
import { ShadowParams } from '../../mol-canvas3d/passes/shadow.js';
import { SsaoParams } from '../../mol-canvas3d/passes/ssao.js';
import { Vec3 } from '../../mol-math/linear-algebra.js';
import { getPluginBoundingSphere } from '../../mol-plugin-state/manager/focus-camera/focus-object.js';
import { PluginCommands } from '../../mol-plugin/commands.js';
import { StateTransform } from '../../mol-state/index.js';
import { fovAdjustedPosition } from '../../mol-util/camera.js';
import { ColorNames } from '../../mol-util/color/names.js';
import { deepClone } from '../../mol-util/object.js';
import { ParamDefinition } from '../../mol-util/param-definition.js';
import { decodeColor } from './helpers/utils.js';
import { MVSTreeSchema } from './tree/mvs/mvs-tree.js';
const DefaultCanvasBackgroundColor = ColorNames.white;
const _tmpVec = Vec3();
/** Set the camera position to the current position (thus suppress automatic adjustment). */
export async function suppressCameraAutoreset(plugin) {
    var _a;
    const snapshot = { ...(_a = plugin.canvas3d) === null || _a === void 0 ? void 0 : _a.camera.state, radius: Infinity }; // `radius: Infinity` avoids clipping when the scene expands
    adjustSceneRadiusFactor(plugin, snapshot.target);
    await PluginCommands.Camera.SetSnapshot(plugin, { snapshot });
}
/** Set the camera based on a camera node params. */
export async function setCamera(plugin, params) {
    const snapshot = cameraParamsToCameraSnapshot(plugin, params);
    adjustSceneRadiusFactor(plugin, snapshot.target);
    await PluginCommands.Camera.SetSnapshot(plugin, { snapshot });
}
export function cameraParamsToCameraSnapshot(plugin, params) {
    var _a;
    const target = Vec3.create(...params.target);
    let position = Vec3.create(...params.position);
    const radius = Vec3.distance(target, position) / 2;
    if (plugin.canvas3d)
        position = fovAdjustedPosition(target, position, plugin.canvas3d.camera.state.mode, plugin.canvas3d.camera.state.fov);
    const up = Vec3.create(...params.up);
    Vec3.orthogonalize(up, Vec3.sub(_tmpVec, target, position), up);
    const snapshot = {
        target,
        position,
        up,
        radius,
        radiusMax: radius,
        minNear: (_a = params.near) !== null && _a !== void 0 ? _a : undefined,
    };
    return snapshot;
}
function snapshotFocusInfoFromMvsFocuses(focuses, ignoreOrientation) {
    var _a, _b;
    const lastFocus = (focuses.length > 0) ? focuses[focuses.length - 1] : undefined;
    const direction = (_a = lastFocus === null || lastFocus === void 0 ? void 0 : lastFocus.params.direction) !== null && _a !== void 0 ? _a : MVSTreeSchema.nodes.focus.params.fields.direction.default;
    const up = (_b = lastFocus === null || lastFocus === void 0 ? void 0 : lastFocus.params.up) !== null && _b !== void 0 ? _b : MVSTreeSchema.nodes.focus.params.fields.up.default;
    return {
        targets: focuses.map(f => {
            var _a, _b, _c;
            return ({
                targetRef: ((_a = f.target) === null || _a === void 0 ? void 0 : _a.ref) === StateTransform.RootRef ? undefined : (_b = f.target) === null || _b === void 0 ? void 0 : _b.ref, // need to treat root separately so it does not include invisible structure parts etc.
                center: f.params.center ? Vec3.create(...f.params.center) : undefined,
                radius: (_c = f.params.radius) !== null && _c !== void 0 ? _c : undefined,
                radiusFactor: f.params.radius_factor,
                extraRadius: f.params.radius_extent,
            });
        }),
        direction: ignoreOrientation ? undefined : Vec3.create(...direction),
        up: ignoreOrientation ? undefined : Vec3.create(...up),
    };
}
/** Adjust `sceneRadiusFactor` property so that the current scene is not cropped */
function adjustSceneRadiusFactor(plugin, cameraTarget) {
    var _a;
    if (!cameraTarget)
        return;
    const boundingSphere = getPluginBoundingSphere(plugin);
    const offset = Vec3.distance(cameraTarget, boundingSphere.center);
    const sceneRadiusFactor = boundingSphere.radius > 0 ? ((boundingSphere.radius + offset) / boundingSphere.radius) : 1;
    (_a = plugin.canvas3d) === null || _a === void 0 ? void 0 : _a.setProps({ sceneRadiusFactor });
}
/** Create object for PluginState.Snapshot.camera based on tree loading context and MVS snapshot metadata */
export function createPluginStateSnapshotCamera(plugin, context, options) {
    var _a, _b;
    const camera = {
        transitionStyle: 'animate',
        transitionDurationInMs: (_a = options.previousTransitionDurationMs) !== null && _a !== void 0 ? _a : 0,
    };
    if (context.camera.cameraParams !== undefined) {
        const cam = context.camera.cameraParams;
        if (options.ignoreCameraOrientation) {
            camera.focus = snapshotFocusInfoFromMvsFocuses([{
                    target: undefined,
                    params: {
                        center: cam.target,
                        radius: Vec3.distance(cam.target, cam.position) / 2,
                        direction: MVSTreeSchema.nodes.focus.params.fields.direction.default, // will be ignored
                        up: MVSTreeSchema.nodes.focus.params.fields.up.default, // will be ignored
                        radius_factor: 1, // will be ignored
                        radius_extent: 0, // will be ignored
                    },
                }], true);
            // This will not work exactly when viewport height>width because of how focusing works (could be solved by adjusting radius by aspect ration, but that would mess up cropping, and wouldn't work properly when aspect ration changes after loading)
        }
        else {
            const currentCameraSnapshot = plugin.canvas3d.camera.getSnapshot();
            const cameraSnapshot = cameraParamsToCameraSnapshot(plugin, cam);
            camera.current = { ...currentCameraSnapshot, ...cameraSnapshot };
        }
    }
    else {
        camera.focus = snapshotFocusInfoFromMvsFocuses(context.camera.focuses, (_b = options.ignoreCameraOrientation) !== null && _b !== void 0 ? _b : false);
    }
    return camera;
}
function optionalParams(enable, values, params, fallback) {
    if (typeof enable === 'boolean') {
        return enable
            ? { name: 'on', params: { ...ParamDefinition.getDefaultValues(params), ...values } }
            : { name: 'off', params: {} };
    }
    return fallback;
}
function normalizeBackground(variant, prev) {
    if (!variant)
        return prev;
    return ParamDefinition.normalizeParams(BackgroundParams, { variant }, 'children');
}
/** Create a deep copy of `oldCanvasProps` with values modified according to a canvas node params. */
export function modifyCanvasProps(oldCanvasProps, canvasNode, animationNode) {
    var _a, _b, _c, _d, _e;
    const params = canvasNode === null || canvasNode === void 0 ? void 0 : canvasNode.params;
    const backgroundColor = (_a = decodeColor(params === null || params === void 0 ? void 0 : params.background_color)) !== null && _a !== void 0 ? _a : DefaultCanvasBackgroundColor;
    const molstar_postprocessing = (_b = canvasNode === null || canvasNode === void 0 ? void 0 : canvasNode.custom) === null || _b === void 0 ? void 0 : _b.molstar_postprocessing;
    const outline = molstar_postprocessing === null || molstar_postprocessing === void 0 ? void 0 : molstar_postprocessing.enable_outline;
    const outlineParams = molstar_postprocessing === null || molstar_postprocessing === void 0 ? void 0 : molstar_postprocessing.outline_params;
    const shadow = molstar_postprocessing === null || molstar_postprocessing === void 0 ? void 0 : molstar_postprocessing.enable_shadow;
    const shadowParams = molstar_postprocessing === null || molstar_postprocessing === void 0 ? void 0 : molstar_postprocessing.shadow_params;
    const occlusion = molstar_postprocessing === null || molstar_postprocessing === void 0 ? void 0 : molstar_postprocessing.enable_ssao;
    const occlusionParams = molstar_postprocessing === null || molstar_postprocessing === void 0 ? void 0 : molstar_postprocessing.ssao_params;
    const fog = molstar_postprocessing === null || molstar_postprocessing === void 0 ? void 0 : molstar_postprocessing.enable_fog;
    const fogParams = molstar_postprocessing === null || molstar_postprocessing === void 0 ? void 0 : molstar_postprocessing.fog_params;
    const dof = molstar_postprocessing === null || molstar_postprocessing === void 0 ? void 0 : molstar_postprocessing.enable_depth_of_field;
    const dofParams = molstar_postprocessing === null || molstar_postprocessing === void 0 ? void 0 : molstar_postprocessing.depth_of_field_params;
    const bloom = molstar_postprocessing === null || molstar_postprocessing === void 0 ? void 0 : molstar_postprocessing.enable_bloom;
    const bloomParams = molstar_postprocessing === null || molstar_postprocessing === void 0 ? void 0 : molstar_postprocessing.bloom_params;
    const background = molstar_postprocessing === null || molstar_postprocessing === void 0 ? void 0 : molstar_postprocessing.background;
    const trackballAnimation = (_c = animationNode === null || animationNode === void 0 ? void 0 : animationNode.custom) === null || _c === void 0 ? void 0 : _c.molstar_trackball;
    const trackballAnimationName = trackballAnimation === null || trackballAnimation === void 0 ? void 0 : trackballAnimation.name;
    const trackballAnimationParams = (_d = trackballAnimation === null || trackballAnimation === void 0 ? void 0 : trackballAnimation.params) !== null && _d !== void 0 ? _d : {};
    return {
        ...oldCanvasProps,
        postprocessing: {
            ...oldCanvasProps.postprocessing,
            outline: optionalParams(outline, outlineParams, OutlineParams, oldCanvasProps.postprocessing.outline),
            shadow: optionalParams(shadow, shadowParams, ShadowParams, oldCanvasProps.postprocessing.shadow),
            occlusion: optionalParams(occlusion, occlusionParams, SsaoParams, oldCanvasProps.postprocessing.occlusion),
            dof: optionalParams(dof, dofParams, DofParams, oldCanvasProps.postprocessing.dof),
            bloom: optionalParams(bloom, bloomParams, BloomParams, oldCanvasProps.postprocessing.bloom),
            background: normalizeBackground(background, oldCanvasProps.postprocessing.background),
        },
        cameraFog: optionalParams(fog, fogParams, CameraFogParams, oldCanvasProps.cameraFog),
        renderer: {
            ...oldCanvasProps.renderer,
            backgroundColor: backgroundColor,
        },
        trackball: {
            ...oldCanvasProps === null || oldCanvasProps === void 0 ? void 0 : oldCanvasProps.trackball,
            ...(trackballAnimationName
                ? {
                    animate: {
                        name: trackballAnimationName,
                        params: {
                            ...(_e = TrackballControlsParams.animate.map(trackballAnimationName)) === null || _e === void 0 ? void 0 : _e.defaultValue,
                            ...trackballAnimationParams
                        }
                    }
                }
                : {}),
        }
    };
}
export function resetCanvasProps(plugin) {
    var _a, _b;
    const old = (_a = plugin.canvas3d) === null || _a === void 0 ? void 0 : _a.props;
    (_b = plugin.canvas3d) === null || _b === void 0 ? void 0 : _b.setProps({
        ...old,
        postprocessing: {
            ...old,
            outline: deepClone(DefaultCanvas3DParams.postprocessing.outline),
            shadow: deepClone(DefaultCanvas3DParams.postprocessing.shadow),
            occlusion: deepClone(DefaultCanvas3DParams.postprocessing.occlusion),
            dof: deepClone(DefaultCanvas3DParams.postprocessing.dof),
            bloom: deepClone(DefaultCanvas3DParams.postprocessing.bloom),
            background: deepClone(DefaultCanvas3DParams.postprocessing.background),
        },
        cameraFog: deepClone(DefaultCanvas3DParams.cameraFog),
        trackball: {
            ...old === null || old === void 0 ? void 0 : old.trackball,
            animate: { name: 'off', params: {} },
        }
    });
}
