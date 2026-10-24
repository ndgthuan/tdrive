import { createModalController } from './modal_store';

export interface UploadOptionsPayload {
    count: number;
}

export const uploadOptionsModal = createModalController<UploadOptionsPayload>();
