/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { getMVSStoriesContext } from './context.js';
import './elements/index.js';
import { MVSData } from '../../extensions/mvs/mvs-data.js';
import { download } from '../../mol-util/download.js';
import './favicon.ico';
import '../../mol-plugin-ui/skin/light.scss';
import './styles.scss';
import './index.html';
export function getContext(name) {
    return getMVSStoriesContext({ name });
}
export function loadFromURL(url, options) {
    setTimeout(() => {
        var _a;
        getContext(options === null || options === void 0 ? void 0 : options.contextName).dispatch({
            kind: 'load-mvs',
            format: (_a = options === null || options === void 0 ? void 0 : options.format) !== null && _a !== void 0 ? _a : 'mvsj',
            url,
        });
    }, 0);
}
export function loadFromData(data, options) {
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
export function loadFromID(id, options) {
    var _a;
    loadFromURL(getStoryUrlFromId(id, options === null || options === void 0 ? void 0 : options.format), { format: (_a = options === null || options === void 0 ? void 0 : options.format) !== null && _a !== void 0 ? _a : 'mvsj', contextName: options === null || options === void 0 ? void 0 : options.contextName });
}
export function downloadCurrentStory(options) {
    var _a;
    const story = getContext(options === null || options === void 0 ? void 0 : options.contextName).state.currentStoryData.value;
    if (!story)
        return;
    const isMVSJ = typeof story === 'string';
    const filename = `${(_a = options === null || options === void 0 ? void 0 : options.filename) !== null && _a !== void 0 ? _a : 'story'}.${isMVSJ ? 'mvsj' : 'mvsx'}`;
    download(new Blob([typeof story === 'string' ? story : story.buffer], { type: isMVSJ ? 'application/json' : 'application/octet-stream' }), filename);
}
;
export { MVSData };
