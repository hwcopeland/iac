import { PluginBehavior } from '../../../mol-plugin/behavior.js';
import { ParamDefinition as PD } from '../../../mol-util/param-definition.js';
import { SbNcbrPartialChargesColorThemeProvider } from './color.js';
import { SbNcbrPartialChargesPropertyProvider } from './property.js';
import { SbNcbrPartialChargesLociLabelProvider } from './labels.js';
import { SbNcbrPartialChargesPreset } from './preset.js';
export const SbNcbrPartialCharges = PluginBehavior.create({
    name: 'sb-ncbr-partial-charges',
    category: 'misc',
    display: {
        name: 'SB NCBR Partial Charges',
    },
    ctor: class extends PluginBehavior.Handler {
        constructor() {
            super(...arguments);
            this.SbNcbrPartialChargesLociLabelProvider = SbNcbrPartialChargesLociLabelProvider(this.ctx);
        }
        register() {
            this.ctx.customModelProperties.register(SbNcbrPartialChargesPropertyProvider, this.params.autoAttach);
            this.ctx.representation.structure.themes.colorThemeRegistry.add(SbNcbrPartialChargesColorThemeProvider);
            this.ctx.managers.lociLabels.addProvider(this.SbNcbrPartialChargesLociLabelProvider);
            this.ctx.builders.structure.representation.registerPreset(SbNcbrPartialChargesPreset);
        }
        unregister() {
            this.ctx.customModelProperties.unregister(SbNcbrPartialChargesPropertyProvider.descriptor.name);
            this.ctx.representation.structure.themes.colorThemeRegistry.remove(SbNcbrPartialChargesColorThemeProvider);
            this.ctx.managers.lociLabels.removeProvider(this.SbNcbrPartialChargesLociLabelProvider);
            this.ctx.builders.structure.representation.unregisterPreset(SbNcbrPartialChargesPreset);
        }
    },
    params: () => ({
        autoAttach: PD.Boolean(true),
        showToolTip: PD.Boolean(true),
    }),
});
