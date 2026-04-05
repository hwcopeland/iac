import { jsx as _jsx } from "react/jsx-runtime";
/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Adam Midlik <midlik@gmail.com>
 */
import * as React from 'react';
import { Subject } from 'rxjs';
import { throttleTime } from 'rxjs/operators';
import { OrderedSet } from '../../mol-data/int.js';
import { EveryLoci } from '../../mol-model/loci.js';
import { StructureElement, StructureProperties, Unit } from '../../mol-model/structure.js';
import { PluginCommands } from '../../mol-plugin/commands.js';
import { Representation } from '../../mol-repr/representation.js';
import { Task } from '../../mol-task/index.js';
import { Color } from '../../mol-util/color/index.js';
import { ButtonsType, getButton, getButtons, getModifiers } from '../../mol-util/input/input-observer.js';
import { memoizeLatest } from '../../mol-util/memoize.js';
import { PluginUIComponent } from '../base.js';
/** Note, if this is changed, the CSS for `msp-sequence-number` needs adjustment too */
const MaxSequenceNumberSize = 5;
const DefaultMarkerColors = {
    selected: 'rgb(51, 255, 25)',
    highlighted: 'rgb(255, 102, 153)',
    focused: '',
};
// TODO: this is somewhat inefficient and should be done using a canvas.
export class Sequence extends PluginUIComponent {
    constructor() {
        super(...arguments);
        this.parentDiv = React.createRef();
        this.lastMouseOverSeqIdx = -1;
        this.highlightQueue = new Subject();
        this.markerColors = { ...DefaultMarkerColors };
        /** @experimental */
        this.customColorThemeWrapper = undefined;
        /** @experimental Custom function that assigns color to residues in the Sequence component (unless highlighted or selected) */
        this.customColorFunction = undefined;
        this.lociHighlightProvider = (loci, action) => {
            const changed = this.props.sequenceWrapper.markResidue(loci.loci, action);
            if (changed)
                this.updateMarker();
        };
        this.lociSelectionProvider = (loci, action) => {
            const changed = this.props.sequenceWrapper.markResidue(loci.loci, action);
            if (changed)
                this.updateMarker();
        };
        this.contextMenu = (e) => {
            e.preventDefault();
        };
        this.mouseDownLoci = undefined;
        this.mouseDown = (e) => {
            e.stopPropagation();
            const seqIdx = this.getSeqIdx(e);
            const loci = this.getLoci(seqIdx);
            this.mouseDownLoci = loci;
        };
        this.mouseUp = (e) => {
            e.stopPropagation();
            // ignore mouse-up events without a bound loci
            if (this.mouseDownLoci === undefined)
                return;
            const seqIdx = this.getSeqIdx(e);
            const loci = this.getLoci(seqIdx);
            if (loci) {
                const buttons = getButtons(e.nativeEvent);
                const button = getButton(e.nativeEvent);
                const modifiers = getModifiers(e.nativeEvent);
                let range = loci;
                if (!StructureElement.Loci.areEqual(this.mouseDownLoci, loci)) {
                    const ref = this.mouseDownLoci.elements[0];
                    const ext = loci.elements[0];
                    const min = Math.min(OrderedSet.min(ref.indices), OrderedSet.min(ext.indices));
                    const max = Math.max(OrderedSet.max(ref.indices), OrderedSet.max(ext.indices));
                    range = StructureElement.Loci(loci.structure, [{
                            unit: ref.unit,
                            indices: OrderedSet.ofRange(min, max)
                        }]);
                }
                this.click(range, buttons, button, modifiers);
            }
            this.mouseDownLoci = undefined;
        };
        this.location = StructureElement.Location.create(void 0);
        this.mouseMove = (e) => {
            e.stopPropagation();
            const buttons = getButtons(e.nativeEvent);
            const button = getButton(e.nativeEvent);
            const modifiers = getModifiers(e.nativeEvent);
            const el = e.target;
            if (!el || !el.getAttribute) {
                if (this.lastMouseOverSeqIdx === -1)
                    return;
                this.lastMouseOverSeqIdx = -1;
                this.highlightQueue.next({ seqIdx: -1, buttons, button, modifiers });
                return;
            }
            const seqIdx = el.hasAttribute('data-seqid') ? +el.getAttribute('data-seqid') : -1;
            if (this.lastMouseOverSeqIdx === seqIdx) {
                return;
            }
            else {
                this.lastMouseOverSeqIdx = seqIdx;
                if (this.mouseDownLoci !== undefined) {
                    const loci = this.getLoci(seqIdx);
                    this.hover(loci, ButtonsType.Flag.None, ButtonsType.Flag.None, modifiers);
                }
                else {
                    this.highlightQueue.next({ seqIdx, buttons, button, modifiers });
                }
            }
        };
        this.mouseLeave = (e) => {
            e.stopPropagation();
            this.mouseDownLoci = undefined;
            if (this.lastMouseOverSeqIdx === -1)
                return;
            this.lastMouseOverSeqIdx = -1;
            const buttons = getButtons(e.nativeEvent);
            const button = getButton(e.nativeEvent);
            const modifiers = getModifiers(e.nativeEvent);
            this.highlightQueue.next({ seqIdx: -1, buttons, button, modifiers });
        };
    }
    get sequenceNumberPeriod() {
        if (this.props.sequenceNumberPeriod !== undefined) {
            return this.props.sequenceNumberPeriod;
        }
        if (this.props.sequenceWrapper.length > 10)
            return 10;
        const lastSeqNum = this.getSequenceNumber(this.props.sequenceWrapper.length - 1);
        if (lastSeqNum.length > 1)
            return 5;
        return 1;
    }
    componentDidMount() {
        this.plugin.managers.interactivity.lociHighlights.addProvider(this.lociHighlightProvider);
        this.plugin.managers.interactivity.lociSelects.addProvider(this.lociSelectionProvider);
        this.subscribe(this.highlightQueue.pipe(throttleTime(3 * 16.666, void 0, { leading: true, trailing: true })), (e) => {
            const loci = this.getLoci(e.seqIdx < 0 ? void 0 : e.seqIdx);
            this.hover(loci, e.buttons, e.button, e.modifiers);
        });
        this.subscribe(this.plugin.managers.structure.focus.behaviors.current, focus => {
            this.updateFocus(focus === null || focus === void 0 ? void 0 : focus.loci);
            this.updateMarker();
        });
        this.updateColors();
        PluginCommands.Canvas3D.SetSettings.subscribe(this.plugin, () => {
            this.updateColors();
            this.updateMarker();
        });
        const experimentalSequenceColorTheme = this.plugin.customUIState.experimentalSequenceColorTheme;
        if (experimentalSequenceColorTheme) {
            this.subscribe(experimentalSequenceColorTheme, theme => {
                if (!theme && !this.customColorThemeWrapper)
                    return;
                this.customColorThemeWrapper = ColorThemeWrapper(this.plugin, theme, () => this.forceUpdate());
                this.forceUpdate();
            });
        }
    }
    updateColors() {
        if (this.plugin.canvas3d) {
            this.markerColors.highlighted = Color.toHexStyle(this.plugin.canvas3d.props.renderer.highlightColor);
            this.markerColors.selected = Color.toHexStyle(this.plugin.canvas3d.props.renderer.selectColor);
        }
        else {
            this.markerColors.highlighted = DefaultMarkerColors.highlighted;
            this.markerColors.selected = DefaultMarkerColors.selected;
        }
    }
    updateFocus(loci) {
        this.props.sequenceWrapper.markResidue(EveryLoci, 'unfocus');
        if (loci) {
            this.props.sequenceWrapper.markResidue(loci, 'focus');
        }
    }
    componentWillUnmount() {
        super.componentWillUnmount();
        this.plugin.managers.interactivity.lociHighlights.removeProvider(this.lociHighlightProvider);
        this.plugin.managers.interactivity.lociSelects.removeProvider(this.lociSelectionProvider);
    }
    getLoci(seqIdx) {
        if (seqIdx !== undefined) {
            const loci = this.props.sequenceWrapper.getLoci(seqIdx);
            if (!StructureElement.Loci.isEmpty(loci))
                return loci;
        }
    }
    getSeqIdx(e) {
        let seqIdx = undefined;
        const el = e.target;
        if (el && el.getAttribute) {
            seqIdx = el.hasAttribute('data-seqid') ? +el.getAttribute('data-seqid') : undefined;
        }
        return seqIdx;
    }
    hover(loci, buttons, button, modifiers) {
        const ev = { current: Representation.Loci.Empty, buttons, button, modifiers };
        if (loci !== undefined && !StructureElement.Loci.isEmpty(loci)) {
            ev.current = { loci };
            if (this.mouseDownLoci) {
                const ref = this.mouseDownLoci.elements[0];
                const ext = loci.elements[0];
                const min = Math.min(OrderedSet.min(ref.indices), OrderedSet.min(ext.indices));
                const max = Math.max(OrderedSet.max(ref.indices), OrderedSet.max(ext.indices));
                const range = StructureElement.Loci(loci.structure, [{
                        unit: ref.unit,
                        indices: OrderedSet.ofRange(min, max)
                    }]);
                ev.current = { loci: range };
            }
        }
        this.plugin.behaviors.interaction.hover.next(ev);
    }
    click(loci, buttons, button, modifiers) {
        const ev = { current: Representation.Loci.Empty, buttons, button, modifiers };
        if (loci !== undefined && !StructureElement.Loci.isEmpty(loci)) {
            ev.current = { loci };
        }
        this.plugin.behaviors.interaction.click.next(ev);
    }
    getBackgroundColor(seqIdx) {
        const seqWrapper = this.props.sequenceWrapper;
        if (seqWrapper.isHighlighted(seqIdx) && this.markerColors.highlighted)
            return this.markerColors.highlighted;
        if (seqWrapper.isSelected(seqIdx) && this.markerColors.selected)
            return this.markerColors.selected;
        if (seqWrapper.isFocused(seqIdx) && this.markerColors.focused)
            return this.markerColors.focused;
        if (this.customColorFunction) {
            return this.customColorFunction(seqIdx);
        }
        return '';
    }
    getResidueClass(seqIdx, label) {
        const seqWrapper = this.props.sequenceWrapper;
        const classes = [seqWrapper.residueClass(seqIdx)];
        if (label.length > 1) {
            classes.push(seqIdx === 0 ? 'msp-sequence-residue-long-begin' : 'msp-sequence-residue-long');
        }
        if (seqWrapper.isHighlighted(seqIdx))
            classes.push('msp-sequence-residue-highlighted');
        if (seqWrapper.isSelected(seqIdx))
            classes.push('msp-sequence-residue-selected');
        if (seqWrapper.isFocused(seqIdx))
            classes.push('msp-sequence-residue-focused');
        return classes.join(' ');
    }
    residue(seqIdx, label) {
        return _jsx("span", { "data-seqid": seqIdx, style: { backgroundColor: this.getBackgroundColor(seqIdx) }, className: this.getResidueClass(seqIdx, label), children: `\u200b${label}\u200b` }, seqIdx);
    }
    getSequenceNumberClass(seqIdx, seqNum, label) {
        const classList = ['msp-sequence-number'];
        if (seqNum.startsWith('-')) {
            if (label.length > 1 && seqIdx > 0)
                classList.push('msp-sequence-number-long-negative');
            else
                classList.push('msp-sequence-number-negative');
        }
        else {
            if (label.length > 1 && seqIdx > 0)
                classList.push('msp-sequence-number-long');
        }
        return classList.join(' ');
    }
    getSequenceNumber(seqIdx) {
        let seqNum = '';
        const loci = this.props.sequenceWrapper.getLoci(seqIdx);
        const l = StructureElement.Loci.getFirstLocation(loci, this.location);
        if (l) {
            if (Unit.isAtomic(l.unit)) {
                const seqId = StructureProperties.residue.auth_seq_id(l);
                const insCode = StructureProperties.residue.pdbx_PDB_ins_code(l);
                seqNum = `${seqId}${insCode ? insCode : ''}`;
            }
            else if (Unit.isCoarse(l.unit)) {
                seqNum = `${seqIdx + 1}`;
            }
        }
        return seqNum;
    }
    padSeqNum(n) {
        if (n.length < MaxSequenceNumberSize)
            return n + new Array(MaxSequenceNumberSize - n.length + 1).join('\u00A0');
        return n;
    }
    getSequenceNumberSpan(seqIdx, label) {
        const seqNum = this.getSequenceNumber(seqIdx);
        return _jsx("span", { className: this.getSequenceNumberClass(seqIdx, seqNum, label), children: this.padSeqNum(seqNum) }, `marker-${seqIdx}`);
    }
    updateMarker() {
        if (!this.parentDiv.current)
            return;
        const xs = this.parentDiv.current.children;
        const hasNumbers = !this.props.hideSequenceNumbers, period = this.sequenceNumberPeriod;
        const seqWrapper = this.props.sequenceWrapper;
        const seqLength = seqWrapper.length;
        let o = 0;
        for (let i = 0; i < seqLength; i++) {
            if (hasNumbers && i % period === 0 && i < seqLength)
                o++;
            // o + 1 to account for help icon
            const span = xs[o];
            if (!span)
                return;
            o++;
            const className = this.getResidueClass(i, seqWrapper.residueLabel(i));
            if (span.className !== className)
                span.className = className;
            const backgroundColor = this.getBackgroundColor(i);
            if (span.style.backgroundColor !== backgroundColor)
                span.style.backgroundColor = backgroundColor;
        }
    }
    render() {
        var _a;
        const sw = this.props.sequenceWrapper;
        const elems = [];
        if (this.customColorThemeWrapper) {
            this.customColorFunction = this.customColorThemeWrapper.getColorFunction(sw);
        }
        const hasNumbers = !this.props.hideSequenceNumbers, period = this.sequenceNumberPeriod;
        for (let i = 0, il = sw.length; i < il; ++i) {
            const label = sw.residueLabel(i);
            // add sequence number before name so the html element do not get separated by a line-break
            if (hasNumbers && i % period === 0 && i < il) {
                elems[elems.length] = this.getSequenceNumberSpan(i, label);
            }
            elems[elems.length] = this.residue(i, label);
        }
        // ensure the focus markers are updated after sequenceRender is recreated
        this.updateFocus((_a = this.plugin.managers.structure.focus.behaviors.current.value) === null || _a === void 0 ? void 0 : _a.loci);
        // calling .updateMarker here is neccesary to ensure existing
        // residue spans are updated as react won't update them
        this.updateMarker();
        return _jsx("div", { className: 'msp-sequence-wrapper', onContextMenu: this.contextMenu, onMouseDown: this.mouseDown, onMouseUp: this.mouseUp, onMouseMove: this.mouseMove, onMouseLeave: this.mouseLeave, ref: this.parentDiv, children: elems });
    }
}
function ColorThemeWrapper(plugin, theme, forceUpdate) {
    const tmpLocation = StructureElement.Location.create();
    function computeColor(sequenceWrapper, idx, locationColor) {
        const loci = sequenceWrapper.getLoci(idx);
        if (!loci || StructureElement.Loci.isEmpty(loci))
            return '';
        StructureElement.Loci.getFirstLocation(loci, tmpLocation);
        const color = locationColor(tmpLocation, false);
        if (color < 0)
            return ''; // Color(-1) is used as special value NoColor
        return Color.toHexStyle(color);
    }
    const getColorFunction = memoizeLatest((sequenceWrapper) => {
        var _a, _b, _c;
        if (!theme)
            return undefined;
        const structure = (_a = sequenceWrapper.getLoci(0)) === null || _a === void 0 ? void 0 : _a.structure;
        if (!structure)
            return undefined;
        let themeColor = undefined;
        const props = (_c = (_b = theme.getProps) === null || _b === void 0 ? void 0 : _b.call(theme, { structure })) !== null && _c !== void 0 ? _c : theme.provider.defaultValues;
        if (theme.provider.ensureCustomProperties) {
            // The following task runs asynchronously
            plugin.runTask(Task.create('Attach custom properties for coloring theme', async (runtime) => {
                var _a;
                try {
                    await ((_a = theme.provider.ensureCustomProperties) === null || _a === void 0 ? void 0 : _a.attach({ assetManager: plugin.managers.asset, runtime }, { structure }));
                }
                catch (err) {
                    console.warn(`Failed to attach custom properties needed for coloring theme ${theme.provider.name}:`, err);
                }
                finally {
                    themeColor = theme.provider.factory({ structure }, props).color;
                    forceUpdate();
                }
            }));
        }
        else {
            themeColor = theme.provider.factory({ structure }, props).color;
        }
        const cache = {};
        return (idx) => {
            var _a;
            if (themeColor) { // custom properties ready
                return (_a = cache[idx]) !== null && _a !== void 0 ? _a : (cache[idx] = computeColor(sequenceWrapper, idx, themeColor));
            }
            else { // custom properties not ready
                return '';
            }
        };
    });
    return {
        themeName: theme === null || theme === void 0 ? void 0 : theme.provider.name,
        getColorFunction,
    };
}
