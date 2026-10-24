// Pure rules for shared-drive roles. Telegram enforces every one of these; the
// frontend mirrors them only to decide what to show. Nothing here talks to the
// backend, so each rule can be tested without a Wails bridge.
import type { AssignableRole, DriveRole } from '../../api/drive_members';

/** Higher rank = more power. Used to sort the member list and to compare roles. */
const ROLE_RANK: Record<DriveRole, number> = { owner: 0, manager: 1, editor: 2, viewer: 3 };

export const DRIVE_ROLES: readonly DriveRole[] = ['owner', 'manager', 'editor', 'viewer'];

/** Owner first, then managers, editors and viewers. */
export function compare_roles(a: DriveRole, b: DriveRole): number {
    return ROLE_RANK[a] - ROLE_RANK[b];
}

/** The translation key of a role's name, as shown in pills and menus. */
export function role_label_key(role: DriveRole): string {
    return `members.role_${role}`;
}

/** Owner and manager pills take the accent tint; editors and viewers stay muted. */
export function role_is_elevated(role: DriveRole): boolean {
    return role === 'owner' || role === 'manager';
}

export interface RolePermissions {
    canUpload: boolean;
    canDeleteOthers: boolean;
    canManageMembers: boolean;
    canInvite: boolean;
}

/**
 * The permission matrix Telegram applies, per role. Owner and manager may do
 * everything in the drive; an editor may only add and change their own files;
 * a viewer may read and nothing else.
 */
export function permissions_for_role(role: DriveRole | ''): RolePermissions {
    switch (role) {
        case 'owner':
        case 'manager':
            return { canUpload: true, canDeleteOthers: true, canManageMembers: true, canInvite: true };
        case 'editor':
            return { canUpload: true, canDeleteOthers: false, canManageMembers: false, canInvite: false };
        default:
            return { canUpload: false, canDeleteOthers: false, canManageMembers: false, canInvite: false };
    }
}

/** Roles the actor may hand out. Only the owner makes managers. */
export function assignable_roles(actor: DriveRole | ''): AssignableRole[] {
    if (actor === 'owner') return ['manager', 'editor', 'viewer'];
    if (actor === 'manager') return ['editor', 'viewer'];
    return [];
}

export interface MemberTarget {
    userId: number;
    role: DriveRole;
    isSelf: boolean;
}

/** Whether the actor may change this member's role to `next`. */
export function can_change_role(
    actor: { role: DriveRole | ''; canManageMembers: boolean },
    target: MemberTarget,
    next: AssignableRole,
): boolean {
    if (!actor.canManageMembers || target.isSelf) return false;
    if (target.role === 'owner') return false;
    // A manager cannot touch another manager: only the owner decides that.
    if (target.role === 'manager' && actor.role !== 'owner') return false;
    return assignable_roles(actor.role).includes(next);
}

/** Whether the actor may remove this member (with or without a ban). */
export function can_remove_member(
    actor: { role: DriveRole | ''; canManageMembers: boolean },
    target: MemberTarget,
): boolean {
    if (!actor.canManageMembers || target.isSelf) return false;
    if (target.role === 'owner') return false;
    return target.role !== 'manager' || actor.role === 'owner';
}

/** The stable error codes from DriveMembersService, mapped to translation keys. */
const ERROR_KEYS: Record<string, string> = {
    permission_denied: 'members.error_permission_denied',
    user_not_found: 'members.error_user_not_found',
    not_a_member: 'members.error_not_a_member',
    cannot_change_owner: 'members.error_cannot_change_owner',
    last_manager: 'members.error_last_manager',
    invalid_role: 'members.error_invalid_role',
    role_unavailable: 'members.error_role_unavailable',
    not_shared_drive: 'members.error_not_shared_drive',
};

/** The translation key for a members error code. Unknown codes get the generic line. */
export function error_message_key(code: string): string {
    return ERROR_KEYS[code] ?? 'members.error_generic';
}

/** Every code this module knows how to say in words. */
export function known_error_codes(): string[] {
    return Object.keys(ERROR_KEYS);
}
