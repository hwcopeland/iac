/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Russell Parker <russell@benchling.com>
 * @author Herman Bergwerf <post@hbergwerf.nl>
 */
import { Observable } from 'rxjs';
import { Viewport } from '../../mol-canvas3d/camera/util.js';
import { Ray3D } from '../../mol-math/geometry/primitives/ray3d.js';
import { BitFlags } from '../bit-flags.js';
export declare function getButtons(event: MouseEvent | Touch): number;
export declare function getButton(event: MouseEvent | Touch): number;
export declare function getModifiers(event: MouseEvent | Touch): ModifiersKeys;
export declare const DefaultInputObserverProps: {
    noScroll: boolean;
    noMiddleClickScroll: boolean;
    noContextMenu: boolean;
    noPinchZoom: boolean;
    noTextSelect: boolean;
    preventGestures: boolean;
    mask: (x: number, y: number) => boolean;
    pixelScale: number;
};
export type InputObserverProps = Partial<typeof DefaultInputObserverProps>;
export type ModifiersKeys = {
    shift: boolean;
    alt: boolean;
    control: boolean;
    meta: boolean;
};
export declare namespace ModifiersKeys {
    const None: Readonly<ModifiersKeys>;
    function areEqual(a: ModifiersKeys, b: ModifiersKeys): boolean;
    function areNone(a: ModifiersKeys): boolean;
    function size(a?: ModifiersKeys): number;
    function create(modifierKeys?: Partial<ModifiersKeys>): ModifiersKeys;
}
export type ButtonsType = BitFlags<ButtonsType.Flag>;
export declare namespace ButtonsType {
    const has: (btn: ButtonsType, f: Flag) => boolean;
    const create: (fs: Flag) => ButtonsType;
    enum Flag {
        /** No button or un-initialized */
        None = 0,
        /** Primary button (usually left) */
        Primary = 1,
        /** Secondary button (usually right) */
        Secondary = 2,
        /** Auxilary button (usually middle or mouse wheel button)  */
        Auxilary = 4,
        /** 4th button (typically the "Browser Back" button) */
        Forth = 8,
        /** 5th button (typically the "Browser Forward" button) */
        Five = 16,
        /** Trigger button */
        Trigger = 32,
        /** Squeeze button */
        Squeeze = 48
    }
}
export type KeyCode = string;
export type PointerHandedness = 'left' | 'right' | 'none';
type BaseInput = {
    buttons: ButtonsType;
    button: ButtonsType.Flag;
    modifiers: ModifiersKeys;
};
export type DragInput = {
    x: number;
    y: number;
    dx: number;
    dy: number;
    pageX: number;
    pageY: number;
    isStart: boolean;
    useDelta?: boolean;
} & BaseInput;
export type WheelInput = {
    x: number;
    y: number;
    pageX: number;
    pageY: number;
    dx: number;
    dy: number;
    dz: number;
    spinX: number;
    spinY: number;
} & BaseInput;
export type ClickInput = {
    x: number;
    y: number;
    pageX: number;
    pageY: number;
    ray?: Ray3D;
} & BaseInput;
export type MoveInput = {
    x: number;
    y: number;
    pageX: number;
    pageY: number;
    ray?: Ray3D;
    movementX?: number;
    movementY?: number;
    inside: boolean;
    onElement: boolean;
} & BaseInput;
export type PinchInput = {
    isStart: boolean;
    distance: number;
    delta: number;
    fraction: number;
    fractionDelta: number;
    startX: number;
    startY: number;
    centerPageX: number;
    centerPageY: number;
} & BaseInput;
export type GestureInput = {
    scale: number;
    rotation: number;
    deltaScale: number;
    deltaRotation: number;
    isStart?: boolean;
    isEnd?: boolean;
} & BaseInput;
export type KeyInput = {
    key: string;
    code: string;
    modifiers: ModifiersKeys;
    x: number;
    y: number;
    pageX: number;
    pageY: number;
    /** for overwriting browser shortcuts like `ctrl+s` as needed */
    preventDefault: () => void;
};
export type TrackedPointerInput = {
    handedness: PointerHandedness;
    buttons: ButtonsType;
    x: number;
    y: number;
    dx: number;
    dy: number;
    ray: Ray3D;
    axes?: readonly number[];
};
export type ScreenTouchInput = {
    x: number;
    y: number;
    ray: Ray3D;
};
export declare const EmptyKeyInput: KeyInput;
export type ResizeInput = {};
interface InputObserver {
    noScroll: boolean;
    noContextMenu: boolean;
    readonly width: number;
    readonly height: number;
    readonly pixelRatio: number;
    readonly pointerLock: boolean;
    readonly drag: Observable<DragInput>;
    readonly interactionEnd: Observable<undefined>;
    readonly wheel: Observable<WheelInput>;
    readonly pinch: Observable<PinchInput>;
    readonly gesture: Observable<GestureInput>;
    readonly click: Observable<ClickInput>;
    readonly move: Observable<MoveInput>;
    readonly leave: Observable<undefined>;
    readonly enter: Observable<undefined>;
    readonly resize: Observable<ResizeInput>;
    readonly modifiers: Observable<ModifiersKeys>;
    readonly key: Observable<KeyInput>;
    readonly keyUp: Observable<KeyInput>;
    readonly keyDown: Observable<KeyInput>;
    readonly lock: Observable<boolean>;
    readonly trackedPointers: Observable<TrackedPointerInput[]>;
    setPixelScale: (pixelScale: number) => void;
    updateTrackedPointers: (trackedPointers: TrackedPointerInput[]) => void;
    updateScreenTouches: (screenTouches: ScreenTouchInput[]) => void;
    requestPointerLock: (viewport: Viewport) => void;
    exitPointerLock: () => void;
    dispose: () => void;
}
declare namespace InputObserver {
    function create(props?: InputObserverProps): InputObserver;
    function fromElement(element: Element, props?: InputObserverProps): InputObserver;
}
export declare function normalizeWheel(event: any): {
    spinX: number;
    spinY: number;
    dx: number;
    dy: number;
    dz: number;
};
export { InputObserver };
