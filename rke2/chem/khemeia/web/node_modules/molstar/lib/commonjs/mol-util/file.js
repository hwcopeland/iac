"use strict";
/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.fileToDataUri = fileToDataUri;
async function fileToDataUri(file) {
    var _a, _b;
    const filename = file.name.toLowerCase() || 'file';
    const isImage = ['jpg', 'jpeg', 'png', 'gif', 'webp'].some(ext => filename.endsWith(`.${ext}`));
    const isAudio = ['mp3', 'wav', 'ogg'].some(ext => filename.endsWith(`.${ext}`));
    let type = 'application/octet-stream';
    if (isImage) {
        const ext = (_a = filename.split('.').pop()) === null || _a === void 0 ? void 0 : _a.toLowerCase();
        switch (ext) {
            case 'jpg':
                type = 'image/jpeg';
                break;
            default:
                type = `image/${ext}`;
                break;
        }
    }
    else if (isAudio) {
        const ext = (_b = filename.split('.').pop()) === null || _b === void 0 ? void 0 : _b.toLowerCase();
        switch (ext) {
            case 'mp3':
                type = 'audio/mpeg';
                break;
            default:
                type = `audio/${ext}`;
                break;
        }
    }
    const bytes = await file.arrayBuffer();
    const reader = new FileReader();
    reader.readAsDataURL(new Blob([bytes], { type }));
    const data = await new Promise((resolve, reject) => {
        reader.onload = () => resolve(reader.result);
        reader.onerror = () => reject(reader.error);
    });
    return data;
}
