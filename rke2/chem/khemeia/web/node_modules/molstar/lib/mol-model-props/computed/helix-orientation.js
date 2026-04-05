/**
 * Copyright (c) 2021 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { CustomPropertyDescriptor } from '../../mol-model/custom-property.js';
import { CustomModelProperty } from '../common/custom-model-property.js';
import { calcHelixOrientation } from './helix-orientation/helix-orientation.js';
export const HelixOrientationParams = {};
export const HelixOrientationProvider = CustomModelProperty.createProvider({
    label: 'Helix Orientation',
    descriptor: CustomPropertyDescriptor({
        name: 'molstar_helix_orientation'
    }),
    type: 'dynamic',
    defaultParams: {},
    getParams: () => ({}),
    isApplicable: (data) => true,
    obtain: async (ctx, data) => {
        return { value: calcHelixOrientation(data) };
    }
});
