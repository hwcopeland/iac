"use strict";
/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.MVSData = void 0;
exports.getContext = getContext;
exports.loadFromURL = loadFromURL;
exports.loadFromData = loadFromData;
exports.loadFromID = loadFromID;
exports.downloadCurrentStory = downloadCurrentStory;
const context_1 = require("./context.js");
require("./elements/index.js");
const mvs_data_1 = require("../../extensions/mvs/mvs-data.js");
Object.defineProperty(exports, "MVSData", { enumerable: true, get: function () { return mvs_data_1.MVSData; } });
const download_1 = require("../../mol-util/download.js");
require("./favicon.ico");
require("../../mol-plugin-ui/skin/light.scss");
require("./styles.scss");
require("./index.html");
function getContext(name) {
    return (0, context_1.getMVSStoriesContext)({ name });
}
function loadFromURL(url, options) {
    setTimeout(() => {
        var _a;
        getContext(options === null || options === void 0 ? void 0 : options.contextName).dispatch({
            kind: 'load-mvs',
            format: (_a = options === null || options === void 0 ? void 0 : options.format) !== null && _a !== void 0 ? _a : 'mvsj',
            url,
        });
    }, 0);
}
function loadFromData(data, options) {
    setTimeout(() => {
        var _a;
        getContext(options === null || options === void 0 ? void 0 : options.contextName).dispatch({
            kind: 'load-mvs',
            format: (_a = options === null || options === void 0 ? void 0 : options.format) !== null && _a !== void 0 ? _a : 'mvsj',
            data,
        });
    }, 0);
}
function getStoryUrlFromId(id, format = 'mvsj') {
    return `https://stories.molstar.org/api/story/${id}/data`;
}
function loadFromID(id, options) {
    var _a;
    loadFromURL(getStoryUrlFromId(id, options === null || options === void 0 ? void 0 : options.format), { format: (_a = options === null || options === void 0 ? void 0 : options.format) !== null && _a !== void 0 ? _a : 'mvsj', contextName: options === null || options === void 0 ? void 0 : options.contextName });
}
function downloadCurrentStory(options) {
    var _a;
    const story = getContext(options === null || options === void 0 ? void 0 : options.contextName).state.currentStoryData.value;
    if (!story)
        return;
    const isMVSJ = typeof story === 'string';
    const filename = `${(_a = options === null || options === void 0 ? void 0 : options.filename) !== null && _a !== void 0 ? _a : 'story'}.${isMVSJ ? 'mvsj' : 'mvsx'}`;
    (0, download_1.download)(new Blob([typeof story === 'string' ? story : story.buffer], { type: isMVSJ ? 'application/json' : 'application/octet-stream' }), filename);
}
;
