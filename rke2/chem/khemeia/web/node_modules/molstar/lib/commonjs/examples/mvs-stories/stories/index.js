"use strict";
/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.Stories = void 0;
const kinase_1 = require("./kinase.js");
const tbp_1 = require("./tbp.js");
const animation_1 = require("./animation.js");
const audio_1 = require("./audio.js");
const motm1_1 = require("./motm1.js");
exports.Stories = [
    { id: 'kinase', name: 'BCR-ABL: A Kinase Out of Control', buildStory: kinase_1.buildStory },
    { id: 'tata', name: 'TATA-Binding Protein and its Role in Transcription Initiation ', buildStory: tbp_1.buildStory },
    { id: 'motm1', name: 'RCSB PDB Molecule of the Month #1', buildStory: motm1_1.buildStory },
    { id: 'animation-example', name: 'Molecular Animation Example', buildStory: animation_1.buildStory },
    { id: 'audio-example', name: 'Audio Playback Example', buildStory: audio_1.buildStory },
];
