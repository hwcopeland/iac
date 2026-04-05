/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { buildStory as kinase } from './kinase.js';
import { buildStory as tbp } from './tbp.js';
import { buildStory as animation } from './animation.js';
import { buildStory as audio } from './audio.js';
import { buildStory as motm1 } from './motm1.js';
export declare const Stories: readonly [{
    readonly id: "kinase";
    readonly name: "BCR-ABL: A Kinase Out of Control";
    readonly buildStory: typeof kinase;
}, {
    readonly id: "tata";
    readonly name: "TATA-Binding Protein and its Role in Transcription Initiation ";
    readonly buildStory: typeof tbp;
}, {
    readonly id: "motm1";
    readonly name: "RCSB PDB Molecule of the Month #1";
    readonly buildStory: typeof motm1;
}, {
    readonly id: "animation-example";
    readonly name: "Molecular Animation Example";
    readonly buildStory: typeof animation;
}, {
    readonly id: "audio-example";
    readonly name: "Audio Playback Example";
    readonly buildStory: typeof audio;
}];
