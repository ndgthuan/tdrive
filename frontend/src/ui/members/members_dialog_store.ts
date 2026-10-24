import { createModalController } from '../modals/modal_store';

export interface MembersDialogPayload {
    driveId: number;
    title: string;
}

/** The Members dialog for one shared drive. Business logic lives in members_store.ts. */
export const members_dialog = createModalController<MembersDialogPayload>();
