"use strict";
/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.queryMVSRef = queryMVSRef;
exports.createMVSRefMap = createMVSRefMap;
exports.tryGetPrimitivesFromLoci = tryGetPrimitivesFromLoci;
exports.getCurrentMVSSnapshot = getCurrentMVSSnapshot;
const int_1 = require("../../mol-data/int.js");
const shape_1 = require("../../mol-model/shape.js");
const mol_state_1 = require("../../mol-state/index.js");
/**
 * Queries all MolViewSpec references in the current state of the plugin.
 */
function queryMVSRef(plugin, ref) {
    return plugin.state.data.selectQ(q => q.root.subtree().withTag(`mvs-ref:${ref}`));
}
/**
 * Creates a mapping of all MolViewSpec references in the current state of the plugin.
 */
function createMVSRefMap(plugin) {
    const tree = plugin.state.data.tree;
    const mapping = new Map();
    mol_state_1.StateTree.doPreOrder(tree, tree.root, { mapping, plugin }, (n, _, s) => {
        if (!n.tags)
            return;
        for (const tag of n.tags) {
            if (!tag.startsWith('mvs-ref:'))
                continue;
            const mvsRef = tag.substring(8);
            const selector = new mol_state_1.StateObjectSelector(n.ref, s.plugin.state.data);
            if (s.mapping.has(mvsRef))
                s.mapping.get(mvsRef).push(selector);
            else
                s.mapping.set(mvsRef, [selector]);
        }
    });
    return mapping;
}
function tryGetPrimitivesFromLoci(loci) {
    if (!shape_1.ShapeGroup.isLoci(loci))
        return undefined;
    const srcData = loci.shape.sourceData;
    if ((srcData === null || srcData === void 0 ? void 0 : srcData.kind) !== 'mvs-primitives')
        return undefined;
    const nodes = [];
    for (const group of loci.groups) {
        int_1.OrderedSet.forEach(group.ids, id => {
            const node = srcData.groupToNode.get(id);
            if (node)
                nodes.push(node);
        });
    }
    return nodes.length > 0 ? nodes : undefined;
}
// Retrieves the MVS snapshot associated with the current snapshot of the plugin
// This will only work if the current state was created from an MVS snapshot
function getCurrentMVSSnapshot(plugin) {
    var _a, _b;
    return (_b = (_a = plugin.managers.snapshot.current) === null || _a === void 0 ? void 0 : _a._transientData) === null || _b === void 0 ? void 0 : _b.sourceMvsSnapshot;
}
