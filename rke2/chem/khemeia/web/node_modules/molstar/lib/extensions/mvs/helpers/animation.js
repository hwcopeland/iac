/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Ludovic Autin <ludovic.autin@gmail.com>
 */
import { SortedArray } from '../../../mol-data/int.js';
import * as EasingFns from '../../../mol-math/easing.js';
import { clamp, lerp } from '../../../mol-math/interpolate.js';
import { EPSILON, Mat3, Mat4, Quat, Vec3 } from '../../../mol-math/linear-algebra.js';
import { deepEqual } from '../../../mol-util/index.js';
import { Color } from '../../../mol-util/color/index.js';
import { decodeColor } from '../../../mol-util/color/utils.js';
import { produce } from '../../../mol-util/produce.js';
import { makeContinuousPaletteCheckpoints } from '../components/annotation-color-theme.js';
import { palettePropsFromMVSPalette } from '../load-helpers.js';
import { MVSAnimationSchema } from '../tree/animation/animation-tree.js';
import { addDefaults } from '../tree/generic/tree-utils.js';
export async function generateStateTransition(ctx, snapshot, snapshotIndex, snapshotCount) {
    var _a, _b, _c, _d, _e;
    if (!snapshot.animation)
        return undefined;
    const tree = addDefaults(snapshot.animation, MVSAnimationSchema);
    const transitions = (_a = tree.children) === null || _a === void 0 ? void 0 : _a.filter(child => child.kind === 'interpolate');
    if (!(transitions === null || transitions === void 0 ? void 0 : transitions.length))
        return undefined;
    const duration = Math.max((_c = (_b = snapshot.animation.params) === null || _b === void 0 ? void 0 : _b.duration_ms) !== null && _c !== void 0 ? _c : 0, ...transitions.map(t => { var _a; return ((_a = t.params.start_ms) !== null && _a !== void 0 ? _a : 0) + t.params.duration_ms; }));
    const frames = [];
    const dt = (_e = (_d = tree.params) === null || _d === void 0 ? void 0 : _d.frame_time_ms) !== null && _e !== void 0 ? _e : (1000 / 60);
    const N = Math.ceil(duration / dt);
    const nodeMap = makeNodeMap(snapshot.root, new Map(), []);
    const cache = new Map();
    const transitionGroups = groupTranstions(transitions);
    let prevRoot;
    for (let i = 0; i <= N; i++) {
        const t = i * dt;
        const root = createSnapshot(snapshot.root, transitionGroups, t, cache, nodeMap);
        if (root === prevRoot || (prevRoot && deepEqual(root, prevRoot))) {
            frames[frames.length - 1][1] += dt;
        }
        else {
            frames.push([root, dt]);
        }
        prevRoot = root;
        if (ctx.shouldUpdate) {
            await ctx.update({ message: `Generating transition for snapshot ${snapshotIndex + 1}/${snapshotCount}`, current: i + 1, max: N });
        }
    }
    return { tree, frametimeMs: dt, frames };
}
const EasingFnMap = {
    'linear': t => t,
    'bounce-in': EasingFns.bounceIn,
    'bounce-out': EasingFns.bounceOut,
    'bounce-in-out': EasingFns.bounceInOut,
    'circle-in': EasingFns.circleIn,
    'circle-out': EasingFns.circleOut,
    'circle-in-out': EasingFns.circleInOut,
    'cubic-in': EasingFns.cubicIn,
    'cubic-out': EasingFns.cubicOut,
    'cubic-in-out': EasingFns.cubicInOut,
    'exp-in': EasingFns.expIn,
    'exp-out': EasingFns.expOut,
    'exp-in-out': EasingFns.expInOut,
    'quad-in': EasingFns.quadIn,
    'quad-out': EasingFns.quadOut,
    'quad-in-out': EasingFns.quadInOut,
    'sin-in': EasingFns.sinIn,
    'sin-out': EasingFns.sinOut,
    'sin-in-out': EasingFns.sinInOut,
};
function getTransitionKey(transition) {
    const prop = transition.params.property;
    if (Array.isArray(prop)) {
        return `${transition.params.target_ref}:${prop.join('.')}`;
    }
    return `${transition.params.target_ref}:${prop}`;
}
function groupTranstions(transitions) {
    const map = new Map();
    const groups = [];
    for (const t of transitions) {
        const key = getTransitionKey(t);
        if (!map.has(key)) {
            const group = [];
            map.set(key, group);
            groups.push(group);
        }
        map.get(key).push(t);
    }
    for (const group of groups) {
        group.sort((a, b) => {
            var _a, _b;
            const s = ((_a = a.params.start_ms) !== null && _a !== void 0 ? _a : 0) - ((_b = b.params.start_ms) !== null && _b !== void 0 ? _b : 0);
            if (s !== 0)
                return s;
            return a.params.duration_ms - b.params.duration_ms;
        });
    }
    return groups;
}
function createSnapshot(tree, transitionGroups, time, cache, nodeMap) {
    let modified = false;
    const ret = produce(tree, (draft) => {
        var _a, _b, _c;
        for (const transitionGroup of transitionGroups) {
            const pivot = transitionGroup[0];
            const nodePath = nodeMap.get(pivot.params.target_ref);
            if (!nodePath)
                continue;
            const node = select(draft, nodePath, 0);
            const target = pivot.params.property[0] === 'custom' ? node === null || node === void 0 ? void 0 : node.custom : node === null || node === void 0 ? void 0 : node.params;
            if (!target)
                continue;
            const offset = pivot.params.property[0] === 'custom' ? 1 : 0;
            let transition = pivot;
            let previous;
            for (let i = transitionGroup.length - 1; i > 0; i--) {
                const current = transitionGroup[i];
                const currentStart = (_a = current.params.start_ms) !== null && _a !== void 0 ? _a : 0;
                if (time >= currentStart) {
                    transition = current;
                    previous = i > 0 ? transitionGroup[i - 1] : undefined;
                    break;
                }
            }
            if (!cache.has(transition)) {
                cache.set(transition, {});
            }
            const cacheEntry = cache.get(transition);
            const startTime = (_b = transition.params.start_ms) !== null && _b !== void 0 ? _b : 0;
            const durationMs = (_c = transition.params.duration_ms) !== null && _c !== void 0 ? _c : 0;
            const t = (time - startTime) / durationMs;
            let next;
            if (transition.params.kind === 'transform_matrix') {
                next = processTransformMatrix(transition, target, clamp(t, 0, 1), cacheEntry, offset, previous);
            }
            else {
                next = processScalarLike(transition, target, t, cacheEntry, offset, previous);
            }
            if (next === undefined) {
                continue;
            }
            modified = true;
            assign(target, transition.params.property, next, offset);
        }
    });
    return modified ? ret : tree;
}
function applyFrequency(t, frequency, alternate) {
    let v = (t * (frequency || 1));
    if (v < 1)
        return v;
    if (!alternate) {
        v = (v % 1);
        if (v === 0)
            return 1;
        return v;
    }
    if (Math.abs(v - 1) < EPSILON)
        return 1;
    v = v % 2;
    if (v > 1)
        return 2 - v;
    return v;
}
function getPreviousScalarEnd(previous) {
    if (!previous || previous.params.kind === 'transform_matrix')
        return undefined;
    return previous.params.end;
}
function processScalarLike(transition, target, time, cacheEntry, offset, previous) {
    var _a, _b, _c, _d, _e, _f, _g, _h;
    if (transition.params.kind === 'transform_matrix')
        return;
    if (previous && previous.params.kind === 'transform_matrix')
        return;
    const startValue = (_b = (_a = transition.params.start) !== null && _a !== void 0 ? _a : getPreviousScalarEnd(previous)) !== null && _b !== void 0 ? _b : select(target, transition.params.property, offset);
    if (transition.params.kind === 'color' && !cacheEntry.paletteFn) {
        cacheEntry.paletteFn = makePaletteFunction(transition);
    }
    const endValue = transition.params.end;
    if (time <= 0)
        return startValue;
    else if (time >= 1 - EPSILON && !transition.params.alternate_direction && transition.params.kind !== 'color')
        return endValue;
    let t = clamp(time, 0, 1);
    t = applyFrequency(t, (_c = transition.params.frequency) !== null && _c !== void 0 ? _c : 1, !!transition.params.alternate_direction);
    const easing = (_e = EasingFnMap[(_d = transition.params.easing) !== null && _d !== void 0 ? _d : 'linear']) !== null && _e !== void 0 ? _e : EasingFnMap['linear'];
    t = easing(t);
    if (transition.params.kind === 'scalar') {
        return interpolateScalars(startValue, endValue, t, (_f = transition.params.noise_magnitude) !== null && _f !== void 0 ? _f : 0, !!transition.params.discrete);
    }
    else if (transition.params.kind === 'vec3') {
        return interpolateVectors(startValue, endValue, t, (_g = transition.params.noise_magnitude) !== null && _g !== void 0 ? _g : 0, !!transition.params.spherical);
    }
    else if (transition.params.kind === 'rotation_matrix') {
        return interpolateRotation(startValue, endValue, t, (_h = transition.params.noise_magnitude) !== null && _h !== void 0 ? _h : 0, cacheEntry);
    }
    else if (transition.params.kind === 'color') {
        if (cacheEntry.paletteFn) {
            const color = cacheEntry.paletteFn(t);
            return Color.toHexStyle(color);
        }
        const baseColors = typeof startValue === 'object' ? select(target, transition.params.property, offset) : undefined;
        return interpolateColors(startValue, endValue, t, cacheEntry, baseColors);
    }
}
function getPreviousMatrixEnd(previous, prop) {
    if (!previous || previous.params.kind !== 'transform_matrix')
        return undefined;
    return previous.params[prop];
}
const TransformState = {
    pivotTranslation: Mat4(),
    pivotTranslationInv: Mat4(),
    rotation: Mat4(),
    scale: Mat4(),
    translation: Mat4(),
    pivotNeg: Vec3(),
    temp: Mat4(),
};
function processTransformMatrix(transition, target, time, cache, offset, previous) {
    var _a, _b, _c, _d, _e, _f, _g, _h, _j, _k, _l, _m, _o, _p, _q, _r, _s, _t, _u, _v;
    if (transition.params.kind !== 'transform_matrix')
        return;
    if (previous && previous.params.kind !== 'transform_matrix')
        return;
    const transform = (_a = select(target, transition.params.property, offset)) !== null && _a !== void 0 ? _a : Mat4.identity();
    const startRotation = (_c = (_b = transition.params.rotation_start) !== null && _b !== void 0 ? _b : getPreviousMatrixEnd(previous, 'rotation_start')) !== null && _c !== void 0 ? _c : Mat3.fromMat4(Mat3(), transform);
    const startTranslation = (_e = (_d = transition.params.translation_start) !== null && _d !== void 0 ? _d : getPreviousMatrixEnd(previous, 'translation_start')) !== null && _e !== void 0 ? _e : Mat4.getTranslation(Vec3(), transform);
    const startScale = (_g = (_f = transition.params.scale_start) !== null && _f !== void 0 ? _f : getPreviousMatrixEnd(previous, 'scale_start')) !== null && _g !== void 0 ? _g : Mat4.getScaling(Vec3(), transform);
    const endRotation = transition.params.rotation_end;
    const endTranslation = transition.params.translation_end;
    const endScale = transition.params.scale_end;
    let rotation, translation, scale;
    if (time <= 0) {
        rotation = startRotation;
        translation = startTranslation;
        scale = startScale;
    }
    else {
        const clampedTime = clamp(time, 0, 1);
        let t = applyFrequency(clampedTime, (_h = transition.params.rotation_frequency) !== null && _h !== void 0 ? _h : 1, !!transition.params.rotation_alternate_direction);
        let easing = (_k = EasingFnMap[(_j = transition.params.rotation_easing) !== null && _j !== void 0 ? _j : 'linear']) !== null && _k !== void 0 ? _k : EasingFnMap['linear'];
        rotation = interpolateRotation(startRotation, endRotation, easing(t), (_l = transition.params.rotation_noise_magnitude) !== null && _l !== void 0 ? _l : 0, cache);
        t = applyFrequency(clampedTime, (_m = transition.params.translation_frequency) !== null && _m !== void 0 ? _m : 1, !!transition.params.translation_alternate_direction);
        easing = (_p = EasingFnMap[(_o = transition.params.translation_easing) !== null && _o !== void 0 ? _o : 'linear']) !== null && _p !== void 0 ? _p : EasingFnMap['linear'];
        translation = interpolateVec3(startTranslation, endTranslation, easing(t), (_q = transition.params.translation_noise_magnitude) !== null && _q !== void 0 ? _q : 0, false);
        t = applyFrequency(clampedTime, (_r = transition.params.scale_frequency) !== null && _r !== void 0 ? _r : 1, !!transition.params.scale_alternate_direction);
        easing = (_t = EasingFnMap[(_s = transition.params.scale_easing) !== null && _s !== void 0 ? _s : 'linear']) !== null && _t !== void 0 ? _t : EasingFnMap['linear'];
        scale = interpolateVec3(startScale, endScale, easing(t), (_u = transition.params.scale_noise_magnitude) !== null && _u !== void 0 ? _u : 0, false);
    }
    const pivot = (_v = transition.params.pivot) !== null && _v !== void 0 ? _v : Vec3.zero();
    Mat4.fromTranslation(TransformState.translation, translation);
    Mat4.fromScaling(TransformState.scale, scale);
    Mat4.setIdentity(TransformState.rotation);
    Mat4.fromMat3(TransformState.rotation, rotation);
    Mat4.fromTranslation(TransformState.pivotTranslation, pivot);
    Mat4.fromTranslation(TransformState.pivotTranslationInv, Vec3.negate(TransformState.pivotNeg, pivot));
    // translation . pivot . rotation . scale . pivotInv
    const result = Mat4();
    Mat4.mul(result, TransformState.scale, TransformState.pivotTranslationInv);
    Mat4.mul(result, TransformState.rotation, result);
    Mat4.mul(result, TransformState.translation, result);
    return result;
}
function interpolateScalars(start, end, t, noise, discrete) {
    if (Array.isArray(start)) {
        const ret = Array.from({ length: start.length }).fill(0.1);
        if (!end || !Array.isArray(end)) {
            for (let i = 0; i < start.length; i++) {
                ret[i] = interpolateScalar(start[i], end, t, noise, discrete);
            }
            return ret;
        }
        for (let i = 0; i < start.length; i++) {
            ret[i] = interpolateScalar(start[i], end[i], t, noise, discrete);
        }
        return ret;
    }
    if (Array.isArray(end)) {
        const ret = Array.from({ length: end.length }).fill(0.1);
        for (let i = 0; i < end.length; i++) {
            ret[i] = interpolateScalar(start, end[i], t, noise, discrete);
        }
        return ret;
    }
    return interpolateScalar(start, end, t, noise, discrete);
}
function interpolateScalar(start, end, t, noise, discrete) {
    let v = typeof end === 'number' ? lerp(start, end, t) : start;
    if (noise) {
        v += (Math.random() - 0.5) * noise;
    }
    if (discrete) {
        v = Math.round(v);
    }
    return v;
}
const InterpolateVectorsState = {
    start: Vec3(),
    end: Vec3(),
    v: Vec3(),
};
function interpolateVectors(start, end, t, noise, isSpherical) {
    if ((!end || start === end) && !noise)
        return start;
    const ret = Array.from({ length: start.length }).fill(0.1);
    for (let i = 0; i < start.length; i += 3) {
        const s = Vec3.fromArray(InterpolateVectorsState.start, start, i);
        let v;
        if (end) {
            const e = Vec3.fromArray(InterpolateVectorsState.end, end, i);
            v = isSpherical
                ? Vec3.slerp(InterpolateVectorsState.v, s, e, t)
                : Vec3.lerp(InterpolateVectorsState.v, s, e, t);
        }
        else {
            v = Vec3.clone(s);
        }
        if (noise && t <= 1 - EPSILON) {
            Vec3.random(Vec3Noise, noise);
            Vec3.add(v, v, Vec3Noise);
        }
        Vec3.toArray(v, ret, i);
    }
    return ret;
}
const Vec3Noise = Vec3();
function interpolateVec3(start, end, t, noise, isSpherical) {
    if ((!end || start === end) && !noise)
        return start;
    let v;
    if (end) {
        v = isSpherical
            ? Vec3.slerp(Vec3(), start, end, t)
            : Vec3.lerp(Vec3(), start, end, t);
    }
    else {
        v = Vec3.clone(start);
    }
    if (noise && t <= 1 - EPSILON) {
        Vec3.random(Vec3Noise, noise);
        Vec3.add(v, v, Vec3Noise);
    }
    return v;
}
const RotationState = {
    start: Quat(),
    end: Quat(),
    v: Quat(),
    noise: Quat(),
    axis: Vec3(),
    temp: Mat4(),
};
function interpolateRotation(start, end, t, noise, cache) {
    if ((!end || start === end) && !noise)
        return start;
    if (end) {
        if (!cache.rotation) {
            cache.rotation = {
                ...relativeAxisAngle(start, end),
                start: Quat.fromMat3(Quat(), start),
                end: Quat.fromMat3(Quat(), end),
            };
        }
        const { axis, angle, start: startQ, end: endQ } = cache.rotation;
        if (angle < 1e-6) {
            // start ≈ end: make a clean spin about the detected (or default) axis
            Quat.setAxisAngle(RotationState.v, axis, t * 2 * Math.PI); // Make a full turn
        }
        else {
            // Normal case: stick with your existing slerp between start/end
            Quat.slerp(RotationState.v, startQ, endQ, t);
        }
    }
    else {
        Quat.fromMat3(RotationState.v, start);
    }
    if (noise && t <= 1 - EPSILON) {
        Vec3.random(RotationState.axis, 1);
        Quat.setAxisAngle(RotationState.noise, RotationState.axis, 2 * Math.PI * noise * (Math.random() - 0.5));
        Quat.multiply(RotationState.v, RotationState.noise, RotationState.v);
    }
    Mat4.fromQuat(RotationState.temp, RotationState.v);
    return Mat3.fromMat4(Mat3(), RotationState.temp);
}
function decodeColors(color, baseColors) {
    if (color === undefined || color === null)
        return undefined;
    if (typeof color === 'object') {
        const ret = {};
        if (baseColors) {
            for (const key of Object.keys(baseColors)) {
                const decoded = decodeColor(baseColors[key]);
                if (decoded !== undefined) {
                    ret[key] = decoded;
                }
            }
        }
        for (const key of Object.keys(color)) {
            const decoded = decodeColor(color[key]);
            if (decoded !== undefined) {
                ret[key] = decoded;
            }
        }
        return ret;
    }
    return decodeColor(color);
}
function interpolateColors(start, end, time, cacheEntry, baseColors) {
    const t = clamp(time, 0, 1);
    if (cacheEntry.paletteFn) {
        const c = cacheEntry.paletteFn(t);
        return Color.toHexStyle(c);
    }
    if (cacheEntry.startColor === undefined) {
        cacheEntry.startColor = decodeColors(start, baseColors);
    }
    if (cacheEntry.endColor === undefined) {
        cacheEntry.endColor = decodeColors(end, undefined);
    }
    const { startColor, endColor } = cacheEntry;
    if (typeof startColor === 'object') {
        if (typeof baseColors !== 'object') {
            throw new Error('Cannot interpolate from scalar color to color mapping');
        }
        const ret = { ...baseColors, ...startColor };
        if (typeof endColor === 'object') {
            for (const key of Object.keys(endColor)) {
                ret[key] = Color.toHexStyle(Color.interpolate(startColor[key], endColor[key], t));
            }
        }
        else if (typeof endColor === 'number') {
            for (const key of Object.keys(startColor)) {
                ret[key] = Color.toHexStyle(Color.interpolate(startColor[key], endColor, t));
            }
        }
        return ret;
    }
    if (typeof endColor === 'object') {
        throw new Error('Cannot interpolate from scalar color to color mapping');
    }
    if (typeof endColor === 'number' && typeof startColor === 'number') {
        return Color.toHexStyle(Color.interpolate(startColor, endColor, t));
    }
    return start;
}
function select(params, path, offset) {
    if (typeof path === 'string') {
        return params === null || params === void 0 ? void 0 : params[path];
    }
    let f = params;
    for (let i = offset; i < path.length; i++) {
        if (!f)
            break;
        f = f[path[i]];
    }
    return f;
}
function assign(params, path, value, offset) {
    if (!params)
        return;
    if (typeof path === 'string') {
        params[path] = value;
        return;
    }
    let f = params;
    for (let i = offset; i < path.length; i++) {
        if (!f)
            break;
        if (i === path.length - 1) {
            f[path[i]] = value;
        }
        else {
            f = f[path[i]];
        }
    }
}
function makeNodeMap(tree, map, currentPath) {
    if (tree.ref) {
        map.set(tree.ref, [...currentPath]);
    }
    if (!tree.children)
        return map;
    currentPath.push('children');
    for (let i = 0; i < tree.children.length; i++) {
        const child = tree.children[i];
        currentPath.push(i);
        makeNodeMap(child, map, currentPath);
        currentPath.pop();
    }
    currentPath.pop();
    return map;
}
function makePaletteFunction(props) {
    if (props.params.kind !== 'color' || !props.params.palette)
        return undefined;
    const params = palettePropsFromMVSPalette(props.params.palette);
    if (params.name === 'discrete')
        return makePaletteFunctionDiscrete(params.params);
    if (params.name === 'continuous')
        return makePaletteFunctionContinuous(params.params);
    throw new Error(`NotImplementedError: makePaletteFunction for ${props.name}`);
}
function makePaletteFunctionDiscrete(props) {
    const defaultColor = Color(0x0);
    if (props.colors.length === 0)
        return () => defaultColor;
    return (value) => {
        const x = clamp(value, 0, 1);
        for (let i = props.colors.length - 1; i >= 0; i--) {
            const { color, fromValue, toValue } = props.colors[i];
            if (fromValue <= x && x <= toValue)
                return color;
        }
        return defaultColor;
    };
}
function makePaletteFunctionContinuous(props) {
    const defaultColor = Color(0x0);
    const { colors, checkpoints } = makeContinuousPaletteCheckpoints(props);
    if (colors.length === 0)
        return () => defaultColor;
    const underflowColor = props.setUnderflowColor ? props.underflowColor : defaultColor;
    const overflowColor = props.setOverflowColor ? props.overflowColor : defaultColor;
    return (value) => {
        const x = clamp(value, 0, 1);
        const gteIdx = SortedArray.findPredecessorIndex(checkpoints, x); // Index of the first greater or equal checkpoint
        if (gteIdx === 0) {
            if (x === checkpoints[0])
                return colors[0];
            else
                return underflowColor;
        }
        if (gteIdx === checkpoints.length) {
            return overflowColor;
        }
        const q = (x - checkpoints[gteIdx - 1]) / (checkpoints[gteIdx] - checkpoints[gteIdx - 1]);
        return Color.interpolate(colors[gteIdx - 1], colors[gteIdx], q);
    };
}
const RelativeAxisAngleState = {
    Rt: Mat3(),
    R: Mat3(),
};
function relativeAxisAngle(start, end) {
    // R_rel = end * start^T
    const R0 = start, R1 = end;
    const Rt = Mat3.transpose(RelativeAxisAngleState.Rt, R0);
    const R = Mat3.mul(RelativeAxisAngleState.R, R1, Rt);
    const tr = R[0] + R[4] + R[8]; // trace
    let angle = Math.acos(clamp((tr - 1) * 0.5, -1, 1)); // in [0, π]
    const axis = Vec3();
    const eps = 1e-6;
    const sinA = Math.sin(angle);
    if (angle < eps) {
        // Near identity: axis undefined; return any unit axis (choose something stable)
        Vec3.set(axis, 0, 0, 1);
        angle = 0.0;
        return { axis, angle };
    }
    if (Math.PI - angle > 1e-4) {
        // General case
        axis[0] = (R[5] - R[7]) / (2 * sinA); // (r32 - r23)
        axis[1] = (R[6] - R[2]) / (2 * sinA); // (r13 - r31)
        axis[2] = (R[1] - R[3]) / (2 * sinA); // (r21 - r12)
        Vec3.normalize(axis, axis);
        return { axis, angle };
    }
    // angle ~ π: use diagonal-based extraction for stability
    // Compute squared components then pick the largest to avoid precision loss
    const xx = Math.max(0, (R[0] + 1) * 0.5);
    const yy = Math.max(0, (R[4] + 1) * 0.5);
    const zz = Math.max(0, (R[8] + 1) * 0.5);
    let x = Math.sqrt(xx), y = Math.sqrt(yy), z = Math.sqrt(zz);
    if (x >= y && x >= z) {
        x = Math.max(x, 1e-8);
        y = (R[1] + R[3]) / (4 * x);
        z = (R[2] + R[6]) / (4 * x);
        Vec3.set(axis, x, y, z);
    }
    else if (y >= x && y >= z) {
        y = Math.max(y, 1e-8);
        x = (R[1] + R[3]) / (4 * y);
        z = (R[5] + R[7]) / (4 * y);
        Vec3.set(axis, x, y, z);
    }
    else {
        z = Math.max(z, 1e-8);
        x = (R[2] + R[6]) / (4 * z);
        y = (R[5] + R[7]) / (4 * z);
        Vec3.set(axis, x, y, z);
    }
    Vec3.normalize(axis, axis);
    return { axis, angle: Math.PI };
}
