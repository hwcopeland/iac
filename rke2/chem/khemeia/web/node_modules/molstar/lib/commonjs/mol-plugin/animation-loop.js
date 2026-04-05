"use strict";
/**
 * Copyright (c) 2020-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.PluginAnimationLoop = void 0;
const now_1 = require("../mol-util/now.js");
const debug_1 = require("../mol-util/debug.js");
const timer_1 = require("../mol-gl/webgl/timer.js");
const MaxProperFrameDelta = 1000 / 30;
class PluginAnimationLoop {
    get isAnimating() {
        return this._isAnimating;
    }
    async tick(t, options) {
        var _a, _b;
        await this.plugin.managers.animation.tick(t, options === null || options === void 0 ? void 0 : options.isSynchronous, options === null || options === void 0 ? void 0 : options.animation);
        (_a = this.plugin.canvas3d) === null || _a === void 0 ? void 0 : _a.tick(t, options);
        if (debug_1.isTimingMode) {
            const timerResults = (_b = this.plugin.canvas3d) === null || _b === void 0 ? void 0 : _b.webgl.timer.resolve();
            if (timerResults) {
                for (const result of timerResults) {
                    (0, timer_1.printTimerResults)([result]);
                }
            }
        }
    }
    resetTime(t) {
        var _a;
        (_a = this.plugin.canvas3d) === null || _a === void 0 ? void 0 : _a.resetTime(t);
    }
    start(options) {
        var _a, _b;
        (_a = this.plugin.canvas3d) === null || _a === void 0 ? void 0 : _a.resume();
        this._isAnimating = true;
        this.resetTime(0);
        this.properTimeT = 0;
        this.lastTickT = (0, now_1.now)();
        if (options === null || options === void 0 ? void 0 : options.immediate)
            this.frame();
        else
            this.currentFrame = (_b = this.plugin.canvas3d) === null || _b === void 0 ? void 0 : _b.requestAnimationFrame(this.frame);
    }
    stop(options) {
        var _a, _b;
        this._isAnimating = false;
        if (this.currentFrame !== undefined) {
            (_a = this.plugin.canvas3d) === null || _a === void 0 ? void 0 : _a.cancelAnimationFrame(this.currentFrame);
            this.currentFrame = undefined;
        }
        if (options === null || options === void 0 ? void 0 : options.noDraw) {
            (_b = this.plugin.canvas3d) === null || _b === void 0 ? void 0 : _b.pause(options === null || options === void 0 ? void 0 : options.noDraw);
        }
    }
    constructor(plugin) {
        this.plugin = plugin;
        this.lastTickT = 0;
        // Proper time is used to prevent animations from skipping
        // if there is a blocking operation, e.g., shader compilation
        // The drawback of this is that sometimes the animation will take
        // longer than intended, but hopefully that's a reasonable tradeoff
        this.properTimeT = 0;
        this.currentFrame = undefined;
        this._isAnimating = false;
        this.frame = (_timestamp, xrFrame) => {
            var _a;
            const t = (0, now_1.now)();
            const dt = t - this.lastTickT;
            this.lastTickT = t;
            this.properTimeT += Math.min(dt, MaxProperFrameDelta);
            this.tick(this.properTimeT, { xrFrame });
            if (this._isAnimating) {
                this.currentFrame = (_a = this.plugin.canvas3d) === null || _a === void 0 ? void 0 : _a.requestAnimationFrame(this.frame);
            }
        };
    }
}
exports.PluginAnimationLoop = PluginAnimationLoop;
