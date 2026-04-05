import { Vec3 } from '../../../mol-math/linear-algebra/3d/vec3.js';
import { PluginContext } from '../../../mol-plugin/context.js';
import { PrincipalAxes } from '../../../mol-math/linear-algebra/matrix/principal-axes.js';
import { StructureComponentRef } from '../structure/hierarchy-state.js';
import { Camera } from '../../../mol-canvas3d/camera.js';
export declare function pcaFocus(plugin: PluginContext, radius: number, options: {
    principalAxes: PrincipalAxes;
    positionToFlip?: Vec3;
}): Partial<Camera.Snapshot> | undefined;
interface PcaTransformData {
    principalAxes: PrincipalAxes;
    positionToFlip: Vec3;
}
export declare function getPcaTransform(group: StructureComponentRef[]): PcaTransformData | undefined;
export {};
