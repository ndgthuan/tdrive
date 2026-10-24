import { createModalController } from './modal_store';

export type LogoutMode = 'soft' | 'full';

export const logoutModal = createModalController<null>();
