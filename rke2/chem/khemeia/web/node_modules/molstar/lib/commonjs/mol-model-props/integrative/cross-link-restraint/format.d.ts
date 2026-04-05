/**
 * Copyright (c) 2018-2020 Mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Model } from '../../../mol-model/structure/model/model.js';
import { Table } from '../../../mol-data/db.js';
import { mmCIF_Schema } from '../../../mol-io/reader/cif/schema/mmcif.js';
import { Unit } from '../../../mol-model/structure.js';
import { ElementIndex } from '../../../mol-model/structure/model/indexing.js';
import { FormatPropertyProvider } from '../../../mol-model-formats/structure/common/property.js';
import { CustomPropertyDescriptor } from '../../../mol-model/custom-property.js';
export { ModelCrossLinkRestraint };
interface ModelCrossLinkRestraint {
    getIndicesByElement: (element: ElementIndex, kind: Unit.Kind) => number[];
    data: Table<mmCIF_Schema['ihm_cross_link_restraint']>;
}
declare namespace ModelCrossLinkRestraint {
    const Descriptor: CustomPropertyDescriptor;
    const Provider: FormatPropertyProvider<ModelCrossLinkRestraint>;
    function fromTable(table: Table<mmCIF_Schema['ihm_cross_link_restraint']>, model: Model): ModelCrossLinkRestraint;
}
