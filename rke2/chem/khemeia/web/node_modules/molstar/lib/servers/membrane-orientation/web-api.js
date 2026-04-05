/**
 * Copyright (c) 2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Sebastian Bittrich <sebastian.bittrich@rcsb.org>
 */
import { MembraneServerConfig } from './config.js';
import { swaggerUiAssetsHandler, swaggerUiIndexHandler } from '../common/swagger-ui/index.js';
import { getSchema, shortcutIconLink } from './web-schema.js';
import { MembraneOrientationProvider } from '../../extensions/anvil/prop.js';
import { SyncRuntimeContext } from '../../mol-task/execution/synchronous.js';
import { AssetManager } from '../../mol-util/assets.js';
import { CIF } from '../../mol-io/reader/cif.js';
import { Structure, StructureSymmetry } from '../../mol-model/structure.js';
import { trajectoryFromMmCIF } from '../../mol-model-formats/structure/mmcif.js';
import { ANVILParams } from '../../extensions/anvil/algorithm.js';
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { ConsoleLogger } from '../../mol-util/console-logger.js';
const assetManager = new AssetManager();
export function initWebApi(app) {
    function makePath(p) {
        return MembraneServerConfig.apiPrefix + '/' + p;
    }
    app.get(makePath('predict/:id/'), async (req, res) => predictMembraneOrientation(req, res));
    app.get(makePath('openapi.json'), (_, res) => {
        res.writeHead(200, {
            'Content-Type': 'application/json; charset=utf-8',
            'Access-Control-Allow-Origin': '*',
            'Access-Control-Allow-Headers': 'X-Requested-With'
        });
        res.end(JSON.stringify(getSchema()));
    });
    app.use(makePath(''), swaggerUiAssetsHandler());
    app.get(makePath(''), swaggerUiIndexHandler({
        openapiJsonUrl: makePath('openapi.json'),
        apiPrefix: MembraneServerConfig.apiPrefix,
        title: 'MembraneServer API',
        shortcutIconLink
    }));
}
async function predictMembraneOrientation(req, res) {
    var _a;
    try {
        const ctx = { runtime: SyncRuntimeContext, assetManager };
        const entryId = req.params.id;
        const assemblyId = (_a = req.query.assemblyId) !== null && _a !== void 0 ? _a : '1';
        const p = parseParams(req);
        ConsoleLogger.log('predictMembraneOrientation', `${entryId}-${assemblyId} with params: ${JSON.stringify(p)}`);
        const cif = await downloadFromPdb(entryId);
        const models = await getModels(cif);
        const structure = await getStructure(models.representative, assemblyId);
        await MembraneOrientationProvider.attach(ctx, structure, p);
        const data = MembraneOrientationProvider.get(structure).value;
        res.status(200).json(data);
    }
    catch (e) {
        const error = 'Failed to compute membrane orientation';
        ConsoleLogger.error(error, e);
        res.status(500).json({ error });
    }
}
const defaults = PD.getDefaultValues(ANVILParams);
function parseParams(req) {
    const { numberOfSpherePoints = defaults.numberOfSpherePoints, stepSize = defaults.stepSize, minThickness = defaults.minThickness, maxThickness = defaults.maxThickness, asaCutoff = defaults.asaCutoff, adjust = defaults.adjust, tmdetDefinition = defaults.tmdetDefinition, } = req.query;
    return {
        numberOfSpherePoints: Number(numberOfSpherePoints),
        stepSize: Number(stepSize),
        minThickness: Number(minThickness),
        maxThickness: Number(maxThickness),
        asaCutoff: Number(asaCutoff),
        adjust: Number(adjust),
        tmdetDefinition: tmdetDefinition === 'true',
    };
}
async function parseCif(data) {
    const comp = CIF.parse(data);
    const parsed = await comp.run();
    if (parsed.isError)
        throw parsed;
    return parsed.result;
}
async function downloadCif(url, isBinary) {
    const data = await fetch(url);
    return parseCif(isBinary ? new Uint8Array(await data.arrayBuffer()) : await data.text());
}
async function downloadFromPdb(pdb) {
    const parsed = await downloadCif(MembraneServerConfig.bcifSource(pdb), true);
    return parsed.blocks[0];
}
async function getModels(frame) {
    return await trajectoryFromMmCIF(frame).run();
}
async function getStructure(model, assemblyId) {
    const modelStructure = Structure.ofModel(model);
    return await StructureSymmetry.buildAssembly(modelStructure, assemblyId).run();
}
