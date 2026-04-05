/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { UUID } from '../../mol-util/index.js';
import { OrderedSet } from '../../mol-data/int.js';
import { Geometry } from '../../mol-geo/geometry/geometry.js';
import { Mat4, Vec3 } from '../../mol-math/linear-algebra.js';
import { Sphere3D } from '../../mol-math/geometry.js';
import { CentroidHelper } from '../../mol-math/geometry/centroid-helper.js';
import { ShapeGroupSizeTheme } from '../../mol-theme/size/shape-group.js';
import { ShapeGroupColorTheme } from '../../mol-theme/color/shape-group.js';
import { createTransform as _createTransform } from '../../mol-geo/geometry/transform-data.js';
import { createRenderObject as _createRenderObject, getNextMaterialId } from '../../mol-gl/render-object.js';
import { LocationIterator } from '../../mol-geo/util/location-iterator.js';
export var Shape;
(function (Shape) {
    function create(name, sourceData, geometry, getColor, getSize, getLabel, transforms, groupCount) {
        return {
            id: UUID.create22(),
            name,
            sourceData,
            geometry,
            transforms: transforms || [Mat4.identity()],
            get groupCount() {
                return groupCount !== null && groupCount !== void 0 ? groupCount : Geometry.getGroupCount(geometry);
            },
            getColor,
            getSize,
            getLabel
        };
    }
    Shape.create = create;
    function getTheme(shape) {
        return {
            color: ShapeGroupColorTheme({ shape }, {}),
            size: ShapeGroupSizeTheme({ shape }, {})
        };
    }
    Shape.getTheme = getTheme;
    function groupIterator(shape) {
        const instanceCount = shape.transforms.length;
        const location = ShapeGroup.Location(shape);
        const getLocation = (groupIndex, instanceIndex) => {
            location.group = groupIndex;
            location.instance = instanceIndex;
            return location;
        };
        return LocationIterator(shape.groupCount, instanceCount, 1, getLocation);
    }
    Shape.groupIterator = groupIterator;
    function createTransform(transforms, invariantBoundingSphere, cellSize, batchSize, transformData) {
        const transformArray = transformData && transformData.aTransform.ref.value.length >= transforms.length * 16 ? transformData.aTransform.ref.value : new Float32Array(transforms.length * 16);
        for (let i = 0, il = transforms.length; i < il; ++i) {
            Mat4.toArray(transforms[i], transformArray, i * 16);
        }
        return _createTransform(transformArray, transforms.length, invariantBoundingSphere, cellSize, batchSize, transformData);
    }
    Shape.createTransform = createTransform;
    function createRenderObject(shape, props) {
        props;
        const theme = getTheme(shape);
        const utils = Geometry.getUtils(shape.geometry);
        const materialId = getNextMaterialId();
        const locationIt = groupIterator(shape);
        const transform = createTransform(shape.transforms, shape.geometry.boundingSphere, props.cellSize, props.batchSize);
        const values = utils.createValues(shape.geometry, transform, locationIt, theme, props);
        const state = utils.createRenderableState(props);
        return _createRenderObject(shape.geometry.kind, values, state, materialId);
    }
    Shape.createRenderObject = createRenderObject;
    function Loci(shape) { return { kind: 'shape-loci', shape }; }
    Shape.Loci = Loci;
    function isLoci(x) { return !!x && x.kind === 'shape-loci'; }
    Shape.isLoci = isLoci;
    function areLociEqual(a, b) { return a.shape === b.shape; }
    Shape.areLociEqual = areLociEqual;
    function isLociEmpty(loci) { return loci.shape.groupCount === 0; }
    Shape.isLociEmpty = isLociEmpty;
})(Shape || (Shape = {}));
export var ShapeGroup;
(function (ShapeGroup) {
    function Location(shape, group = 0, instance = 0) {
        return { kind: 'group-location', shape: shape, group, instance };
    }
    ShapeGroup.Location = Location;
    function isLocation(x) {
        return !!x && x.kind === 'group-location';
    }
    ShapeGroup.isLocation = isLocation;
    function Loci(shape, groups) {
        return { kind: 'group-loci', shape, groups: groups };
    }
    ShapeGroup.Loci = Loci;
    function isLoci(x) {
        return !!x && x.kind === 'group-loci';
    }
    ShapeGroup.isLoci = isLoci;
    function areLociEqual(a, b) {
        if (a.shape !== b.shape)
            return false;
        if (a.groups.length !== b.groups.length)
            return false;
        for (let i = 0, il = a.groups.length; i < il; ++i) {
            const { ids: idsA, instance: instanceA } = a.groups[i];
            const { ids: idsB, instance: instanceB } = b.groups[i];
            if (instanceA !== instanceB)
                return false;
            if (!OrderedSet.areEqual(idsA, idsB))
                return false;
        }
        return true;
    }
    ShapeGroup.areLociEqual = areLociEqual;
    function isLociEmpty(loci) {
        return size(loci) === 0 ? true : false;
    }
    ShapeGroup.isLociEmpty = isLociEmpty;
    function size(loci) {
        let size = 0;
        for (const group of loci.groups) {
            size += OrderedSet.size(group.ids);
        }
        return size;
    }
    ShapeGroup.size = size;
    const sphereHelper = new CentroidHelper(), tmpPos = Vec3.zero();
    function sphereHelperInclude(groups, mapping, positions, transforms) {
        const { indices, offsets } = mapping;
        for (const { ids, instance } of groups) {
            OrderedSet.forEach(ids, v => {
                for (let i = offsets[v], il = offsets[v + 1]; i < il; ++i) {
                    Vec3.fromArray(tmpPos, positions, indices[i] * 3);
                    Vec3.transformMat4(tmpPos, tmpPos, transforms[instance]);
                    sphereHelper.includeStep(tmpPos);
                }
            });
        }
    }
    function sphereHelperRadius(groups, mapping, positions, transforms) {
        const { indices, offsets } = mapping;
        for (const { ids, instance } of groups) {
            OrderedSet.forEach(ids, v => {
                for (let i = offsets[v], il = offsets[v + 1]; i < il; ++i) {
                    Vec3.fromArray(tmpPos, positions, indices[i] * 3);
                    Vec3.transformMat4(tmpPos, tmpPos, transforms[instance]);
                    sphereHelper.radiusStep(tmpPos);
                }
            });
        }
    }
    function getBoundingSphere(loci, boundingSphere) {
        if (!boundingSphere)
            boundingSphere = Sphere3D();
        sphereHelper.reset();
        let padding = 0;
        const { geometry, transforms } = loci.shape;
        if (geometry.kind === 'mesh' || geometry.kind === 'points') {
            const positions = geometry.kind === 'mesh'
                ? geometry.vertexBuffer.ref.value
                : geometry.centerBuffer.ref.value;
            sphereHelperInclude(loci.groups, geometry.groupMapping, positions, transforms);
            sphereHelper.finishedIncludeStep();
            sphereHelperRadius(loci.groups, geometry.groupMapping, positions, transforms);
        }
        else if (geometry.kind === 'lines') {
            const start = geometry.startBuffer.ref.value;
            const end = geometry.endBuffer.ref.value;
            sphereHelperInclude(loci.groups, geometry.groupMapping, start, transforms);
            sphereHelperInclude(loci.groups, geometry.groupMapping, end, transforms);
            sphereHelper.finishedIncludeStep();
            sphereHelperRadius(loci.groups, geometry.groupMapping, start, transforms);
            sphereHelperRadius(loci.groups, geometry.groupMapping, end, transforms);
        }
        else if (geometry.kind === 'spheres' || geometry.kind === 'text') {
            const positions = geometry.centerBuffer.ref.value;
            sphereHelperInclude(loci.groups, geometry.groupMapping, positions, transforms);
            sphereHelper.finishedIncludeStep();
            sphereHelperRadius(loci.groups, geometry.groupMapping, positions, transforms);
            for (const { ids, instance } of loci.groups) {
                OrderedSet.forEach(ids, v => {
                    const value = loci.shape.getSize(v, instance);
                    if (padding < value)
                        padding = value;
                });
            }
        }
        else {
            // use whole shape bounding-sphere for other geometry kinds
            return Sphere3D.copy(boundingSphere, geometry.boundingSphere);
        }
        Vec3.copy(boundingSphere.center, sphereHelper.center);
        boundingSphere.radius = Math.sqrt(sphereHelper.radiusSq);
        Sphere3D.expand(boundingSphere, boundingSphere, padding);
        return boundingSphere;
    }
    ShapeGroup.getBoundingSphere = getBoundingSphere;
})(ShapeGroup || (ShapeGroup = {}));
