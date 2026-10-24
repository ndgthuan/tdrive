import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import * as api from '../../api/drive_members';
import { sidebarState } from '../sidebar/sidebar_store';
import {
    access_from_snapshot,
    change_member_role,
    clear_drive_roles,
    drive_access_for,
    load_drive_role,
    load_members,
    members_view,
    remove_member,
} from './members_store';

vi.mock('../../api/runtime', () => ({
    onRuntimeEvent: vi.fn(() => () => {}),
}));

vi.mock('../../api/drive_members', async (importOriginal) => {
    const actual = await importOriginal<typeof import('../../api/drive_members')>();
    return {
        ...actual,
        my_drive_role: vi.fn(),
        refresh_my_drive_role: vi.fn(),
        list_drive_members: vi.fn(),
        set_drive_member_role: vi.fn(),
        remove_drive_member: vi.fn(),
    };
});

const SHARED_ID = 5;

function snapshot(role: api.DriveRole, overrides: Partial<api.DriveRoleSnapshot> = {}): api.DriveRoleSnapshot {
    const perms = {
        owner: { canUpload: true, canDeleteOthers: true, canManageMembers: true, canInvite: true },
        manager: { canUpload: true, canDeleteOthers: true, canManageMembers: true, canInvite: true },
        editor: { canUpload: true, canDeleteOthers: false, canManageMembers: false, canInvite: false },
        viewer: { canUpload: false, canDeleteOthers: false, canManageMembers: false, canInvite: false },
    }[role];
    return {
        ok: true,
        errorCode: '',
        errorMessage: '',
        role,
        ...perms,
        ...overrides,
    };
}

function row(userId: number, role: api.DriveRole, name = `User ${userId}`): api.DriveMemberRow {
    return { userId, displayName: name, username: `user${userId}`, role, joinedAt: 0, isSelf: false };
}

function seedMembers(rows: api.DriveMemberRow[]): void {
    members_view.set({
        driveId: SHARED_ID,
        query: '',
        status: 'ready',
        members: rows,
        total: rows.length,
        nextOffset: rows.length,
        hasMore: false,
        loadingMore: false,
        busyUserId: 0,
        errorCode: '',
        errorMessage: '',
    });
}

beforeEach(() => {
    vi.mocked(api.my_drive_role).mockReset();
    vi.mocked(api.refresh_my_drive_role).mockReset();
    vi.mocked(api.list_drive_members).mockReset();
    vi.mocked(api.set_drive_member_role).mockReset();
    vi.mocked(api.remove_drive_member).mockReset();
    clear_drive_roles();
    sidebarState.set({
        personal: [{ id: 1, title: 'TDrive', kind: 'personal', isActive: false, inviteLink: '' }],
        shared: [{ id: SHARED_ID, title: 'Team', kind: 'shared', isActive: true, inviteLink: '' }],
        pending: [],
        activeChannelId: SHARED_ID,
        virtualView: null,
    });
    seedMembers([]);
});

describe('access from a role snapshot', () => {
    it('allows everything on a personal drive', () => {
        const access = access_from_snapshot(1, false, undefined);
        expect(access).toMatchObject({ shared: false, known: true, role: 'owner', canUpload: true, canDeleteOthers: true, viewOnly: false });
    });

    it('marks a viewer as view only and lets nothing else through', () => {
        const access = access_from_snapshot(SHARED_ID, true, snapshot('viewer'));
        expect(access).toMatchObject({ known: true, viewOnly: true, canUpload: false, canDeleteOthers: false, canManageMembers: false });
    });

    it('keeps uploads open but hides member and delete-others controls while the role is unknown', () => {
        const access = access_from_snapshot(SHARED_ID, true, undefined);
        expect(access).toMatchObject({ known: false, canUpload: true, canDeleteOthers: false, canManageMembers: false, canInvite: false, viewOnly: false });
    });

    it('treats a refused role lookup as unknown rather than as a viewer', () => {
        const access = access_from_snapshot(SHARED_ID, true, snapshot('viewer', { ok: false, errorCode: 'not_shared_drive' }));
        expect(access.known).toBe(false);
        expect(access.viewOnly).toBe(false);
    });
});

describe('role cache', () => {
    it('caches MyRole per drive and reports the caller access', async () => {
        vi.mocked(api.my_drive_role).mockResolvedValue(snapshot('editor'));
        const access = await load_drive_role(SHARED_ID);
        expect(api.my_drive_role).toHaveBeenCalledWith(SHARED_ID);
        expect(access).toMatchObject({ role: 'editor', canUpload: true, canDeleteOthers: false });
        expect(drive_access_for(SHARED_ID, true)).toMatchObject({ role: 'editor' });
    });

    it('bypasses the cache when asked to refresh', async () => {
        vi.mocked(api.refresh_my_drive_role).mockResolvedValue(snapshot('manager'));
        await load_drive_role(SHARED_ID, true);
        expect(api.refresh_my_drive_role).toHaveBeenCalledWith(SHARED_ID);
        expect(drive_access_for(SHARED_ID, true).canManageMembers).toBe(true);
    });

    it('keeps the drive unknown when the lookup throws', async () => {
        vi.mocked(api.my_drive_role).mockRejectedValue(new Error('offline'));
        await load_drive_role(SHARED_ID);
        expect(drive_access_for(SHARED_ID, true).known).toBe(false);
    });
});

describe('member list', () => {
    it('loads the first page for the drive', async () => {
        vi.mocked(api.list_drive_members).mockResolvedValue({
            ok: true,
            errorCode: '',
            errorMessage: '',
            members: [row(1, 'owner', 'Ann')],
            total: 1,
            nextOffset: 1,
            hasMore: false,
        });
        await load_members(SHARED_ID, '');
        expect(get(members_view)).toMatchObject({ status: 'ready', members: [expect.objectContaining({ userId: 1 })], total: 1 });
    });

    it('moves a member to the new role straight away and keeps it when Telegram agrees', async () => {
        seedMembers([row(2, 'editor')]);
        vi.mocked(api.set_drive_member_role).mockResolvedValue({ ok: true, errorCode: '', errorMessage: '' });

        const pending = change_member_role(2, 'viewer');
        expect(get(members_view).members[0].role).toBe('viewer');
        expect(get(members_view).busyUserId).toBe(2);

        const outcome = await pending;
        expect(outcome.ok).toBe(true);
        expect(get(members_view).members[0].role).toBe('viewer');
        expect(get(members_view).busyUserId).toBe(0);
    });

    it('rolls the role back when Telegram refuses, and reports the code', async () => {
        seedMembers([row(3, 'manager')]);
        vi.mocked(api.set_drive_member_role).mockResolvedValue({ ok: false, errorCode: 'last_manager', errorMessage: 'nope' });

        const outcome = await change_member_role(3, 'editor');

        expect(outcome).toEqual({ ok: false, errorCode: 'last_manager', errorMessage: 'nope' });
        expect(get(members_view).members[0].role).toBe('manager');
        expect(get(members_view).busyUserId).toBe(0);
    });

    it('rolls the role back when the call throws', async () => {
        seedMembers([row(4, 'editor')]);
        vi.mocked(api.set_drive_member_role).mockRejectedValue(new Error('network down'));

        const outcome = await change_member_role(4, 'manager');

        expect(outcome.ok).toBe(false);
        expect(get(members_view).members[0].role).toBe('editor');
    });

    it('removes a member at once and puts the row back in place when the removal is refused', async () => {
        seedMembers([row(5, 'editor'), row(6, 'viewer'), row(7, 'viewer')]);
        vi.mocked(api.remove_drive_member).mockResolvedValue({ ok: false, errorCode: 'permission_denied', errorMessage: '' });

        const pending = remove_member(6, true);
        expect(get(members_view).members.map((member) => member.userId)).toEqual([5, 7]);
        expect(get(members_view).total).toBe(2);

        const outcome = await pending;

        expect(outcome.errorCode).toBe('permission_denied');
        expect(get(members_view).members.map((member) => member.userId)).toEqual([5, 6, 7]);
        expect(get(members_view).total).toBe(3);
        expect(api.remove_drive_member).toHaveBeenCalledWith(SHARED_ID, 6, true);
    });

    it('keeps a successful removal gone', async () => {
        seedMembers([row(8, 'viewer')]);
        vi.mocked(api.remove_drive_member).mockResolvedValue({ ok: true, errorCode: '', errorMessage: '' });

        const outcome = await remove_member(8, false);

        expect(outcome.ok).toBe(true);
        expect(get(members_view).members).toEqual([]);
    });
});
