import { writable } from 'svelte/store';

export interface SelectionBarState {
    count: number;
    active?: boolean;
}

export const selectionBarState = writable<SelectionBarState>({ count: 0 });

// Whether the desktop list is in selection mode: checkboxes on every row, the
// header count and the bottom bar. Set only by entering or leaving the mode, so
// a plain click that selects one row does not turn it on.
export const selectionModeOn = writable(false);

export function setSelectionCount(count: number): void {
    const normalized = Math.max(0, Math.trunc(count));
    selectionBarState.update((current) => ({
        count: normalized,
        active: Boolean(current.active || normalized > 0),
    }));
}

export function setSelectionModeActive(active: boolean): void {
    selectionModeOn.set(active);
    selectionBarState.update((current) => ({ ...current, active }));
}
