// Name-clash prompt: shown when an import holds a file whose name is taken on the
// drive by a different file. Every clash starts on skip. Resolves to the choice
// for each entry index, or null when the user cancels the whole import.

import {
    nameClashModal,
    type NameClashChoice,
    type NameClashItem,
} from '../../ui/modals/name_clash_modal_store';

export type { NameClashChoice, NameClashItem } from '../../ui/modals/name_clash_modal_store';

let pending: ((result: Map<number, NameClashChoice> | null) => void) | null = null;

function finish(result: Map<number, NameClashChoice> | null): void {
    nameClashModal.close();
    if (pending) {
        const resolve = pending;
        pending = null;
        resolve(result);
    }
}

export function cancelNameClash(): void {
    finish(null);
}

export function confirmNameClash(choices: Map<number, NameClashChoice>): void {
    finish(choices);
}

export function openNameClashModal(items: NameClashItem[]): Promise<Map<number, NameClashChoice> | null> {
    return new Promise((resolve) => {
        // A re-open while one is pending would strand the previous caller.
        if (pending) {
            const prev = pending;
            pending = null;
            prev(null);
        }
        pending = resolve;
        nameClashModal.open({ items });
    });
}
