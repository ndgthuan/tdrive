import { createModalController } from './modal_store';

export interface ShareDrivePayload {
    link: string;
    approvalRequired: boolean;
}

export const shareDriveModal = createModalController<ShareDrivePayload>();
