import { t } from '../i18n/i18n';
/**
 * Notification action constants that other modules may import without pulling
 * in the action dispatcher.
 *
 * notification_actions.ts depends on the photo backup controller, and the
 * photo backup background module needs PAUSE_BACKUP_ACTION to build its notice.
 * Keeping the constant here, in a module with no runtime imports, breaks that
 * cycle: a module that read the constant during evaluation used to see it still
 * uninitialised (a TDZ ReferenceError at startup under Vite's dev graph).
 */

import type { NotificationAction } from './android_foreground';

/** Pauses the photo backup queue, the way the panel's own Pause does. */
export const PAUSE_BACKUP_ACTION: NotificationAction = {
    id: 'backup:pause',
    // A getter, so the label follows the language at read time rather than at import.
    get label() {
        return t('messages.action_pause');
    },
};
