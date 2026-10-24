// The drive and folder the assistant works in, read from the app's current
// state at the moment a request is sent.
import { state } from '../../state';
import { t } from '../../i18n/i18n';
import type { Scope } from './ai_api';

/** The drive's display name: "My Drive" for the personal drive. */
export function drive_name(): string {
    const channel = state.activeChannel;
    if (!channel || channel.kind === 'personal') return t('sidebar.my_drive');
    return channel.title;
}

/** Scope for a request: the open drive, and the open folder unless the user chose the drive root. */
export function build_scope(include_folder: boolean): Scope {
    return {
        channel_id: state.activeChannel?.id ?? 0,
        folder_id: include_folder ? state.currentFolderId || '' : '',
    };
}
