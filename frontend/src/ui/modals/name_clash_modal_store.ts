import { createModalController } from './modal_store';

// What to do with one file that has the same name as a different file on the
// drive. skip sends nothing; keep_both uploads it as keptName; replace uploads it
// as keptName and then swaps it in for the old file, which goes to the trash.
export type NameClashChoice = 'replace' | 'keep_both' | 'skip';

export interface NameClashItem {
    // The entry's index in the import request, so the answer finds its file.
    index: number;
    name: string;
    incomingSize: number;
    existingSize: number;
    // The free copy name for this file, or "" when none is free.
    keptName: string;
}

export interface NameClashPayload {
    items: NameClashItem[];
}

export const nameClashModal = createModalController<NameClashPayload>();
