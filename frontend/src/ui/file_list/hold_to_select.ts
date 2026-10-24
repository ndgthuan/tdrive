// Desktop press and hold on a row: after HOLD_MS the row is handed to the
// caller, which enters selection mode with it. A press that moves past the slop
// or lets go early is a plain click and does nothing here. While the press is
// held a thin ring grows around the row's icon, so the wait reads as progress.
//
// Only the desktop list binds this. The phone keeps its own 350 ms long press in
// touch.ts, which toggles a row rather than starting a mode.

export const HOLD_MS = 400;
export const HOLD_SLOP_PX = 6;
// The click the browser fires on release arrives right after the press that
// fired the hold. Within this window it is swallowed, so the row does not also
// open or select.
const HOLD_CLICK_GUARD_MS = 700;
const RING_RADIUS = 18;
const RING_CIRCUMFERENCE = 2 * Math.PI * RING_RADIUS;
const ICON_SELECTOR = '.folder-chip, .file-type-icon, .row-thumbnail';
const SVG_NS = 'http://www.w3.org/2000/svg';

export interface HoldToSelectOptions {
    /** Whether a hold may start on this row (the trash, for one, may not). */
    canStart: (row: HTMLElement) => boolean;
    /** Called once the hold completes, with the row under the pointer. */
    onHold: (row: HTMLElement) => void;
}

function make_ring(row: HTMLElement): SVGSVGElement | null {
    const icon = row.querySelector<HTMLElement>(ICON_SELECTOR);
    if (!icon) return null;
    const rowBox = row.getBoundingClientRect();
    const iconBox = icon.getBoundingClientRect();
    const size = 40;
    const ring = document.createElementNS(SVG_NS, 'svg');
    ring.setAttribute('class', 'hold-ring');
    ring.setAttribute('width', String(size));
    ring.setAttribute('height', String(size));
    ring.setAttribute('viewBox', `0 0 ${size} ${size}`);
    ring.setAttribute('aria-hidden', 'true');
    // Centred on the icon: the icon's box is 32px, the ring 40px.
    ring.style.left = `${iconBox.left - rowBox.left + iconBox.width / 2 - size / 2}px`;
    ring.style.top = `${iconBox.top - rowBox.top + iconBox.height / 2 - size / 2}px`;
    const track = document.createElementNS(SVG_NS, 'circle');
    track.setAttribute('class', 'hold-ring-track');
    track.setAttribute('cx', String(size / 2));
    track.setAttribute('cy', String(size / 2));
    track.setAttribute('r', String(RING_RADIUS));
    const bar = document.createElementNS(SVG_NS, 'circle');
    bar.setAttribute('class', 'hold-ring-bar');
    bar.setAttribute('cx', String(size / 2));
    bar.setAttribute('cy', String(size / 2));
    bar.setAttribute('r', String(RING_RADIUS));
    bar.setAttribute('stroke-dasharray', String(RING_CIRCUMFERENCE));
    bar.setAttribute('stroke-dashoffset', String(RING_CIRCUMFERENCE));
    bar.setAttribute('transform', `rotate(-90 ${size / 2} ${size / 2})`);
    ring.append(track, bar);
    // Fill on the next frame, so the transition has a starting value to leave.
    requestAnimationFrame(() => {
        bar.style.transition = `stroke-dashoffset ${HOLD_MS}ms linear`;
        bar.setAttribute('stroke-dashoffset', '0');
    });
    return ring;
}

/**
 * Binds the desktop press and hold to every descendant of `host` matching
 * `selector`. Returns the cleanup.
 */
export function bind_hold_to_select(host: HTMLElement, selector: string, options: HoldToSelectOptions): () => void {
    let timer = 0;
    let row: HTMLElement | null = null;
    let pointerId = -1;
    let startX = 0;
    let startY = 0;
    let ring: SVGSVGElement | null = null;
    // The pointer whose press completed a hold. Its release is followed by the
    // click the guard below swallows, and its drags are cancelled. Both stay
    // armed until the next press, since a drag may end on a pointercancel
    // before the browser has finished starting it.
    let holding_fired = false;
    let holding_pointer = -1;
    let firedAt = 0;

    const remove_ring = () => {
        ring?.remove();
        ring = null;
    };
    const cancel = () => {
        window.clearTimeout(timer);
        timer = 0;
        row = null;
        pointerId = -1;
        remove_ring();
    };
    const on_pointer_down = (event: PointerEvent) => {
        if (event.button !== 0 || !event.isPrimary) return;
        const origin = event.target instanceof Element ? event.target : null;
        // Buttons keep their own clicks (the checkbox, the row actions).
        if (origin?.closest('button, input, textarea, select, a')) return;
        const target = origin?.closest<HTMLElement>(selector);
        if (!target || !options.canStart(target)) return;
        cancel();
        holding_fired = false;
        firedAt = 0;
        row = target;
        pointerId = event.pointerId;
        startX = event.clientX;
        startY = event.clientY;
        ring = make_ring(target);
        if (ring) target.append(ring);
        timer = window.setTimeout(() => {
            const pressed = row;
            const pid = pointerId;
            cancel();
            if (!pressed?.isConnected) return;
            holding_fired = true;
            holding_pointer = pid;
            options.onHold(pressed);
        }, HOLD_MS);
    };
    const on_pointer_move = (event: PointerEvent) => {
        if (event.pointerId !== pointerId || timer === 0) return;
        if (Math.hypot(event.clientX - startX, event.clientY - startY) > HOLD_SLOP_PX) cancel();
    };
    const on_pointer_end = (event: PointerEvent) => {
        if (event.pointerId === pointerId) cancel();
        if (holding_fired && event.pointerId === holding_pointer) {
            // The release is the last step of the hold; its click follows at once.
            firedAt = Date.now();
        }
    };
    const on_click = (event: MouseEvent) => {
        if (!firedAt || Date.now() - firedAt > HOLD_CLICK_GUARD_MS) return;
        firedAt = 0;
        event.preventDefault();
        event.stopPropagation();
    };
    const on_drag_start = (event: DragEvent) => {
        if (holding_fired) event.preventDefault();
    };

    host.addEventListener('pointerdown', on_pointer_down);
    host.addEventListener('pointermove', on_pointer_move);
    host.addEventListener('pointerup', on_pointer_end);
    host.addEventListener('pointercancel', on_pointer_end);
    host.addEventListener('click', on_click, true);
    host.addEventListener('dragstart', on_drag_start, true);
    return () => {
        cancel();
        host.removeEventListener('pointerdown', on_pointer_down);
        host.removeEventListener('pointermove', on_pointer_move);
        host.removeEventListener('pointerup', on_pointer_end);
        host.removeEventListener('pointercancel', on_pointer_end);
        host.removeEventListener('click', on_click, true);
        host.removeEventListener('dragstart', on_drag_start, true);
    };
}
