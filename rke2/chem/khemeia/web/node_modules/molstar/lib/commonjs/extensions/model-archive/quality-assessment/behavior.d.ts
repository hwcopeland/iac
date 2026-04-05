/**
 * Copyright (c) 2021-25 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { ParamDefinition as PD } from '../../../mol-util/param-definition.js';
import { StructureRepresentationPresetProvider } from '../../../mol-plugin-state/builder/structure/representation-preset.js';
import { PluginConfigItem } from '../../../mol-plugin/config.js';
export declare const MAQualityAssessmentConfig: {
    EnablePairwiseScorePlot: PluginConfigItem<boolean>;
};
export declare const MAQualityAssessment: import("../../../mol-state/index.js").StateTransformer<any, any, any>;
export declare const QualityAssessmentPLDDTPreset: StructureRepresentationPresetProvider<{
    ignoreHydrogens: boolean | undefined;
    ignoreHydrogensVariant: "all" | "non-polar" | undefined;
    ignoreLight: boolean | undefined;
    quality: "auto" | "medium" | "high" | "low" | "custom" | "highest" | "higher" | "lower" | "lowest" | undefined;
    theme: PD.Normalize<{
        globalName: /*elided*/ any;
        globalColorParams: /*elided*/ any;
        carbonColor: /*elided*/ any;
        symmetryColor: /*elided*/ any;
        symmetryColorParams: /*elided*/ any;
        focus: /*elided*/ any;
    }> | undefined;
}, {
    components?: undefined;
    representations?: undefined;
} | {
    components: {
        polymer: import("../../../mol-state/index.js").StateObjectSelector<import("../../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure, import("../../../mol-state/index.js").StateTransformer<import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined;
    };
    representations: {
        polymer: import("../../../mol-state/index.js").StateObjectSelector<import("../../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure.Representation3D, import("../../../mol-state/index.js").StateTransformer<import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, any>>;
    };
} | {
    components: {
        all: import("../../../mol-state/index.js").StateObjectSelector<import("../../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure, import("../../../mol-state/index.js").StateTransformer<import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined;
        branched: undefined;
    };
    representations: {
        all: import("../../../mol-state/index.js").StateObjectSelector<import("../../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure.Representation3D, import("../../../mol-state/index.js").StateTransformer<import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, any>>;
    };
}>;
export declare const QualityAssessmentQmeanPreset: StructureRepresentationPresetProvider<{
    ignoreHydrogens: boolean | undefined;
    ignoreHydrogensVariant: "all" | "non-polar" | undefined;
    ignoreLight: boolean | undefined;
    quality: "auto" | "medium" | "high" | "low" | "custom" | "highest" | "higher" | "lower" | "lowest" | undefined;
    theme: PD.Normalize<{
        globalName: /*elided*/ any;
        globalColorParams: /*elided*/ any;
        carbonColor: /*elided*/ any;
        symmetryColor: /*elided*/ any;
        symmetryColorParams: /*elided*/ any;
        focus: /*elided*/ any;
    }> | undefined;
}, {
    components?: undefined;
    representations?: undefined;
} | {
    components: {
        polymer: import("../../../mol-state/index.js").StateObjectSelector<import("../../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure, import("../../../mol-state/index.js").StateTransformer<import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined;
    };
    representations: {
        polymer: import("../../../mol-state/index.js").StateObjectSelector<import("../../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure.Representation3D, import("../../../mol-state/index.js").StateTransformer<import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, any>>;
    };
} | {
    components: {
        all: import("../../../mol-state/index.js").StateObjectSelector<import("../../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure, import("../../../mol-state/index.js").StateTransformer<import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined;
        branched: undefined;
    };
    representations: {
        all: import("../../../mol-state/index.js").StateObjectSelector<import("../../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure.Representation3D, import("../../../mol-state/index.js").StateTransformer<import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, any>>;
    };
}>;
