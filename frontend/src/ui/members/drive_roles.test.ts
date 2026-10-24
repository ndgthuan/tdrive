import { describe, expect, it } from 'vitest';
import { set_locale, t } from '../../i18n/i18n';
import {
    assignable_roles,
    can_change_role,
    can_remove_member,
    compare_roles,
    error_message_key,
    known_error_codes,
    permissions_for_role,
    role_is_elevated,
    role_label_key,
    type MemberTarget,
} from './drive_roles';
import type { DriveRole } from '../../api/drive_members';

const owner = { role: 'owner' as DriveRole, canManageMembers: true };
const manager = { role: 'manager' as DriveRole, canManageMembers: true };
const editor = { role: 'editor' as DriveRole, canManageMembers: false };
const viewer = { role: 'viewer' as DriveRole, canManageMembers: false };

function member(role: DriveRole, isSelf = false): MemberTarget {
    return { userId: 42, role, isSelf };
}

describe('role ordering and labels', () => {
    it('sorts owner, then manager, editor and viewer', () => {
        const roles: DriveRole[] = ['viewer', 'owner', 'editor', 'manager'];
        expect([...roles].sort(compare_roles)).toEqual(['owner', 'manager', 'editor', 'viewer']);
    });

    it('names each role through its own translation key', () => {
        expect(role_label_key('owner')).toBe('members.role_owner');
        expect(role_label_key('viewer')).toBe('members.role_viewer');
    });

    it('accents only the owner and manager pills', () => {
        expect(role_is_elevated('owner')).toBe(true);
        expect(role_is_elevated('manager')).toBe(true);
        expect(role_is_elevated('editor')).toBe(false);
        expect(role_is_elevated('viewer')).toBe(false);
    });

    it('shows the English name for each role', async () => {
        await set_locale('en');
        expect(t(role_label_key('manager'))).toBe('Manager');
        expect(t(role_label_key('editor'))).toBe('Editor');
    });
});

describe('permission matrix', () => {
    it('lets owner and manager do everything', () => {
        for (const role of ['owner', 'manager'] as const) {
            expect(permissions_for_role(role)).toEqual({
                canUpload: true,
                canDeleteOthers: true,
                canManageMembers: true,
                canInvite: true,
            });
        }
    });

    it('lets an editor upload but never delete others or manage members', () => {
        expect(permissions_for_role('editor')).toEqual({
            canUpload: true,
            canDeleteOthers: false,
            canManageMembers: false,
            canInvite: false,
        });
    });

    it('gives a viewer and an unknown role nothing', () => {
        const nothing = { canUpload: false, canDeleteOthers: false, canManageMembers: false, canInvite: false };
        expect(permissions_for_role('viewer')).toEqual(nothing);
        expect(permissions_for_role('')).toEqual(nothing);
    });
});

describe('who may hand out which role', () => {
    it('lets only the owner make managers', () => {
        expect(assignable_roles('owner')).toEqual(['manager', 'editor', 'viewer']);
        expect(assignable_roles('manager')).toEqual(['editor', 'viewer']);
        expect(assignable_roles('editor')).toEqual([]);
        expect(assignable_roles('viewer')).toEqual([]);
    });

    it('lets the owner promote an editor to manager', () => {
        expect(can_change_role(owner, member('editor'), 'manager')).toBe(true);
    });

    it('stops a manager from promoting anyone to manager', () => {
        expect(can_change_role(manager, member('editor'), 'manager')).toBe(false);
        expect(can_change_role(manager, member('viewer'), 'editor')).toBe(true);
    });

    it('stops a manager from touching another manager, but not the owner from doing it', () => {
        expect(can_change_role(manager, member('manager'), 'editor')).toBe(false);
        expect(can_change_role(owner, member('manager'), 'editor')).toBe(true);
    });

    it('never lets anyone change the owner or their own role', () => {
        expect(can_change_role(owner, member('owner'), 'editor')).toBe(false);
        expect(can_change_role(owner, member('editor', true), 'viewer')).toBe(false);
    });

    it('denies every change to an actor who cannot manage members', () => {
        expect(can_change_role(editor, member('viewer'), 'editor')).toBe(false);
        expect(can_change_role(viewer, member('editor'), 'viewer')).toBe(false);
    });
});

describe('who may remove whom', () => {
    it('lets a manager remove an editor or viewer, but not a manager', () => {
        expect(can_remove_member(manager, member('editor'))).toBe(true);
        expect(can_remove_member(manager, member('viewer'))).toBe(true);
        expect(can_remove_member(manager, member('manager'))).toBe(false);
    });

    it('lets the owner remove a manager', () => {
        expect(can_remove_member(owner, member('manager'))).toBe(true);
    });

    it('never removes the owner or yourself', () => {
        expect(can_remove_member(owner, member('owner'))).toBe(false);
        expect(can_remove_member(owner, member('editor', true))).toBe(false);
    });

    it('denies removal to an editor or viewer', () => {
        expect(can_remove_member(editor, member('viewer'))).toBe(false);
        expect(can_remove_member(viewer, member('editor'))).toBe(false);
    });
});

describe('error code mapping', () => {
    it('maps every drive_roles code to its own message', () => {
        const codes = [
            'permission_denied',
            'user_not_found',
            'not_a_member',
            'cannot_change_owner',
            'last_manager',
            'invalid_role',
            'role_unavailable',
            'not_shared_drive',
        ];
        expect(known_error_codes().sort()).toEqual([...codes].sort());
        const keys = codes.map(error_message_key);
        expect(new Set(keys).size).toBe(codes.length);
        for (const key of keys) expect(key.startsWith('members.error_')).toBe(true);
    });

    it('falls back to the generic message for unknown or empty codes', () => {
        expect(error_message_key('something_new')).toBe('members.error_generic');
        expect(error_message_key('')).toBe('members.error_generic');
    });

    it('resolves a refusal to readable English text', async () => {
        await set_locale('en');
        expect(t(error_message_key('last_manager'))).toBe('A drive needs at least one manager.');
        expect(t(error_message_key('unknown_code'))).toBe('The change could not be saved. Try again.');
    });
});
