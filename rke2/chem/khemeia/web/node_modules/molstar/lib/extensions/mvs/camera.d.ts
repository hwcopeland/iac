/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { Camera } from '../../mol-canvas3d/camera.js';
import { Canvas3DProps } from '../../mol-canvas3d/canvas3d.js';
import { PluginContext } from '../../mol-plugin/context.js';
import { PluginState } from '../../mol-plugin/state.js';
import { MolstarLoadingContext } from './load.js';
import { MVSAnimationNode } from './tree/animation/animation-tree.js';
import { MolstarNode, MolstarNodeParams } from './tree/molstar/molstar-tree.js';
/** Set the camera position to the current position (thus suppress automatic adjustment). */
export declare function suppressCameraAutoreset(plugin: PluginContext): Promise<void>;
/** Set the camera based on a camera node params. */
export declare function setCamera(plugin: PluginContext, params: MolstarNodeParams<'camera'>): Promise<void>;
export declare function cameraParamsToCameraSnapshot(plugin: PluginContext, params: MolstarNodeParams<'camera'>): Partial<Camera.Snapshot>;
/** Create object for PluginState.Snapshot.camera based on tree loading context and MVS snapshot metadata */
export declare function createPluginStateSnapshotCamera(plugin: PluginContext, context: MolstarLoadingContext, options: {
    previousTransitionDurationMs?: number;
    ignoreCameraOrientation?: boolean;
}): PluginState.Snapshot['camera'];
/** Create a deep copy of `oldCanvasProps` with values modified according to a canvas node params. */
export declare function modifyCanvasProps(oldCanvasProps: Canvas3DProps, canvasNode: MolstarNode<'canvas'> | undefined, animationNode: MVSAnimationNode<'animation'> | undefined): Canvas3DProps;
export declare function resetCanvasProps(plugin: PluginContext): void;
