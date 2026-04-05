"use strict";
/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.PointerHelperGroup = exports.PointerHelper = exports.PointerHelperParams = void 0;
const scene_1 = require("../../mol-gl/scene.js");
const mesh_builder_1 = require("../../mol-geo/geometry/mesh/mesh-builder.js");
const linear_algebra_1 = require("../../mol-math/linear-algebra.js");
const mesh_1 = require("../../mol-geo/geometry/mesh/mesh.js");
const names_1 = require("../../mol-util/color/names.js");
const mol_util_1 = require("../../mol-util/index.js");
const param_definition_1 = require("../../mol-util/param-definition.js");
const geometry_1 = require("../../mol-geo/geometry/geometry.js");
const cylinder_1 = require("../../mol-geo/geometry/mesh/builder/cylinder.js");
const sphere_1 = require("../../mol-geo/geometry/mesh/builder/sphere.js");
const camera_1 = require("../camera.js");
const util_1 = require("../camera/util.js");
const shape_1 = require("../../mol-model/shape/shape.js");
exports.PointerHelperParams = {
    ...mesh_1.Mesh.Params,
    enabled: param_definition_1.ParamDefinition.Select('off', param_definition_1.ParamDefinition.arrayToOptions(['on', 'off']), { isEssential: true }),
    ignoreLight: { ...mesh_1.Mesh.Params.ignoreLight, defaultValue: true },
    color: param_definition_1.ParamDefinition.Color(names_1.ColorNames.grey, { isEssential: true }),
    hitColor: param_definition_1.ParamDefinition.Color(names_1.ColorNames.pink, { isEssential: true }),
};
class PointerHelper {
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
        camera_1.Camera.copySnapshot(this.camera.state, camera.state);
        util_1.Viewport.copy(this.camera.viewport, camera.viewport);
        linear_algebra_1.Mat4.copy(this.camera.view, camera.view);
        linear_algebra_1.Mat4.copy(this.camera.projection, camera.projection);
        linear_algebra_1.Mat4.copy(this.camera.projectionView, camera.projectionView);
        linear_algebra_1.Mat4.copy(this.camera.headRotation, camera.headRotation);
        camera_1.Camera.copyViewOffset(this.camera.viewOffset, camera.viewOffset);
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
        mol_util_1.ValueCell.updateIfChanged(this.renderObject.values.drawCount, geometry_1.Geometry.getDrawCount(this.shape.geometry));
        mol_util_1.ValueCell.updateIfChanged(this.renderObject.values.uVertexCount, geometry_1.Geometry.getVertexCount(this.shape.geometry));
        mol_util_1.ValueCell.updateIfChanged(this.renderObject.values.uGroupCount, 2);
        mesh_1.Mesh.Utils.updateBoundingSphere(this.renderObject.values, this.shape.geometry);
        mesh_1.Mesh.Utils.updateValues(this.renderObject.values, this.props);
        mesh_1.Mesh.Utils.updateRenderableState(this.renderObject.state, this.props);
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
        this.scene = scene_1.Scene.create(webgl, 'blended');
        this.props = { ...param_definition_1.ParamDefinition.getDefaultValues(exports.PointerHelperParams), ...props };
        this.camera = new camera_1.Camera();
        this.shape = getPointerMeshShape(this.getData(), this.props, this.shape);
        this.renderObject = createMeshRenderObject(this.shape, this.props);
        this.scene.add(this.renderObject);
    }
}
exports.PointerHelper = PointerHelper;
var PointerHelperGroup;
(function (PointerHelperGroup) {
    PointerHelperGroup[PointerHelperGroup["None"] = 0] = "None";
    PointerHelperGroup[PointerHelperGroup["Hit"] = 1] = "Hit";
})(PointerHelperGroup || (exports.PointerHelperGroup = PointerHelperGroup = {}));
const tmpV = (0, linear_algebra_1.Vec3)();
function getSizeForPixels(position, pixels, camera, modelScale) {
    const cameraPosition = linear_algebra_1.Vec3.scale(tmpV, camera.state.position, modelScale);
    const d = linear_algebra_1.Vec3.distance(position, cameraPosition);
    const height = 2 * Math.tan(camera.state.fov / 2) * d;
    return (height / camera.viewport.height) * pixels;
}
;
function createPointerMesh(data, mesh) {
    const state = mesh_builder_1.MeshBuilder.createState(512, 256, mesh);
    const radius = 0.0005;
    const cylinderProps = { radiusTop: radius, radiusBottom: radius, radialSegments: 32 };
    const { modelScale, camera, pixels } = data;
    state.currentGroup = PointerHelperGroup.None;
    for (const pointer of data.pointers) {
        (0, cylinder_1.addCylinderFromRay3D)(state, pointer, 0.2, cylinderProps);
        const size = getSizeForPixels(pointer.origin, pixels, camera, modelScale);
        (0, sphere_1.addSphere)(state, pointer.origin, size, 1);
    }
    for (const point of data.points) {
        const size = getSizeForPixels(point, pixels, camera, modelScale);
        (0, sphere_1.addSphere)(state, point, size, 1);
    }
    if (data.hit) {
        state.currentGroup = PointerHelperGroup.Hit;
        const size = getSizeForPixels(data.hit, pixels, camera, modelScale);
        (0, sphere_1.addSphere)(state, data.hit, size, 1);
    }
    return mesh_builder_1.MeshBuilder.getMesh(state);
}
function getPointerMeshShape(data, props, shape) {
    const mesh = createPointerMesh(data, shape === null || shape === void 0 ? void 0 : shape.geometry);
    const getColor = (groupId) => {
        switch (groupId) {
            case PointerHelperGroup.Hit: return props.hitColor;
            default: return props.color;
        }
    };
    return shape_1.Shape.create('pointer-mesh', data, mesh, getColor, () => 1, () => '', undefined, 2);
}
function createMeshRenderObject(shape, props) {
    return shape_1.Shape.createRenderObject(shape, {
        ...param_definition_1.ParamDefinition.getDefaultValues(mesh_1.Mesh.Params),
        ...props,
        ignoreLight: props.ignoreLight,
        cellSize: 0,
    });
}
