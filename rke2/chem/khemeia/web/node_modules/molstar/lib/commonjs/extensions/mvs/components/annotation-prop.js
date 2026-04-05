"use strict";
/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.MVSAnnotation = exports.MVSAnnotations = exports.MVSAnnotationsProvider = exports.MVSAnnotationsParams = void 0;
exports.getMVSAnnotationForStructure = getMVSAnnotationForStructure;
const db_1 = require("../../../mol-data/db.js");
const cif_1 = require("../../../mol-io/reader/cif.js");
const schema_1 = require("../../../mol-io/reader/cif/schema.js");
const mmcif_1 = require("../../../mol-model-formats/structure/mmcif.js");
const custom_model_property_1 = require("../../../mol-model-props/common/custom-model-property.js");
const custom_property_1 = require("../../../mol-model/custom-property.js");
const structure_1 = require("../../../mol-model/structure/structure.js");
const assets_1 = require("../../../mol-util/assets.js");
const json_1 = require("../../../mol-util/json.js");
const object_1 = require("../../../mol-util/object.js");
const param_choice_1 = require("../../../mol-util/param-choice.js");
const param_definition_1 = require("../../../mol-util/param-definition.js");
const element_ranges_1 = require("../helpers/element-ranges.js");
const indexing_1 = require("../helpers/indexing.js");
const param_definition_2 = require("../helpers/param-definition.js");
const schemas_1 = require("../helpers/schemas.js");
const selections_1 = require("../helpers/selections.js");
const utils_1 = require("../helpers/utils.js");
/** Allowed values for the annotation format parameter */
const MVSAnnotationFormat = new param_choice_1.Choice({ json: 'json', cif: 'cif', bcif: 'bcif' }, 'json');
const MVSAnnotationFormatTypes = { json: 'string', cif: 'string', bcif: 'binary' };
exports.MVSAnnotationsParams = {
    annotations: param_definition_1.ParamDefinition.ObjectList({
        source: param_definition_1.ParamDefinition.MappedStatic('source-cif', {
            'source-cif': param_definition_1.ParamDefinition.EmptyGroup(),
            'url': param_definition_1.ParamDefinition.Group({
                url: param_definition_1.ParamDefinition.Text(''),
                format: MVSAnnotationFormat.PDSelect(),
            }),
        }),
        schema: schemas_1.MVSAnnotationSchema.PDSelect(),
        cifBlock: param_definition_1.ParamDefinition.MappedStatic('index', {
            index: param_definition_1.ParamDefinition.Group({ index: param_definition_1.ParamDefinition.Numeric(0, { min: 0, step: 1 }, { description: '0-based index of the block' }) }),
            header: param_definition_1.ParamDefinition.Group({ header: param_definition_1.ParamDefinition.Text(undefined, { description: 'Block header' }) }),
        }, { description: 'Specify which CIF block contains annotation data (only relevant when format=cif or format=bcif)' }),
        cifCategory: (0, param_definition_2.MaybeStringParamDefinition)({ placeholder: 'Take first category', description: 'Specify which CIF category contains annotation data (only relevant when format=cif or format=bcif)' }),
        fieldRemapping: param_definition_1.ParamDefinition.ObjectList({
            standardName: param_definition_1.ParamDefinition.Text('', { placeholder: ' ', description: 'Standard name of the selector field (e.g. label_asym_id)' }),
            actualName: (0, param_definition_2.MaybeStringParamDefinition)({ placeholder: 'Ignore field', description: 'Actual name of the field in the annotation data (e.g. spam_chain_id), null to ignore the field with standard name' }),
        }, e => `"${e.standardName}": ${e.actualName === null ? 'null' : `"${e.actualName}"`}`, { description: 'Optional remapping of annotation field names { standardName1: actualName1, ... }. Use { "label_asym_id": "X" } to load actual field "X" as "label_asym_id". Use { "label_asym_id": null } to ignore actual field "label_asym_id". Fields not mentioned here are mapped implicitely (i.e. actual name = standard name).' }),
        id: param_definition_1.ParamDefinition.Text('', { description: 'Arbitrary identifier that can be referenced by MVSAnnotationColorTheme' }),
    }, obj => obj.id),
};
/** Provider for custom model property "Annotations" */
exports.MVSAnnotationsProvider = custom_model_property_1.CustomModelProperty.createProvider({
    label: 'MVS Annotations',
    descriptor: (0, custom_property_1.CustomPropertyDescriptor)({
        name: 'mvs-annotations',
    }),
    type: 'static',
    defaultParams: exports.MVSAnnotationsParams,
    getParams: (data) => exports.MVSAnnotationsParams,
    isApplicable: (data) => true,
    obtain: async (ctx, data, props) => {
        var _a;
        props = { ...param_definition_1.ParamDefinition.getDefaultValues(exports.MVSAnnotationsParams), ...props };
        const specs = (_a = props.annotations) !== null && _a !== void 0 ? _a : [];
        const annots = await MVSAnnotations.fromSpecs(ctx, specs, data);
        return { value: annots };
    }
});
/** Represents multiple annotations retrievable by their ID */
class MVSAnnotations {
    constructor(dict) {
        this.dict = dict;
    }
    static async fromSpecs(ctx, specs, model) {
        var _a;
        const sources = specs.map(annotationSourceFromSpec);
        const files = await getFilesFromSources(ctx, sources, model);
        const annots = {};
        for (let i = 0; i < specs.length; i++) {
            const spec = specs[i];
            try {
                const file = files[i];
                if (!file.ok)
                    throw file.error;
                annots[spec.id] = await MVSAnnotation.fromSpec(ctx, spec, file.value);
            }
            catch (err) {
                (_a = ctx.errorContext) === null || _a === void 0 ? void 0 : _a.add('mvs', `Failed to obtain annotation (${err}).\nAnnotation specification source params: ${JSON.stringify(spec.source.params)}`);
                console.error(`Failed to obtain annotation (${err}).\nAnnotation specification:`, spec);
                annots[spec.id] = MVSAnnotation.createEmpty(spec.schema);
            }
        }
        return new MVSAnnotations(annots);
    }
    getAnnotation(id) {
        return this.dict[id];
    }
    getAllAnnotations() {
        return Object.values(this.dict);
    }
}
exports.MVSAnnotations = MVSAnnotations;
/** Retrieve annotation with given `annotationId` from custom model property "MVS Annotations" and the model from which it comes */
function getMVSAnnotationForStructure(structure, annotationId) {
    const models = structure.isEmpty ? [] : structure.models;
    for (const model of models) {
        if (model.customProperties.has(exports.MVSAnnotationsProvider.descriptor)) {
            const annots = exports.MVSAnnotationsProvider.get(model).value;
            const annotation = annots === null || annots === void 0 ? void 0 : annots.getAnnotation(annotationId);
            if (annotation) {
                return { annotation, model };
            }
        }
    }
    return { annotation: undefined, model: undefined };
}
function getIndexedElementsForUnitKind(indexedModel, unitKind) {
    if (unitKind === structure_1.Unit.Kind.Atomic)
        return indexedModel.atoms;
    if (unitKind === structure_1.Unit.Kind.Spheres)
        return indexedModel.spheres;
    if (unitKind === structure_1.Unit.Kind.Gaussians)
        return indexedModel.gaussians;
    console.warn(`Unknown Unit.Kind value: ${unitKind}`);
    return null;
}
/** Main class for processing MVS annotation */
class MVSAnnotation {
    constructor(data, schema, fieldRemapping) {
        this.data = data;
        this.schema = schema;
        this.fieldRemapping = fieldRemapping;
        /** Cached `IndexedModel` per `Model.id` (if annotation contains no instanceIds)
         * or per `Model.id:instanceId` combination (if at least one row contains instanceId). */
        this._indexedModels = new Map();
        /** Cached annotation rows. Do not use directly, use `getRows` instead. */
        this._rows = undefined;
        this._hasInstanceIds = undefined;
        this.nRows = getRowCount(data);
    }
    /** Create a new `MVSAnnotation` based on specification `spec`. Use `file` if provided, otherwise download the file.
     * Throw error if download fails or problem with data. */
    static async fromSpec(ctx, spec, file) {
        var _a;
        file !== null && file !== void 0 ? file : (file = await getFileFromSource(ctx, annotationSourceFromSpec(spec)));
        let data;
        switch (file.format) {
            case 'json':
                data = file;
                break;
            case 'cif':
                if (file.data.blocks.length === 0)
                    throw new Error('No block in CIF');
                const blockSpec = spec.cifBlock;
                let block;
                switch (blockSpec.name) {
                    case 'header':
                        const foundBlock = file.data.blocks.find(b => b.header === blockSpec.params.header);
                        if (!foundBlock)
                            throw new Error(`CIF block with header "${blockSpec.params.header}" not found`);
                        block = foundBlock;
                        break;
                    case 'index':
                        block = file.data.blocks[blockSpec.params.index];
                        if (!block)
                            throw new Error(`CIF block with index ${blockSpec.params.index} not found`);
                        break;
                }
                const categoryName = (_a = spec.cifCategory) !== null && _a !== void 0 ? _a : Object.keys(block.categories)[0];
                if (!categoryName)
                    throw new Error('There are no categories in CIF block');
                const category = block.categories[categoryName];
                if (!category)
                    throw new Error(`CIF category "${categoryName}" not found`);
                data = { format: 'cif', data: category };
                break;
        }
        return new MVSAnnotation(data, spec.schema, Object.fromEntries(spec.fieldRemapping.map(e => [e.standardName, e.actualName])));
    }
    static createEmpty(schema) {
        return new MVSAnnotation({ format: 'json', data: [] }, schema, {});
    }
    /** Return value of field `fieldName` assigned to location `loc`, if any */
    getValueForLocation(loc, fieldName) {
        const indexedModel = this.getIndexedModel(loc.unit.model, loc.unit.conformation.operator.instanceId);
        const indexedElements = getIndexedElementsForUnitKind(indexedModel, loc.unit.kind);
        const iRow = indexedElements ? indexedElements[loc.element] : -1;
        return this.getValueForRow(iRow, fieldName);
    }
    /** Return value of field `fieldName` assigned to `i`-th annotation row, if any */
    getValueForRow(i, fieldName) {
        if (i < 0)
            return undefined;
        switch (this.data.format) {
            case 'json':
                const value = getValueFromJson(i, fieldName, this.data.data);
                if (value === undefined || typeof value === 'string')
                    return value;
                else
                    return `${value}`;
            case 'cif':
                return getValueFromCif(i, fieldName, this.data.data);
        }
    }
    /** Return cached `ElementIndex` -> `MVSAnnotationRow` mapping for `Model` (or create it if not cached yet) */
    getIndexedModel(model, instanceId) {
        const key = this.hasInstanceIds() ? `${model.id}:${instanceId}` : model.id;
        if (!this._indexedModels.has(key)) {
            const result = this.getRowForEachAtom(model, instanceId);
            this._indexedModels.set(key, result);
        }
        return this._indexedModels.get(key);
    }
    /** Create `ElementIndex` -> `MVSAnnotationRow` mapping for `Model` */
    getRowForEachAtom(model, instanceId) {
        const indices = indexing_1.IndicesAndSortings.get(model);
        const nAtoms = model.atomicHierarchy.atoms._rowCount;
        const nSpheres = model.coarseHierarchy.spheres.count;
        const nGaussians = model.coarseHierarchy.gaussians.count;
        let indexedAtoms = null;
        let indexedSpheres = null;
        let indexedGaussians = null;
        const rows = this.getRows();
        for (let iRow = 0, nRows = rows.length; iRow < nRows; iRow++) {
            const row = rows[iRow];
            const atomRanges = (0, selections_1.getAtomRangesForRow)(row, model, instanceId, indices);
            indexedAtoms = fillValueOnRanges(indexedAtoms, nAtoms, atomRanges, iRow);
            const sphereRanges = (0, selections_1.getSphereRangesForRow)(row, model, instanceId, indices);
            indexedSpheres = fillValueOnRanges(indexedSpheres, nSpheres, sphereRanges, iRow);
            const gaussianRanges = (0, selections_1.getGaussianRangesForRow)(row, model, instanceId, indices);
            indexedGaussians = fillValueOnRanges(indexedGaussians, nGaussians, gaussianRanges, iRow);
        }
        return { atoms: indexedAtoms, spheres: indexedSpheres, gaussians: indexedGaussians };
    }
    /** Parse and return all annotation rows in this annotation, or return cached result if available */
    getRows() {
        var _a;
        return (_a = this._rows) !== null && _a !== void 0 ? _a : (this._rows = this._getRows());
    }
    /** Parse and return all annotation rows in this annotation */
    _getRows() {
        switch (this.data.format) {
            case 'json':
                return getRowsFromJson(this.data.data, this.schema, this.fieldRemapping);
            case 'cif':
                return getRowsFromCif(this.data.data, this.schema, this.fieldRemapping);
        }
    }
    /** Return `true` if some rows in the annotation contain `instance_id` field. */
    hasInstanceIds() {
        var _a;
        return (_a = this._hasInstanceIds) !== null && _a !== void 0 ? _a : (this._hasInstanceIds = this.getRows().some(row => (0, utils_1.isDefined)(row.instance_id)));
    }
    /** Return list of all distinct values appearing in field `fieldName`, in order of first occurrence. Ignores special values `.` and `?`. If `caseInsensitive`, make all values uppercase. */
    getDistinctValuesInField(fieldName, caseInsensitive) {
        const seen = new Set();
        const out = [];
        for (let i = 0; i < this.nRows; i++) {
            let value = this.getValueForRow(i, fieldName);
            if (caseInsensitive)
                value = value === null || value === void 0 ? void 0 : value.toUpperCase();
            if (value !== undefined && !seen.has(value)) {
                seen.add(value);
                out.push(value);
            }
        }
        return out;
    }
}
exports.MVSAnnotation = MVSAnnotation;
function getValueFromJson(rowIndex, fieldName, data) {
    var _a, _b;
    const js = data;
    if (Array.isArray(js)) {
        const row = (_a = js[rowIndex]) !== null && _a !== void 0 ? _a : {};
        return row[fieldName];
    }
    else {
        const column = (_b = js[fieldName]) !== null && _b !== void 0 ? _b : [];
        return column[rowIndex];
    }
}
function getValueFromCif(rowIndex, fieldName, data) {
    const column = data.getField(fieldName);
    if (!column)
        return undefined;
    if (column.valueKind(rowIndex) !== db_1.Column.ValueKind.Present)
        return undefined;
    return column.str(rowIndex);
}
/** Return number of rows in this annotation (without parsing all the data) */
function getRowCount(data) {
    switch (data.format) {
        case 'json':
            return getRowCountFromJson(data.data);
        case 'cif':
            return getRowCountFromCif(data.data);
    }
}
function getRowCountFromJson(data) {
    const js = data;
    if (Array.isArray(js)) {
        // array of objects
        return js.length;
    }
    else {
        // object of arrays
        const keys = Object.keys(js);
        if (keys.length > 0) {
            return js[keys[0]].length;
        }
        else {
            return 0;
        }
    }
}
function getRowCountFromCif(data) {
    return data.rowCount;
}
function getRowsFromJson(data, schema, fieldRemapping) {
    const js = data;
    const cifSchema = (0, schemas_1.getCifAnnotationSchema)(schema);
    const cifSchemaKeys = Object.keys(cifSchema);
    if (Array.isArray(js)) {
        // array of objects
        return js.map(row => (0, object_1.pickObjectKeysWithRemapping)(row, cifSchemaKeys, fieldRemapping));
    }
    else {
        // object of arrays
        const selectedFields = (0, object_1.pickObjectKeysWithRemapping)(js, cifSchemaKeys, fieldRemapping);
        return (0, object_1.objectOfArraysToArrayOfObjects)(selectedFields);
    }
}
function getRowsFromCif(data, schema, fieldRemapping) {
    const cifSchema = (0, schemas_1.getCifAnnotationSchema)(schema);
    const cifSchemaKeys = Object.keys(cifSchema);
    const columns = {};
    for (const key of cifSchemaKeys) {
        let srcKey = fieldRemapping[key];
        if (srcKey === null)
            continue; // Ignore key
        if (srcKey === undefined)
            srcKey = key; // Implicit key mapping
        const columnArray = getArrayFromCifCategory(data, srcKey, cifSchema[key]); // Avoiding `column.toArray` as it replaces . and ? fields by 0 or ''
        if (columnArray)
            columns[key] = columnArray;
    }
    if (Object.keys(columns).length === 0)
        return new Array(data.rowCount).fill({});
    return (0, object_1.objectOfArraysToArrayOfObjects)(columns);
}
/** Load data from a specific column in a CIF category into an array. Load `.` and `?` as undefined. */
function getArrayFromCifCategory(data, columnName, columnSchema) {
    if (data.getField(columnName) === undefined)
        return undefined;
    const table = (0, schema_1.toTable)({ [columnName]: columnSchema }, data); // a bit dumb, I don't know how to make column directly
    const column = table[columnName];
    return getArrayFromCifColumn(column); // Avoiding `column.toArray` as it replaces . and ? fields by 0 or ''
}
/** Same as `column.toArray` but reads `.` and `?` as undefined (instead of using type defaults) */
function getArrayFromCifColumn(column) {
    const nRows = column.rowCount;
    const Present = db_1.Column.ValueKind.Present;
    const out = new Array(nRows);
    for (let iRow = 0; iRow < nRows; iRow++) {
        out[iRow] = column.valueKind(iRow) === Present ? column.value(iRow) : undefined;
    }
    return out;
}
async function getFileFromSource(ctx, source, model) {
    switch (source.kind) {
        case 'source-cif':
            return { format: 'cif', data: getSourceFileFromModel(model) };
        case 'url':
            const url = assets_1.Asset.getUrlAsset(ctx.assetManager, source.url);
            const dataType = MVSAnnotationFormatTypes[source.format];
            const dataWrapper = await ctx.assetManager.resolve(url, dataType).runInContext(ctx.runtime);
            const rawData = dataWrapper.data;
            if (!rawData)
                throw new Error('Missing data');
            switch (source.format) {
                case 'json':
                    const json = JSON.parse(rawData);
                    return { format: 'json', data: json };
                case 'cif':
                case 'bcif':
                    const parsed = await cif_1.CIF.parse(rawData).run();
                    if (parsed.isError)
                        throw new Error(`Failed to parse ${source.format}`);
                    return { format: 'cif', data: parsed.result };
            }
    }
}
/** Like `sources.map(s => safePromise(getFileFromSource(ctx, s)))`
 * but downloads a repeating source only once. */
async function getFilesFromSources(ctx, sources, model) {
    var _a;
    const promises = {};
    for (const src of sources) {
        const key = (0, json_1.canonicalJsonString)(src);
        (_a = promises[key]) !== null && _a !== void 0 ? _a : (promises[key] = (0, utils_1.safePromise)(getFileFromSource(ctx, src, model)));
    }
    const files = await (0, object_1.promiseAllObj)(promises);
    return sources.map(src => files[(0, json_1.canonicalJsonString)(src)]);
}
function getSourceFileFromModel(model) {
    if (model && mmcif_1.MmcifFormat.is(model.sourceData)) {
        if (model.sourceData.data.file) {
            return model.sourceData.data.file;
        }
        else {
            const frame = model.sourceData.data.frame;
            const block = (0, cif_1.CifBlock)(Array.from(frame.categoryNames), frame.categories, frame.header);
            const file = (0, cif_1.CifFile)([block]);
            return file;
        }
    }
    else {
        console.warn('Could not get CifFile from Model, returning empty CifFile');
        return (0, cif_1.CifFile)([]);
    }
}
function annotationSourceFromSpec(s) {
    switch (s.source.name) {
        case 'url':
            return { kind: 'url', ...s.source.params };
        case 'source-cif':
            return { kind: 'source-cif' };
    }
}
/** In `array`, set value `fillValue` to all positions described by `fillRanges`. In case `array` is `null`, initialize it with length `n` prefilled with -1. */
function fillValueOnRanges(array, n, fillRanges, fillValue) {
    if (!fillRanges || element_ranges_1.ElementRanges.count(fillRanges) === 0)
        return array;
    const out = array !== null && array !== void 0 ? array : Array(n).fill(-1);
    element_ranges_1.ElementRanges.foreach(fillRanges, (from, to) => out.fill(fillValue, from, to));
    return out;
}
