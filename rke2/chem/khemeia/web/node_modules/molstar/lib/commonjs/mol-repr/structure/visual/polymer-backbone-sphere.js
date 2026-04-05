"use strict";
/**
 * Copyright (c) 2021-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.PolymerBackboneSphereParams = void 0;
exports.PolymerBackboneSphereVisual = PolymerBackboneSphereVisual;
exports.PolymerBackboneSphereImpostorVisual = PolymerBackboneSphereImpostorVisual;
exports.PolymerBackboneSphereMeshVisual = PolymerBackboneSphereMeshVisual;
const param_definition_1 = require("../../../mol-util/param-definition.js");
const structure_1 = require("../../../mol-model/structure.js");
const mesh_1 = require("../../../mol-geo/geometry/mesh/mesh.js");
const mesh_builder_1 = require("../../../mol-geo/geometry/mesh/mesh-builder.js");
const linear_algebra_1 = require("../../../mol-math/linear-algebra.js");
const polymer_1 = require("./util/polymer.js");
const units_visual_1 = require("../units-visual.js");
const base_1 = require("../../../mol-geo/geometry/base.js");
const geometry_1 = require("../../../mol-math/geometry.js");
const sphere_1 = require("../../../mol-geo/geometry/mesh/builder/sphere.js");
const sphere_2 = require("../../../mol-geo/primitive/sphere.js");
const spheres_1 = require("../../../mol-geo/geometry/spheres/spheres.js");
const spheres_builder_1 = require("../../../mol-geo/geometry/spheres/spheres-builder.js");
const backbone_1 = require("./util/polymer/backbone.js");
const common_1 = require("./util/common.js");
exports.PolymerBackboneSphereParams = {
    ...units_visual_1.UnitsMeshParams,
    ...units_visual_1.UnitsSpheresParams,
    sizeFactor: param_definition_1.ParamDefinition.Numeric(0.3, { min: 0, max: 10, step: 0.01 }),
    detail: param_definition_1.ParamDefinition.Numeric(0, { min: 0, max: 3, step: 1 }, base_1.BaseGeometry.CustomQualityParamInfo),
    tryUseImpostor: param_definition_1.ParamDefinition.Boolean(true),
};
function PolymerBackboneSphereVisual(materialId, structure, props, webgl) {
    return props.tryUseImpostor && (0, common_1.checkSphereImpostorSupport)(webgl)
        ? PolymerBackboneSphereImpostorVisual(materialId)
        : PolymerBackboneSphereMeshVisual(materialId);
}
function createPolymerBackboneSphereImpostor(ctx, unit, structure, theme, props, spheres) {
    const polymerElementCount = unit.polymerElements.length;
    if (!polymerElementCount)
        return spheres_1.Spheres.createEmpty(spheres);
    const builder = spheres_builder_1.SpheresBuilder.create(polymerElementCount, polymerElementCount / 2, spheres);
    const c = unit.conformation;
    const p = (0, linear_algebra_1.Vec3)();
    const add = (index, group) => {
        c.invariantPosition(index, p);
        builder.add(p[0], p[1], p[2], group);
    };
    (0, backbone_1.eachPolymerBackboneElement)(unit, add);
    const s = builder.getSpheres();
    const sphere = geometry_1.Sphere3D.expand((0, geometry_1.Sphere3D)(), unit.boundary.sphere, 1 * props.sizeFactor);
    s.setBoundingSphere(sphere);
    return s;
}
function PolymerBackboneSphereImpostorVisual(materialId) {
    return (0, units_visual_1.UnitsSpheresVisual)({
        defaultProps: param_definition_1.ParamDefinition.getDefaultValues(exports.PolymerBackboneSphereParams),
        createGeometry: createPolymerBackboneSphereImpostor,
        createLocationIterator: (structureGroup) => polymer_1.PolymerLocationIterator.fromGroup(structureGroup),
        getLoci: polymer_1.getPolymerElementLoci,
        eachLocation: polymer_1.eachPolymerElement,
        setUpdateState: (state, newProps, currentProps) => { },
        mustRecreate: (structureGroup, props, webgl) => {
            return !props.tryUseImpostor || !webgl;
        }
    }, materialId);
}
function createPolymerBackboneSphereMesh(ctx, unit, structure, theme, props, mesh) {
    const polymerElementCount = unit.polymerElements.length;
    if (!polymerElementCount)
        return mesh_1.Mesh.createEmpty(mesh);
    const { detail, sizeFactor } = props;
    const vertexCount = polymerElementCount * (0, sphere_2.sphereVertexCount)(detail);
    const builderState = mesh_builder_1.MeshBuilder.createState(vertexCount, vertexCount / 2, mesh);
    const c = unit.conformation;
    const p = (0, linear_algebra_1.Vec3)();
    const center = structure_1.StructureElement.Location.create(structure, unit);
    const add = (index, group) => {
        center.element = index;
        c.invariantPosition(center.element, p);
        builderState.currentGroup = group;
        (0, sphere_1.addSphere)(builderState, p, theme.size.size(center) * sizeFactor, detail);
    };
    (0, backbone_1.eachPolymerBackboneElement)(unit, add);
    const m = mesh_builder_1.MeshBuilder.getMesh(builderState);
    const sphere = geometry_1.Sphere3D.expand((0, geometry_1.Sphere3D)(), unit.boundary.sphere, 1 * props.sizeFactor);
    m.setBoundingSphere(sphere);
    return m;
}
function PolymerBackboneSphereMeshVisual(materialId) {
    return (0, units_visual_1.UnitsMeshVisual)({
        defaultProps: param_definition_1.ParamDefinition.getDefaultValues(exports.PolymerBackboneSphereParams),
        createGeometry: createPolymerBackboneSphereMesh,
        createLocationIterator: (structureGroup) => polymer_1.PolymerLocationIterator.fromGroup(structureGroup),
        getLoci: polymer_1.getPolymerElementLoci,
        eachLocation: polymer_1.eachPolymerElement,
        setUpdateState: (state, newProps, currentProps) => {
            state.createGeometry = (newProps.sizeFactor !== currentProps.sizeFactor ||
                newProps.detail !== currentProps.detail);
        },
        mustRecreate: (structureGroup, props, webgl) => {
            return props.tryUseImpostor && !!webgl;
        }
    }, materialId);
}
