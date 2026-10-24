import { beforeEach, describe, expect, it, vi } from 'vitest';
import * as bindings from '../../bindings/TDrive/internal/app/drivemembersservice';
import {
    list_drive_members,
    my_drive_role,
    normalizeDriveMembersPage,
    normalizeDriveRoleSnapshot,
    set_drive_member_role,
} from './drive_members';

vi.mock('../../bindings/TDrive/internal/app/drivemembersservice', () => ({
    CreateInvite: vi.fn(),
    GetDefaultRole: vi.fn(),
    ListMembers: vi.fn(),
    MyRole: vi.fn(),
    RefreshMyRole: vi.fn(),
    RemoveMember: vi.fn(),
    SetDefaultRole: vi.fn(),
    SetRole: vi.fn(),
}));

describe('drive members api wrapper', () => {
    beforeEach(() => {
        vi.mocked(bindings.MyRole).mockReset();
        vi.mocked(bindings.ListMembers).mockReset();
        vi.mocked(bindings.SetRole).mockReset();
    });

    it('reads the role permissions from the Telegram answer', async () => {
        vi.mocked(bindings.MyRole).mockResolvedValue({
            result: { ok: true },
            role: 'editor',
            can_upload: true,
            can_delete_others: false,
            can_manage_members: false,
            can_invite: false,
        } as never);
        await expect(my_drive_role(5)).resolves.toEqual({
            ok: true,
            errorCode: '',
            errorMessage: '',
            role: 'editor',
            canUpload: true,
            canDeleteOthers: false,
            canManageMembers: false,
            canInvite: false,
        });
        expect(bindings.MyRole).toHaveBeenCalledWith(5);
    });

    it('keeps the error code and message from a refused call', async () => {
        vi.mocked(bindings.SetRole).mockResolvedValue({
            ok: false,
            error: { code: 'cannot_change_owner', message: 'The owner cannot be changed.' },
        } as never);
        await expect(set_drive_member_role(5, 9, 'editor')).resolves.toEqual({
            ok: false,
            errorCode: 'cannot_change_owner',
            errorMessage: 'The owner cannot be changed.',
        });
    });

    it('lists one page with its cursor and drops unknown roles to viewer', async () => {
        vi.mocked(bindings.ListMembers).mockResolvedValue({
            result: { ok: true },
            members: [
                { user_id: 1, display_name: 'Ann', username: 'ann', role: 'owner', joined_at: 10, is_self: true },
                { user_id: 2, display_name: 'Bo', username: '', role: 'weird', joined_at: 0, is_self: false },
            ],
            total: 2,
            next_offset: 2,
            has_more: true,
        } as never);
        const page = await list_drive_members(5, '', 0, 50);
        expect(bindings.ListMembers).toHaveBeenCalledWith(5, '', 0, 50);
        expect(page.hasMore).toBe(true);
        expect(page.nextOffset).toBe(2);
        expect(page.members.map((member) => [member.userId, member.role, member.isSelf])).toEqual([
            [1, 'owner', true],
            [2, 'viewer', false],
        ]);
    });

    it('treats a null member list as empty and a missing envelope as a failure', () => {
        expect(normalizeDriveMembersPage({ result: { ok: true }, members: null }).members).toEqual([]);
        expect(normalizeDriveRoleSnapshot({}).ok).toBe(false);
    });
});
