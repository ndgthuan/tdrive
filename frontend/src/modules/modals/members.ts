// Members and roles dialog for one shared drive. Opened from the drive's menu.
import { t } from '../../i18n/i18n';
import { load_drive_role, load_members } from '../../ui/members/members_store';
import { members_dialog } from '../../ui/members/members_dialog_store';
import type { DriveChannel } from '../../types';

export async function openMembersDialog(drive: Pick<DriveChannel, 'id' | 'title'>): Promise<void> {
    const driveId = Number(drive?.id || 0);
    if (!driveId) return;
    members_dialog.open({ driveId, title: String(drive?.title || t('members.subtitle_fallback')) });
    // The role decides which controls the dialog draws, so it is asked for first.
    await load_drive_role(driveId, true);
    await load_members(driveId, '');
}
