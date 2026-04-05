/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Scene } from '../../mol-gl/scene.js';
import { MeshBuilder } from '../../mol-geo/geometry/mesh/mesh-builder.js';
import { Mat4, Vec3 } from '../../mol-math/linear-algebra.js';
import { Mesh } from '../../mol-geo/geometry/mesh/mesh.js';
import { ColorNames } from '../../mol-util/color/names.js';
import { ValueCell } from '../../mol-util/index.js';
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { Geometry } from '../../mol-geo/geometry/geometry.js';
import { addCylinderFromRay3D } from '../../mol-geo/geometry/mesh/builder/cylinder.js';
import { addSphere } from '../../mol-geo/geometry/mesh/builder/sphere.js';
import { Camera } from '../camera.js';
import { Viewport } from '../camera/util.js';
import { Shape } from '../../mol-model/shape/shape.js';
export const PointerHelperParams = {
    ...Mesh.Params,
    enabled: PD.Select('off', PD.arrayToOptions(['on', 'off']), { isEssential: true }),
    ignoreLight: { ...Mesh.Params.ignoreLight, defaultValue: true },
    color: PD.Color(ColorNames.grey, { isEssential: true }),
    hitColor: PD.Color(ColorNames.pink, { isEssential: true }),
};
export class PointerHelper {
    setProps(props) {
        Object.assign(this.props, props);
        if (this.isEnabled)
            this.update(this.pointers, this.points, this.hit);
    }
    ensureEnabled() {
        if (this.props.enabled !== 'on')
            this.props.enabled = 'on';
    }
    get isEnabled() {
        return this.props.enabled === 'on';
    }
    setCamera(camera) {
        Camera.copySnapshot(this.camera.state, camera.state);
        Viewport.copy(this.camera.viewport, camera.viewport);
        Mat4.copy(this.camera.view, camera.view);
        Mat4.copy(this.camera.projection, camera.projection);
        Mat4.copy(this.camera.projectionView, camera.projectionView);
        Mat4.copy(this.camera.headRotation, camera.headRotation);
        Camera.copyViewOffset(this.camera.viewOffset, camera.viewOffset);
        this.camera.far = camera.far;
        this.camera.near = camera.near;
        this.camera.fogFar = camera.fogFar;
        this.camera.fogNear = camera.fogNear;
        this.camera.forceFull = camera.forceFull;
        this.camera.scale = 1;
        this.modelScale = camera.scale;
    }
    update(pointers, points, hit) {
        this.pointers = pointers;
        this.points = points;
        this.hit = hit;
        const p = this.props;
        if (p.enabled !== 'on') {
            if (this.renderObject)
                this.renderObject.state.visible = false;
            return;
        }
        this.shape = getPointerMeshShape(this.getData(), this.props, this.shape);
        ValueCell.updateIfChanged(this.renderObject.values.drawCount, Geometry.getDrawCount(this.shape.geometry));
        ValueCell.updateIfChanged(this.renderObject.values.uVertexCount, Geometry.getVertexCount(this.shape.geometry));
        ValueCell.updateIfChanged(this.renderObject.values.uGroupCount, 2);
        Mesh.Utils.updateBoundingSphere(this.renderObject.values, this.shape.geometry);
        Mesh.Utils.updateValues(this.renderObject.values, this.props);
        Mesh.Utils.updateRenderableState(this.renderObject.state, this.props);
        this.renderObject.state.visible = true;
        this.scene.update(void 0, false);
        this.scene.commit();
    }
    getData() {
        return {
            pointers: this.pointers,
            points: this.points,
            hit: this.hit,
            modelScale: this.modelScale,
            camera: this.camera,
            pixels: 12,
        };
    }
    constructor(webgl, props = {}) {
        this.pixelScale = 1;
        this.modelScale = 1;
        this.pointers = [];
        this.points = [];
        this.hit = undefined;
        this.scene = Scene.create(webgl, 'blended');
        this.props = { ...PD.getDefaultValues(PointerHelperParams), ...props };
        this.camera = new Camera();
        this.shape = getPointerMeshShape(this.getData(), this.props, this.shape);
        this.renderObject = createMeshRenderObject(this.shape, this.props);
        this.scene.add(this.renderObject);
    }
}
export var PointerHelperGroup;
(function (PointerHelperGroup) {
    PointerHelperGroup[PointerHelperGroup["None"] = 0] = "None";
    PointerHelperGroup[PointerHelperGroup["Hit"] = 1] = "Hit";
})(PointerHelperGroup || (PointerHelperGroup = {}));
const tmpV = Vec3();
function getSizeForPixels(position, pixels, camera, modelScale) {
    const cameraPosition = Vec3.scale(tmpV, camera.state.position, modelScale);
    const d = Vec3.distance(position, cameraPosition);
    const height = 2 * Math.tan(camera.state.fov / 2) * d;
    return (height / camera.viewport.height) * pixels;
}
;
function createPointerMesh(data, mesh) {
    const state = MeshBuilder.createState(512, 256, mesh);
    const radius = 0.0005;
    const cylinderProps = { radiusTop: radius, radiusBottom: radius, radialSegments: 32 };
    const { modelScale, camera, pixels } = data;
    state.currentGroup = PointerHelperGroup.None;
    for (const pointer of data.pointers) {
        addCylinderFromRay3D(state, pointer, 0.2, cylinderProps);
        const size = getSizeForPixels(pointer.origin, pixels, camera, modelScale);
        addSphere(state, pointer.origin, size, 1);
    }
    for (const point of data.points) {
        const size = getSizeForPixels(point, pixels, camera, modelScale);
        addSphere(state, point, size, 1);
    }
    if (data.hit) {
        state.currentGroup = PointerHelperGroup.Hit;
        const size = getSizeForPixels(data.hit, pixels, camera, modelScale);
        addSphere(state, data.hit, size, 1);
    }
    return MeshBuilder.getMesh(state);
}
function getPointerMeshShape(data, props, shape) {
    const mesh = createPointerMesh(data, shape === null || shape === void 0 ? void 0 : shape.geometry);
    const getColor = (groupId) => {
        switch (groupId) {
            case PointerHelperGroup.Hit: return props.hitColor;
            default: return props.color;
        }
    };
    return Shape.create('pointer-mesh', data, mesh, getColor, () => 1, () => '', undefined, 2);
}
function createMeshRenderObject(shape, props) {
    return Shape.createRenderObject(shape, {
        ...PD.getDefaultValues(Mesh.Params),
        ...props,
        ignoreLight: props.ignoreLight,
        cellSize: 0,
    });
}
