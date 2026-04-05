/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Russell Parker <russell@benchling.com>
 * @author Herman Bergwerf <post@hbergwerf.nl>
 */
import { Subject } from 'rxjs';
import { Viewport } from '../../mol-canvas3d/camera/util.js';
import { Vec2 } from '../../mol-math/linear-algebra/3d/vec2.js';
import { Vec3 } from '../../mol-math/linear-algebra/3d/vec3.js';
import { EPSILON } from '../../mol-math/linear-algebra/3d/common.js';
import { BitFlags } from '../bit-flags.js';
import { noop } from '../index.js';
export function getButtons(event) {
    if (typeof event === 'object') {
        if ('buttons' in event) {
            return event.buttons;
        }
        else if ('which' in event) {
            const b = event.which; // 'any' to support older browsers
            if (b === 2) {
                return 4;
            }
            else if (b === 3) {
                return 2;
            }
            else if (b > 0) {
                return 1 << (b - 1);
            }
        }
    }
    return 0;
}
export function getButton(event) {
    if (typeof event === 'object') {
        if ('button' in event) {
            const b = event.button;
            if (b === 1) {
                return 4;
            }
            else if (b === 2) {
                return 2;
            }
            else if (b >= 0) {
                return 1 << b;
            }
        }
    }
    return 0;
}
export function getModifiers(event) {
    return {
        alt: 'altKey' in event ? event.altKey : false,
        shift: 'shiftKey' in event ? event.shiftKey : false,
        control: 'ctrlKey' in event ? event.ctrlKey : false,
        meta: 'metaKey' in event ? event.metaKey : false
    };
}
export const DefaultInputObserverProps = {
    noScroll: true,
    noMiddleClickScroll: true,
    noContextMenu: true,
    noPinchZoom: true,
    noTextSelect: true,
    preventGestures: false,
    mask: (x, y) => true,
    pixelScale: 1
};
export var ModifiersKeys;
(function (ModifiersKeys) {
    ModifiersKeys.None = create();
    function areEqual(a, b) {
        return a.shift === b.shift && a.alt === b.alt && a.control === b.control && a.meta === b.meta;
    }
    ModifiersKeys.areEqual = areEqual;
    function areNone(a) {
        return areEqual(a, ModifiersKeys.None);
    }
    ModifiersKeys.areNone = areNone;
    function size(a) {
        if (!a)
            return 0;
        let ret = 0;
        if (!!a.shift)
            ret++;
        if (!!a.alt)
            ret++;
        if (!!a.control)
            ret++;
        if (!!a.meta)
            ret++;
        return ret;
    }
    ModifiersKeys.size = size;
    function create(modifierKeys = {}) {
        return {
            shift: !!modifierKeys.shift,
            alt: !!modifierKeys.alt,
            control: !!modifierKeys.control,
            meta: !!modifierKeys.meta
        };
    }
    ModifiersKeys.create = create;
})(ModifiersKeys || (ModifiersKeys = {}));
export var ButtonsType;
(function (ButtonsType) {
    ButtonsType.has = BitFlags.has;
    ButtonsType.create = BitFlags.create;
    let Flag;
    (function (Flag) {
        /** No button or un-initialized */
        Flag[Flag["None"] = 0] = "None";
        /** Primary button (usually left) */
        Flag[Flag["Primary"] = 1] = "Primary";
        /** Secondary button (usually right) */
        Flag[Flag["Secondary"] = 2] = "Secondary";
        /** Auxilary button (usually middle or mouse wheel button)  */
        Flag[Flag["Auxilary"] = 4] = "Auxilary";
        /** 4th button (typically the "Browser Back" button) */
        Flag[Flag["Forth"] = 8] = "Forth";
        /** 5th button (typically the "Browser Forward" button) */
        Flag[Flag["Five"] = 16] = "Five";
        /** Trigger button */
        Flag[Flag["Trigger"] = 32] = "Trigger";
        /** Squeeze button */
        Flag[Flag["Squeeze"] = 48] = "Squeeze";
    })(Flag = ButtonsType.Flag || (ButtonsType.Flag = {}));
})(ButtonsType || (ButtonsType = {}));
export const EmptyKeyInput = {
    key: '',
    code: '',
    modifiers: ModifiersKeys.None,
    x: -1,
    y: -1,
    pageX: -1,
    pageY: -1,
    preventDefault: noop,
};
var DraggingState;
(function (DraggingState) {
    DraggingState[DraggingState["Stopped"] = 0] = "Stopped";
    DraggingState[DraggingState["Started"] = 1] = "Started";
    DraggingState[DraggingState["Moving"] = 2] = "Moving";
})(DraggingState || (DraggingState = {}));
function createEvents() {
    return {
        drag: new Subject(),
        interactionEnd: new Subject(),
        click: new Subject(),
        move: new Subject(),
        wheel: new Subject(),
        pinch: new Subject(),
        gesture: new Subject(),
        resize: new Subject(),
        leave: new Subject(),
        enter: new Subject(),
        modifiers: new Subject(),
        key: new Subject(),
        keyUp: new Subject(),
        keyDown: new Subject(),
        lock: new Subject(),
        trackedPointers: new Subject(),
    };
}
const AllowedNonPrintableKeys = ['Backspace', 'Delete'];
var InputObserver;
(function (InputObserver) {
    function create(props = {}) {
        const { noScroll, noContextMenu } = { ...DefaultInputObserverProps, ...props };
        return {
            noScroll,
            noContextMenu,
            pointerLock: false,
            width: 0,
            height: 0,
            pixelRatio: 1,
            ...createEvents(),
            setPixelScale: noop,
            updateTrackedPointers: noop,
            updateScreenTouches: noop,
            requestPointerLock: noop,
            exitPointerLock: noop,
            dispose: noop
        };
    }
    InputObserver.create = create;
    function fromElement(element, props = {}) {
        let { noScroll, noMiddleClickScroll, noContextMenu, noPinchZoom, noTextSelect, mask, pixelScale, preventGestures } = { ...DefaultInputObserverProps, ...props };
        let width = element.clientWidth * pixelRatio();
        let height = element.clientHeight * pixelRatio();
        let isLocked = false;
        let lockedViewport = Viewport();
        const pointerDown = Vec2();
        const pointerStart = Vec2();
        const pointerEnd = Vec2();
        const pointerDelta = Vec2();
        const rectSize = Vec2();
        const modifierKeys = {
            shift: false,
            alt: false,
            control: false,
            meta: false
        };
        const position = {
            x: -1,
            y: -1,
            pageX: -1,
            pageY: -1,
        };
        function pixelRatio() {
            return window.devicePixelRatio * pixelScale;
        }
        function getModifierKeys() {
            return { ...modifierKeys };
        }
        function getKeyOnElement(event) {
            return event.target === document.body || event.target === element;
        }
        let dragging = DraggingState.Stopped;
        let disposed = false;
        let buttons = ButtonsType.create(ButtonsType.Flag.None);
        let button = ButtonsType.Flag.None;
        let isInside = false;
        let hasMoved = false;
        let resizeObserver;
        if (typeof window.ResizeObserver !== 'undefined') {
            resizeObserver = new window.ResizeObserver(onResize);
        }
        const events = createEvents();
        const { drag, interactionEnd, wheel, pinch, gesture, click, move, leave, enter, resize, modifiers, key, keyUp, keyDown, lock, trackedPointers } = events;
        attach();
        function attach() {
            element.addEventListener('contextmenu', onContextMenu, false);
            element.addEventListener('wheel', onMouseWheel, false);
            element.addEventListener('mousedown', onMouseDown, false);
            // for dragging to work outside canvas bounds,
            // mouse move/up events have to be added to a parent, i.e. window
            window.addEventListener('mousemove', onMouseMove, false);
            window.addEventListener('mouseup', onMouseUp, false);
            element.addEventListener('touchstart', onTouchStart, false);
            element.addEventListener('touchmove', onTouchMove, false);
            element.addEventListener('touchend', onTouchEnd, false);
            element.addEventListener('gesturechange', onGestureChange, false);
            element.addEventListener('gesturestart', onGestureStart, false);
            element.addEventListener('gestureend', onGestureEnd, false);
            // reset buttons and modifier keys state when browser window looses focus
            window.addEventListener('blur', handleBlur);
            window.addEventListener('keyup', handleKeyUp, false);
            window.addEventListener('keydown', handleKeyDown, false);
            window.addEventListener('keypress', handleKeyPress, false);
            document.addEventListener('pointerlockchange', onPointerLockChange, false);
            document.addEventListener('pointerlockerror', onPointerLockError, false);
            if (resizeObserver != null) {
                resizeObserver.observe(element.parentElement);
            }
            else {
                window.addEventListener('resize', onResize, false);
            }
        }
        function dispose() {
            if (disposed)
                return;
            disposed = true;
            element.removeEventListener('contextmenu', onContextMenu, false);
            element.removeEventListener('wheel', onMouseWheel, false);
            element.removeEventListener('mousedown', onMouseDown, false);
            window.removeEventListener('mousemove', onMouseMove, false);
            window.removeEventListener('mouseup', onMouseUp, false);
            element.removeEventListener('touchstart', onTouchStart, false);
            element.removeEventListener('touchmove', onTouchMove, false);
            element.removeEventListener('touchend', onTouchEnd, false);
            element.removeEventListener('gesturechange', onGestureChange, false);
            element.removeEventListener('gesturestart', onGestureStart, false);
            element.removeEventListener('gestureend', onGestureEnd, false);
            window.removeEventListener('blur', handleBlur);
            window.removeEventListener('keyup', handleKeyUp, false);
            window.removeEventListener('keydown', handleKeyDown, false);
            window.removeEventListener('keypress', handleKeyPress, false);
            document.removeEventListener('pointerlockchange', onPointerLockChange, false);
            document.removeEventListener('pointerlockerror', onPointerLockError, false);
            cross.remove();
            if (resizeObserver != null) {
                resizeObserver.unobserve(element.parentElement);
                resizeObserver.disconnect();
            }
            else {
                window.removeEventListener('resize', onResize, false);
            }
        }
        function onPointerLockChange() {
            if (element.ownerDocument.pointerLockElement === element) {
                isLocked = true;
            }
            else {
                isLocked = false;
            }
            toggleCross(isLocked);
            lock.next(isLocked);
        }
        function onPointerLockError() {
            console.error('Unable to use Pointer Lock API');
            isLocked = false;
            toggleCross(isLocked);
            lock.next(isLocked);
        }
        function onContextMenu(event) {
            if (!mask(event.clientX, event.clientY))
                return;
            if (noContextMenu) {
                event.preventDefault();
            }
        }
        function updateModifierKeys(event) {
            modifierKeys.alt = event.altKey;
            modifierKeys.shift = event.shiftKey;
            modifierKeys.control = event.ctrlKey;
            modifierKeys.meta = event.metaKey;
        }
        function handleBlur() {
            if (buttons || modifierKeys.shift || modifierKeys.alt || modifierKeys.meta || modifierKeys.control) {
                buttons = 0;
                modifierKeys.shift = modifierKeys.alt = modifierKeys.control = modifierKeys.meta = false;
            }
        }
        function handleKeyDown(event) {
            let changed = false;
            if (!modifierKeys.alt && event.altKey) {
                changed = true;
                modifierKeys.alt = true;
            }
            if (!modifierKeys.shift && event.shiftKey) {
                changed = true;
                modifierKeys.shift = true;
            }
            if (!modifierKeys.control && event.ctrlKey) {
                changed = true;
                modifierKeys.control = true;
            }
            if (!modifierKeys.meta && event.metaKey) {
                changed = true;
                modifierKeys.meta = true;
            }
            if (changed && isInside)
                modifiers.next(getModifierKeys());
            if (getKeyOnElement(event) && isInside) {
                keyDown.next({
                    key: event.key,
                    code: event.code,
                    modifiers: getModifierKeys(),
                    ...position,
                    preventDefault: () => event.preventDefault(),
                });
            }
        }
        function handleKeyUp(event) {
            let changed = false;
            if (modifierKeys.alt && !event.altKey) {
                changed = true;
                modifierKeys.alt = false;
            }
            if (modifierKeys.shift && !event.shiftKey) {
                changed = true;
                modifierKeys.shift = false;
            }
            if (modifierKeys.control && !event.ctrlKey) {
                changed = true;
                modifierKeys.control = false;
            }
            if (modifierKeys.meta && !event.metaKey) {
                changed = true;
                modifierKeys.meta = false;
            }
            if (changed && isInside)
                modifiers.next(getModifierKeys());
            if (AllowedNonPrintableKeys.includes(event.key))
                handleKeyPress(event);
            if (getKeyOnElement(event) && isInside) {
                keyUp.next({
                    key: event.key,
                    code: event.code,
                    modifiers: getModifierKeys(),
                    ...position,
                    preventDefault: () => event.preventDefault(),
                });
            }
        }
        function handleKeyPress(event) {
            if (!getKeyOnElement(event) || !isInside)
                return;
            key.next({
                key: event.key,
                code: event.code,
                modifiers: getModifierKeys(),
                ...position,
                preventDefault: () => event.preventDefault(),
            });
        }
        const trackedPointerState = {
            primary: { prev: undefined, down: Vec2(), end: Vec2(), axes: [] },
            secondary: { prev: undefined, down: Vec2(), end: Vec2(), axes: [] },
            distance: -1,
        };
        function buttonUp(flag, buttons, prevButtons) {
            const pressed = ButtonsType.has(buttons, flag);
            const prevPressed = prevButtons ? ButtonsType.has(prevButtons, flag) : false;
            return prevPressed && !pressed;
        }
        function handleTrackedPointer(trackedPointer, isSecondary) {
            const { x, y, dx, dy, ray, buttons, axes } = trackedPointer;
            const [pageX, pageY] = [x, y];
            const modifiers = ModifiersKeys.create();
            const useDelta = true;
            const state = isSecondary ? trackedPointerState.secondary : trackedPointerState.primary;
            const { prev } = state;
            if (buttons === ButtonsType.Flag.Secondary) {
                move.next({ x, y, pageX, pageY, buttons: ButtonsType.Flag.None, button: ButtonsType.Flag.None, modifiers, inside: true, onElement: true, ray });
            }
            else if ((prev === null || prev === void 0 ? void 0 : prev.buttons) === ButtonsType.Flag.Secondary) {
                leave.next(undefined);
            }
            const pressed = ButtonsType.has(buttons, ButtonsType.Flag.Primary);
            const prevPressed = prev ? ButtonsType.has(prev.buttons, ButtonsType.Flag.Primary) : false;
            if (prevPressed && !pressed) {
                Vec2.set(state.end, x, y);
                if (Vec2.distance(state.end, state.down) < 20) {
                    click.next({ x, y, pageX, pageY, buttons: ButtonsType.Flag.Trigger, button: ButtonsType.Flag.Trigger, modifiers, ray });
                }
                interactionEnd.next(undefined);
            }
            if (pressed) {
                const isStart = !prevPressed;
                drag.next({ x, y, dx, dy, pageX, pageY, buttons: ButtonsType.Flag.Trigger, button: ButtonsType.Flag.Trigger, modifiers, isStart, useDelta });
                if (isStart) {
                    Vec2.set(state.down, x, y);
                }
            }
            const keyValue = { x: 0, y: 0, pageX: 0, pageY: 0, modifiers, preventDefault: () => { } };
            if (buttonUp(ButtonsType.Flag.Forth, buttons, prev === null || prev === void 0 ? void 0 : prev.buttons)) {
                const forth = isSecondary ? 'GamepadX' : 'GamepadA';
                keyUp.next({ ...keyValue, code: forth, key: forth });
            }
            if (buttonUp(ButtonsType.Flag.Five, buttons, prev === null || prev === void 0 ? void 0 : prev.buttons)) {
                const five = isSecondary ? 'GamepadY' : 'GamepadB';
                keyUp.next({ ...keyValue, code: five, key: five });
            }
            if (axes) {
                if (axes[2] < 0) {
                    keyDown.next({ ...keyValue, code: 'GamepadLeft', key: 'GamepadLeft' });
                    keyUp.next({ ...keyValue, code: 'GamepadRight', key: 'GamepadRight' });
                }
                else if (axes[2] > 0) {
                    keyDown.next({ ...keyValue, code: 'GamepadRight', key: 'GamepadRight' });
                    keyUp.next({ ...keyValue, code: 'GamepadLeft', key: 'GamepadLeft' });
                }
                else if (axes[2] !== state.axes[2]) {
                    keyUp.next({ ...keyValue, code: 'GamepadLeft', key: 'GamepadLeft' });
                    keyUp.next({ ...keyValue, code: 'GamepadRight', key: 'GamepadRight' });
                }
                if (axes[3] < 0) {
                    keyDown.next({ ...keyValue, code: 'GamepadUp', key: 'GamepadUp' });
                    keyUp.next({ ...keyValue, code: 'GamepadDown', key: 'GamepadDown' });
                }
                else if (axes[3] > 0) {
                    keyDown.next({ ...keyValue, code: 'GamepadDown', key: 'GamepadDown' });
                    keyUp.next({ ...keyValue, code: 'GamepadUp', key: 'GamepadUp' });
                }
                else if (axes[3] !== state.axes[3]) {
                    keyUp.next({ ...keyValue, code: 'GamepadUp', key: 'GamepadUp' });
                    keyUp.next({ ...keyValue, code: 'GamepadDown', key: 'GamepadDown' });
                }
            }
            state.axes = axes || [];
            state.prev = trackedPointer;
        }
        function handleTrackedPointerGesture(primary, left, button) {
            const d = Vec3.distance(primary.ray.origin, left.ray.origin);
            if (trackedPointerState.distance > 0) {
                const f = d / trackedPointerState.distance;
                gesture.next({
                    scale: f, rotation: 0, deltaScale: 0, deltaRotation: 0,
                    buttons: button, button, modifiers: getModifierKeys()
                });
            }
            trackedPointerState.distance = d;
        }
        function handleTrackedPointers(trackedPointers) {
            let primary;
            let secondary;
            for (const tp of trackedPointers) {
                if (tp.handedness === 'right') {
                    primary = tp;
                }
                else if (tp.handedness === 'left') {
                    secondary = tp;
                }
            }
            if (!primary && secondary) {
                primary = secondary;
                secondary = undefined;
            }
            if ((primary === null || primary === void 0 ? void 0 : primary.buttons) === ButtonsType.Flag.Primary && (secondary === null || secondary === void 0 ? void 0 : secondary.buttons) === ButtonsType.Flag.Primary) {
                handleTrackedPointerGesture(primary, secondary, ButtonsType.Flag.Trigger);
            }
            else if ((primary === null || primary === void 0 ? void 0 : primary.buttons) === ButtonsType.Flag.Secondary && (secondary === null || secondary === void 0 ? void 0 : secondary.buttons) === ButtonsType.Flag.Secondary) {
                handleTrackedPointerGesture(primary, secondary, ButtonsType.Flag.Squeeze);
            }
            else {
                if (trackedPointerState.distance === -1) {
                    if (primary)
                        handleTrackedPointer(primary, false);
                    if (secondary)
                        handleTrackedPointer(secondary, true);
                }
                // end gesture only if all button are released
                if (!(primary === null || primary === void 0 ? void 0 : primary.buttons) && !(secondary === null || secondary === void 0 ? void 0 : secondary.buttons)) {
                    trackedPointerState.distance = -1;
                }
            }
        }
        let screenTouchPrev = undefined;
        let screenTouchStart = undefined;
        let screenTouchDistance = -1;
        function handleScreenTouches(screenTouches) {
            if (screenTouches.length === 2) {
                const a = Vec2.fromObj(screenTouches[0]);
                const b = Vec2.fromObj(screenTouches[1]);
                const d = Vec2.distance(a, b);
                const button = ButtonsType.Flag.Trigger;
                if (screenTouchDistance > 0) {
                    const f = d / screenTouchDistance;
                    gesture.next({
                        scale: f, rotation: 0, deltaScale: 0, deltaRotation: 0,
                        buttons: button, button, modifiers: getModifierKeys()
                    });
                }
                screenTouchDistance = d;
                screenTouchPrev = undefined;
                return;
            }
            screenTouchDistance = -1;
            const t = screenTouches[0];
            if (!t) {
                if (screenTouchStart && screenTouchPrev) {
                    const a = Vec2.fromObj(screenTouchStart);
                    const b = Vec2.fromObj(screenTouchPrev);
                    if (Vec2.distance(a, b) < 10) {
                        const [x, y] = a;
                        const [pageX, pageY] = [x, y];
                        const modifiers = ModifiersKeys.create();
                        const button = ButtonsType.Flag.Trigger;
                        const { ray } = screenTouchStart;
                        click.next({ x, y, pageX, pageY, buttons: button, button, modifiers, ray });
                    }
                    interactionEnd.next(undefined);
                }
                screenTouchPrev = undefined;
                screenTouchStart = undefined;
                return;
            }
            const p = Vec2.fromObj(t);
            const [x, y] = p;
            const [pageX, pageY] = [x, y];
            const modifiers = ModifiersKeys.create();
            const button = ButtonsType.Flag.Trigger;
            const useDelta = true;
            const isStart = screenTouchPrev === undefined;
            const dx = screenTouchPrev ? x - screenTouchPrev.x : 0;
            const dy = screenTouchPrev ? y - screenTouchPrev.y : 0;
            screenTouchPrev = t;
            if (isStart)
                screenTouchStart = t;
            drag.next({ x, y, dx, dy, pageX, pageY, buttons: button, button, modifiers, isStart, useDelta });
        }
        function getCenterTouch(ev) {
            const t0 = ev.touches[0];
            const t1 = ev.touches[1];
            return {
                clientX: (t0.clientX + t1.clientX) / 2,
                clientY: (t0.clientY + t1.clientY) / 2,
                pageX: (t0.pageX + t1.pageX) / 2,
                pageY: (t0.pageY + t1.pageY) / 2,
                target: ev.target
            };
        }
        function getTouchDistance(ev) {
            const dx = ev.touches[0].pageX - ev.touches[1].pageX;
            const dy = ev.touches[0].pageY - ev.touches[1].pageY;
            return Math.sqrt(dx * dx + dy * dy);
        }
        let singleTouchDistance = -1;
        let lastSingleTouch = undefined;
        const singleTouchPosition = Vec2(), singleTouchTmp = Vec2();
        function updateSingleTouchDistance(ev) {
            if (singleTouchDistance < 0)
                return;
            Vec2.set(singleTouchTmp, ev.touches[0].pageX, ev.touches[0].pageY);
            singleTouchDistance += Vec2.distance(singleTouchPosition, singleTouchTmp);
            Vec2.copy(singleTouchPosition, singleTouchTmp);
        }
        const firstTouchStart = Vec2();
        let firstTouchStartSet = false;
        let initialTouchDistance = 0, lastTouchFraction = 1;
        function onTouchStart(ev) {
            ev.preventDefault();
            lastSingleTouch = undefined;
            singleTouchDistance = -1;
            if (ev.touches.length === 1) {
                buttons = button = ButtonsType.Flag.Primary;
                singleTouchDistance = 0;
                Vec2.set(singleTouchPosition, ev.touches[0].pageX, ev.touches[0].pageY);
                lastSingleTouch = ev.touches[0];
                onPointerDown(ev.touches[0]);
                Vec2.copy(firstTouchStart, pointerStart);
                firstTouchStartSet = true;
            }
            else if (ev.touches.length === 2) {
                buttons = ButtonsType.Flag.Secondary | ButtonsType.Flag.Auxilary;
                button = ButtonsType.Flag.Secondary;
                updateModifierKeys(ev);
                lastTouchFraction = 1;
                initialTouchDistance = getTouchDistance(ev);
                const { pageX: centerPageX, pageY: centerPageY } = getPagePosition(getCenterTouch(ev));
                if (!firstTouchStartSet) {
                    eventOffset(firstTouchStart, getCenterTouch(ev));
                    firstTouchStartSet = true;
                }
                pinch.next({
                    isStart: true,
                    distance: initialTouchDistance,
                    delta: 0,
                    fraction: lastTouchFraction,
                    fractionDelta: 0,
                    startX: firstTouchStart[0],
                    startY: firstTouchStart[1],
                    centerPageX,
                    centerPageY,
                    buttons,
                    button,
                    modifiers: getModifierKeys()
                });
            }
            else if (ev.touches.length === 3) {
                buttons = button = ButtonsType.Flag.Forth;
                onPointerDown(getCenterTouch(ev));
            }
        }
        function onTouchEnd(ev) {
            endDrag();
            if (lastSingleTouch && singleTouchDistance <= 4) {
                const t = lastSingleTouch;
                if (!mask(t.clientX, t.clientY))
                    return;
                eventOffset(singleTouchTmp, t);
                const { pageX, pageY } = getPagePosition(t);
                const [x, y] = singleTouchTmp;
                click.next({ x, y, pageX, pageY, buttons, button, modifiers: getModifierKeys() });
            }
            lastSingleTouch = undefined;
            firstTouchStartSet = false;
        }
        function onTouchMove(ev) {
            button = ButtonsType.Flag.None;
            if (noPinchZoom) {
                ev.preventDefault();
                ev.stopPropagation();
                if (ev.originalEvent) {
                    ev.originalEvent.preventDefault();
                    ev.originalEvent.stopPropagation();
                }
            }
            lastSingleTouch = undefined;
            if (ev.touches.length === 1) {
                buttons = ButtonsType.Flag.Primary;
                lastSingleTouch = ev.touches[0];
                updateSingleTouchDistance(ev);
                onPointerMove(ev.touches[0]);
            }
            else if (ev.touches.length === 2) {
                buttons = ButtonsType.Flag.Secondary | ButtonsType.Flag.Auxilary;
                button = ButtonsType.Flag.Secondary;
                updateModifierKeys(ev);
                const { pageX: centerPageX, pageY: centerPageY } = getPagePosition(getCenterTouch(ev));
                const distance = getTouchDistance(ev);
                const delta = initialTouchDistance - distance;
                const fraction = initialTouchDistance / distance;
                const fractionDelta = fraction - lastTouchFraction;
                lastTouchFraction = fraction;
                pinch.next({
                    isStart: false,
                    distance,
                    delta,
                    fraction,
                    fractionDelta,
                    startX: firstTouchStart[0],
                    startY: firstTouchStart[1],
                    centerPageX,
                    centerPageY,
                    buttons,
                    button,
                    modifiers: getModifierKeys()
                });
            }
            else if (ev.touches.length === 3) {
                buttons = ButtonsType.Flag.Forth;
                onPointerMove(getCenterTouch(ev));
            }
        }
        function onMouseDown(ev) {
            updateModifierKeys(ev);
            buttons = getButtons(ev);
            button = getButton(ev);
            if (noMiddleClickScroll && buttons === ButtonsType.Flag.Auxilary) {
                ev.preventDefault;
            }
            onPointerDown(ev);
        }
        function onMouseMove(ev) {
            updateModifierKeys(ev);
            buttons = getButtons(ev);
            button = ButtonsType.Flag.None;
            onPointerMove(ev);
        }
        function onMouseUp(ev) {
            updateModifierKeys(ev);
            buttons = getButtons(ev);
            button = getButton(ev);
            onPointerUp(ev);
            endDrag();
        }
        function endDrag() {
            interactionEnd.next(void 0);
        }
        function onPointerDown(ev) {
            if (!mask(ev.clientX, ev.clientY))
                return;
            eventOffset(pointerStart, ev);
            Vec2.copy(pointerDown, pointerStart);
            if (insideBounds(pointerStart)) {
                dragging = DraggingState.Started;
            }
        }
        function onPointerUp(ev) {
            dragging = DraggingState.Stopped;
            if (!mask(ev.clientX, ev.clientY))
                return;
            eventOffset(pointerEnd, ev);
            if (!hasMoved && Vec2.distance(pointerEnd, pointerDown) < 4) {
                const { pageX, pageY } = getPagePosition(ev);
                const [x, y] = pointerEnd;
                click.next({ x, y, pageX, pageY, buttons, button, modifiers: getModifierKeys() });
            }
            hasMoved = false;
        }
        function onPointerMove(ev) {
            var _a;
            eventOffset(pointerEnd, ev);
            const { pageX, pageY } = getPagePosition(ev);
            const [x, y] = pointerEnd;
            const { movementX, movementY } = ev;
            const inside = insideBounds(pointerEnd) && mask(ev.clientX, ev.clientY);
            if (isInside && !inside) {
                leave.next(void 0);
            }
            else if (!isInside && inside) {
                enter.next(void 0);
            }
            isInside = inside;
            position.x = x;
            position.y = y;
            position.pageX = pageX;
            position.pageY = pageY;
            move.next({ x, y, pageX, pageY, movementX, movementY, buttons, button, modifiers: getModifierKeys(), inside, onElement: ev.target === element });
            if (dragging === DraggingState.Stopped)
                return;
            if (noTextSelect) {
                (_a = ev.preventDefault) === null || _a === void 0 ? void 0 : _a.call(ev);
            }
            Vec2.div(pointerDelta, Vec2.sub(pointerDelta, pointerEnd, pointerStart), getClientSize(rectSize));
            if (Vec2.magnitude(pointerDelta) < EPSILON)
                return;
            const isStart = dragging === DraggingState.Started;
            if (isStart && !mask(ev.clientX, ev.clientY))
                return;
            if (Vec2.distance(pointerEnd, pointerDown) >= 4) {
                hasMoved = true;
            }
            const [dx, dy] = pointerDelta;
            drag.next({ x, y, dx, dy, pageX, pageY, buttons, button, modifiers: getModifierKeys(), isStart });
            Vec2.copy(pointerStart, pointerEnd);
            dragging = DraggingState.Moving;
        }
        function onMouseWheel(ev) {
            if (!mask(ev.clientX, ev.clientY))
                return;
            eventOffset(pointerEnd, ev);
            const { pageX, pageY } = getPagePosition(ev);
            const [x, y] = pointerEnd;
            if (noScroll) {
                ev.preventDefault();
            }
            const normalized = normalizeWheel(ev);
            buttons = button = ButtonsType.Flag.Auxilary;
            if (normalized.dx || normalized.dy || normalized.dz) {
                wheel.next({ x, y, pageX, pageY, ...normalized, buttons, button, modifiers: getModifierKeys() });
            }
        }
        function tryPreventGesture(ev) {
            var _a, _b;
            // console.log(ev, preventGestures);
            if (!preventGestures)
                return;
            ev.preventDefault();
            (_a = ev.stopImmediatePropagation) === null || _a === void 0 ? void 0 : _a.call(ev);
            (_b = ev.stopPropagation) === null || _b === void 0 ? void 0 : _b.call(ev);
        }
        let prevGestureScale = 0, prevGestureRotation = 0;
        function onGestureStart(ev) {
            tryPreventGesture(ev);
            prevGestureScale = ev.scale;
            prevGestureRotation = ev.rotation;
            gesture.next({ scale: ev.scale, rotation: ev.rotation, deltaRotation: 0, deltaScale: 0, isStart: true, buttons, button, modifiers: getModifierKeys() });
        }
        function gestureDelta(ev, isEnd) {
            gesture.next({
                scale: ev.scale,
                rotation: ev.rotation,
                deltaRotation: prevGestureRotation - ev.rotation,
                deltaScale: prevGestureScale - ev.scale,
                isEnd,
                buttons,
                button,
                modifiers: getModifierKeys()
            });
            prevGestureRotation = ev.rotation;
            prevGestureScale = ev.scale;
        }
        function onGestureChange(ev) {
            tryPreventGesture(ev);
            gestureDelta(ev);
        }
        function onGestureEnd(ev) {
            tryPreventGesture(ev);
            gestureDelta(ev, true);
        }
        function onResize() {
            width = element.clientWidth * pixelRatio();
            height = element.clientHeight * pixelRatio();
            resize.next({});
        }
        function insideBounds(pos) {
            if (element instanceof Window || element instanceof Document || element === document.body) {
                return true;
            }
            else {
                const rect = element.getBoundingClientRect();
                return pos[0] >= 0 && pos[1] >= 0 && pos[0] < rect.width && pos[1] < rect.height;
            }
        }
        function getClientSize(out) {
            out[0] = element.clientWidth;
            out[1] = element.clientHeight;
            return out;
        }
        function eventOffset(out, ev) {
            width = element.clientWidth * pixelRatio();
            height = element.clientHeight * pixelRatio();
            if (isLocked) {
                const pr = pixelRatio();
                out[0] = (lockedViewport.x + lockedViewport.width / 2) / pr;
                out[1] = (height - (lockedViewport.y + lockedViewport.height / 2)) / pr;
            }
            else {
                const rect = element.getBoundingClientRect();
                out[0] = (ev.clientX || 0) - rect.left;
                out[1] = (ev.clientY || 0) - rect.top;
            }
            return out;
        }
        function getPagePosition(ev) {
            if (isLocked) {
                return {
                    pageX: Math.round(window.innerWidth / 2) + lockedViewport.x,
                    pageY: Math.round(window.innerHeight / 2) + lockedViewport.y
                };
            }
            else {
                return {
                    pageX: ev.pageX,
                    pageY: ev.pageY
                };
            }
        }
        const crossWidth = 30;
        const cross = addCross();
        function addCross() {
            var _a;
            const cross = document.createElement('div');
            const b = '30%';
            const t = '10%';
            const c = `#000 ${b}, #0000 0 calc(100% - ${b}), #000 0`;
            const vline = `linear-gradient(0deg, ${c}) 50%/${t} 100% no-repeat`;
            const hline = `linear-gradient(90deg, ${c}) 50%/100% ${t} no-repeat`;
            const cdot = 'radial-gradient(circle at 50%, #000 5%, #0000 5%)';
            Object.assign(cross.style, {
                width: `${crossWidth}px`,
                aspectRatio: 1,
                background: `${vline}, ${hline}, ${cdot}`,
                display: 'none',
                zIndex: 1000,
                position: 'absolute',
                mixBlendMode: 'difference',
                filter: 'invert(1)',
            });
            (_a = element.parentElement) === null || _a === void 0 ? void 0 : _a.appendChild(cross);
            return cross;
        }
        function toggleCross(value) {
            cross.style.display = value ? 'block' : 'none';
            if (value) {
                const pr = pixelRatio();
                const offsetX = (lockedViewport.x + lockedViewport.width / 2) / pr;
                const offsetY = (lockedViewport.y + lockedViewport.height / 2) / pr;
                cross.style.width = `${crossWidth}px`;
                cross.style.left = `calc(${offsetX}px - ${crossWidth / 2}px)`;
                cross.style.bottom = `calc(${offsetY}px - ${crossWidth / 2}px)`;
            }
        }
        return {
            get noScroll() { return noScroll; },
            set noScroll(value) { noScroll = value; },
            get noContextMenu() { return noContextMenu; },
            set noContextMenu(value) { noContextMenu = value; },
            get width() { return width; },
            get height() { return height; },
            get pixelRatio() { return pixelRatio(); },
            get pointerLock() { return isLocked; },
            ...events,
            setPixelScale: (value) => {
                pixelScale = value;
                width = element.clientWidth * pixelRatio();
                height = element.clientHeight * pixelRatio();
            },
            updateTrackedPointers: (input) => {
                handleTrackedPointers(input);
                trackedPointers.next(input);
            },
            updateScreenTouches: (input) => {
                handleScreenTouches(input);
            },
            requestPointerLock: (viewport) => {
                lockedViewport = viewport;
                if (!isLocked) {
                    element.requestPointerLock();
                }
            },
            exitPointerLock: () => {
                if (isLocked) {
                    element.ownerDocument.exitPointerLock();
                }
            },
            dispose
        };
    }
    InputObserver.fromElement = fromElement;
})(InputObserver || (InputObserver = {}));
// Adapted from https://stackoverflow.com/a/30134826
// License: https://creativecommons.org/licenses/by-sa/3.0/
export function normalizeWheel(event) {
    // Reasonable defaults
    const PIXEL_STEP = 10;
    const LINE_HEIGHT = 40;
    const PAGE_HEIGHT = 800;
    let spinX = 0, spinY = 0, dx = 0, dy = 0, dz = 0; // pixelX, pixelY, pixelZ
    // Legacy
    if ('detail' in event) {
        spinY = event.detail;
    }
    if ('wheelDelta' in event) {
        spinY = -event.wheelDelta / 120;
    }
    if ('wheelDeltaY' in event) {
        spinY = -event.wheelDeltaY / 120;
    }
    if ('wheelDeltaX' in event) {
        spinX = -event.wheelDeltaX / 120;
    }
    // side scrolling on FF with DOMMouseScroll
    if ('axis' in event && event.axis === event.HORIZONTAL_AXIS) {
        spinX = spinY;
        spinY = 0;
    }
    dx = spinX * PIXEL_STEP;
    dy = spinY * PIXEL_STEP;
    if ('deltaY' in event) {
        dy = event.deltaY;
    }
    if ('deltaX' in event) {
        dx = event.deltaX;
    }
    if ('deltaZ' in event) {
        dz = event.deltaZ;
    }
    if ((dx || dy || dz) && event.deltaMode) {
        if (event.deltaMode === 1) { // delta in LINE units
            dx *= LINE_HEIGHT;
            dy *= LINE_HEIGHT;
            dz *= LINE_HEIGHT;
        }
        else { // delta in PAGE units
            dx *= PAGE_HEIGHT;
            dy *= PAGE_HEIGHT;
            dz *= PAGE_HEIGHT;
        }
    }
    // Fall-back if spin cannot be determined
    if (dx && !spinX) {
        spinX = (dx < 0) ? -1 : 1;
    }
    if (dy && !spinY) {
        spinY = (dy < 0) ? -1 : 1;
    }
    return { spinX, spinY, dx, dy, dz };
}
export { InputObserver };
