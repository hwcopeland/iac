/**
 * Copyright (c) 2022-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { StateObjectRef } from '../../../../mol-state/index.js';
import { StructureFromPetworld } from './model.js';
import { Color } from '../../../../mol-util/color/index.js';
import { SpacefillRepresentationProvider } from '../../../../mol-repr/structure/representation/spacefill.js';
import { StructureRepresentation3D } from '../../../../mol-plugin-state/transforms/representation.js';
import { MesoscaleGroup, MesoscaleState, getDistinctBaseColors, getGraphicsModeProps, getMesoscaleGroupParams } from '../state.js';
import { ColorNames } from '../../../../mol-util/color/names.js';
import { MmcifFormat } from '../../../../mol-model-formats/structure/mmcif.js';
import { Task } from '../../../../mol-task/index.js';
function getSpacefillParams(color, graphics) {
    const gmp = getGraphicsModeProps(graphics === 'custom' ? 'quality' : graphics);
    return {
        type: {
            name: 'spacefill',
            params: {
                ...SpacefillRepresentationProvider.defaultValues,
                ignoreHydrogens: true,
                instanceGranularity: true,
                ignoreLight: true,
                lodLevels: gmp.lodLevels,
                quality: 'lowest', // avoid 'auto', triggers boundary calc
                clip: {
                    variant: 'instance',
                    objects: [],
                },
                clipPrimitive: true,
                approximate: gmp.approximate,
                alphaThickness: gmp.alphaThickness,
                interior: {
                    color: Color.fromNormalizedRgb(0, 0, 0),
                    colorStrength: 0.15,
                    substance: { metalness: 0.0, roughness: 1.0, bumpiness: 0.0 },
                    substanceStrength: 1,
                }
            },
        },
        colorTheme: {
            name: 'uniform',
            params: {
                value: color,
                saturation: 0,
                lightness: 0,
            }
        },
        sizeTheme: {
            name: 'physical',
            params: {
                scale: 1,
            }
        },
    };
}
export async function createPetworldHierarchy(plugin, trajectory) {
    var _a;
    const cell = StateObjectRef.resolveAndCheck(plugin.state.data, trajectory);
    const tr = (_a = cell === null || cell === void 0 ? void 0 : cell.obj) === null || _a === void 0 ? void 0 : _a.data;
    if (!cell || !tr)
        return;
    if (!MmcifFormat.is(tr.representative.sourceData))
        return;
    const membrane = [];
    const other = [];
    for (let i = 0; i < tr.frameCount; ++i) {
        const m = await Task.resolveInContext(tr.getFrameAtIndex(i));
        // cannot use m.properties.structAsymMap because petworld models
        // may assign the same asymId to multiple entities
        const { label_asym_id, label_entity_id, _rowCount } = m.atomicHierarchy.chains;
        const membraneIds = [];
        const otherIds = [];
        const seen = new Set();
        for (let i = 0; i < _rowCount; i++) {
            const entityId = label_entity_id.value(i);
            if (seen.has(entityId))
                continue;
            const asymId = label_asym_id.value(i);
            if (asymId.startsWith('MEM')) {
                membraneIds.push(entityId);
            }
            else {
                otherIds.push(entityId);
            }
            seen.add(entityId);
        }
        if (membraneIds.length) {
            membrane.push({ modelIndex: i, entityIds: membraneIds });
        }
        if (otherIds.length) {
            other.push({ modelIndex: i, entityIds: otherIds });
        }
    }
    const state = plugin.state.data;
    const graphicsMode = MesoscaleState.get(plugin).graphics;
    const groupParams = getMesoscaleGroupParams(graphicsMode);
    const group = await state.build()
        .toRoot()
        .apply(MesoscaleGroup, { ...groupParams, root: true, index: -1, tag: `ent:`, label: 'entity', color: { type: 'generate', illustrative: false, value: ColorNames.white, variability: 20, shift: 0, lightness: 0, alpha: 1, emissive: 0 } }, { tags: ['group:ent:'], state: { isCollapsed: false, isHidden: groupParams.hidden } })
        .commit({ revertOnError: true });
    await state.build()
        .to(group)
        .apply(MesoscaleGroup, { ...groupParams, index: undefined, tag: `ent:mem`, label: 'Membrane', color: { type: 'uniform', illustrative: false, value: ColorNames.lightgrey, variability: 20, shift: 0, lightness: 0, alpha: 1, emissive: 0 } }, { tags: ['group:ent:mem', 'ent:', '__no_group_color__'], state: { isCollapsed: true, isHidden: groupParams.hidden } })
        .commit();
    const colors = getDistinctBaseColors(other.length, 0);
    await state.transaction(async () => {
        try {
            plugin.animationLoop.stop({ noDraw: true });
            let build = state.build();
            for (let i = 0, il = membrane.length; i < il; ++i) {
                build = build
                    .to(cell)
                    .apply(StructureFromPetworld, membrane[i])
                    .apply(StructureRepresentation3D, getSpacefillParams(ColorNames.lightgrey, graphicsMode), { tags: ['ent:mem', '__no_group_color__'] });
            }
            for (let i = 0, il = other.length; i < il; ++i) {
                build = build
                    .to(cell)
                    .apply(StructureFromPetworld, other[i])
                    .apply(StructureRepresentation3D, getSpacefillParams(colors[i], graphicsMode), { tags: ['ent:'] });
            }
            await build.commit();
        }
        catch (e) {
            console.error(e);
            plugin.log.error(e);
        }
        finally {
            plugin.animationLoop.start();
        }
    }).run();
}
