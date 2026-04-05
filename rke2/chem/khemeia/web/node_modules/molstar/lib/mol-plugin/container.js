/**
 * Copyright (c) 2024-25 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
export class PluginContainer {
    mount(target) {
        var _a;
        if (this.parent.parentElement !== target) {
            (_a = this.parent.parentElement) === null || _a === void 0 ? void 0 : _a.removeChild(this.parent);
        }
        target.appendChild(this.parent);
    }
    unmount() {
        var _a;
        (_a = this.parent.parentElement) === null || _a === void 0 ? void 0 : _a.removeChild(this.parent);
    }
    /**
     * options.checkeredCanvasBackground has no effect. Use canvas3d.checkeredTransparentBackground instead.
     * TODO: remove in v6
     */
    constructor(options) {
        this.options = options;
        const parent = document.createElement('div');
        Object.assign(parent.style, {
            position: 'absolute',
            left: 0,
            top: 0,
            right: 0,
            bottom: 0,
            '-webkit-user-select': 'none',
            'user-select': 'none',
            '-webkit-tap-highlight-color': 'rgba(0,0,0,0)',
            '-webkit-touch-callout': 'none',
            'touch-action': 'manipulation',
        });
        let canvas = options === null || options === void 0 ? void 0 : options.canvas;
        if (!canvas) {
            canvas = document.createElement('canvas');
            parent.appendChild(canvas);
        }
        this.canvas = canvas;
        this.parent = parent;
    }
}
