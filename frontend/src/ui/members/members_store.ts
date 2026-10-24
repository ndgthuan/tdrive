// Role cache and member list state for shared drives.
//
// The cache holds the caller's MyRole per drive. Telegram is the only authority:
// a cached answer only decides what is hidden, and the backend still refuses
// anything that slips through. Personal drives are never gated.
import { derived, get, writable } from 'svelte/store';
import {
    list_drive_members,
    my_drive_role,
    refresh_my_drive_role,
    remove_drive_member,
    set_drive_member_role,
    type AssignableRole,
    type DriveMemberRow,
    type DriveOperationOutcome,
    type DriveRole,
    type DriveRoleSnapshot,
} from '../../api/drive_members';
import { onRuntimeEvent, type RuntimeUnsubscribe } from '../../api/runtime';
import { sidebarState, type SidebarState } from '../sidebar/sidebar_store';

export const MEMBERS_PAGE_SIZE = 50;

// --- Role cache -----------------------------------------------------------

const role_cache = writable<Record<number, DriveRoleSnapshot>>({});

/** The raw cached MyRole answers by drive id. Components derive access from them with access_from_snapshot. */
export const drive_roles = { subscribe: role_cache.subscribe };

/**
 * What the caller may do in one drive. For a personal drive everything is allowed.
 * For a shared drive whose role has not loaded yet, only the rules that are
 * always true apply: uploads stay open, deleting others' files stays closed,
 * and member management stays hidden until Telegram answers.
 */
export interface DriveAccess {
    driveId: number;
    shared: boolean;
    /** True once Telegram has answered for this drive. */
    known: boolean;
    role: DriveRole | '';
    canUpload: boolean;
    canDeleteOthers: boolean;
    canManageMembers: boolean;
    canInvite: boolean;
    /** The caller reads this shared drive and cannot change anything in it. */
    viewOnly: boolean;
}

export function access_from_snapshot(driveId: number, shared: boolean, snapshot: DriveRoleSnapshot | undefined): DriveAccess {
    if (!shared) {
        return {
            driveId,
            shared: false,
            known: true,
            role: 'owner',
            canUpload: true,
            canDeleteOthers: true,
            canManageMembers: true,
            canInvite: true,
            viewOnly: false,
        };
    }
    if (!snapshot || !snapshot.ok || snapshot.role === '') {
        return {
            driveId,
            shared: true,
            known: false,
            role: '',
            canUpload: true,
            canDeleteOthers: false,
            canManageMembers: false,
            canInvite: false,
            viewOnly: false,
        };
    }
    return {
        driveId,
        shared: true,
        known: true,
        role: snapshot.role,
        canUpload: snapshot.canUpload,
        canDeleteOthers: snapshot.canDeleteOthers,
        canManageMembers: snapshot.canManageMembers,
        canInvite: snapshot.canInvite,
        viewOnly: snapshot.role === 'viewer',
    };
}

function is_shared_drive(sidebar: SidebarState, driveId: number): boolean {
    return sidebar.shared.some((drive) => drive.id === driveId);
}

/** Access for a drive, read now. Used by plain modules that are not components. */
export function drive_access_for(driveId: number, shared: boolean): DriveAccess {
    return access_from_snapshot(driveId, shared, get(role_cache)[driveId]);
}

/** Access for the drive the sidebar has open. Reactive, so the UI follows a switch. */
export const active_drive_access = derived([sidebarState, role_cache], ([sidebar, roles]) => {
    const driveId = sidebar.activeChannelId ?? 0;
    return access_from_snapshot(driveId, is_shared_drive(sidebar, driveId), roles[driveId]);
});

/** Whether the open drive takes writes from this caller: true for personal drives and unknown roles. */
export const active_drive_can_edit = derived(active_drive_access, (access) => !access.shared || !access.known || access.canUpload);

/** Whether the open drive lets this caller empty its trash or delete others' items. */
export const active_drive_can_delete_others = derived(active_drive_access, (access) => !access.shared || !access.known || access.canDeleteOthers);

/** Access for one drive given by id, reactive. Used where a drive is not the one open. */
export function drive_access_store(driveId: number, shared: boolean) {
    return derived(role_cache, (roles) => access_from_snapshot(driveId, shared, roles[driveId]));
}

/** Loads (or refreshes, bypassing Telegram's one-minute cache) the caller's role in a drive. */
export async function load_drive_role(driveId: number, refresh = false): Promise<DriveAccess | null> {
    if (!driveId) return null;
    try {
        const snapshot = refresh ? await refresh_my_drive_role(driveId) : await my_drive_role(driveId);
        role_cache.update((roles) => ({ ...roles, [driveId]: snapshot }));
    } catch {
        // A failed lookup leaves the drive in its unknown state; the backend still decides.
    }
    return drive_access_for(driveId, is_shared_drive(get(sidebarState), driveId));
}

/** Forgets every cached role. Used when the session ends and in tests. */
export function clear_drive_roles(): void {
    role_cache.set({});
}

// --- Member list ----------------------------------------------------------

export type MembersStatus = 'idle' | 'loading' | 'ready' | 'error';

export interface MembersView {
    driveId: number;
    query: string;
    status: MembersStatus;
    members: DriveMemberRow[];
    total: number;
    nextOffset: number;
    hasMore: boolean;
    loadingMore: boolean;
    /** The user whose role or membership is being written right now, 0 when idle. */
    busyUserId: number;
    errorCode: string;
    errorMessage: string;
}

function empty_view(): MembersView {
    return {
        driveId: 0,
        query: '',
        status: 'idle',
        members: [],
        total: 0,
        nextOffset: 0,
        hasMore: false,
        loadingMore: false,
        busyUserId: 0,
        errorCode: '',
        errorMessage: '',
    };
}

export const members_view = writable<MembersView>(empty_view());

let list_epoch = 0;

/** Starts a fresh listing for a drive, or for a new search query. */
export async function load_members(driveId: number, query = get(members_view).query): Promise<void> {
    const epoch = ++list_epoch;
    members_view.set({ ...empty_view(), driveId, query, status: 'loading' });
    try {
        const page = await list_drive_members(driveId, query, 0, MEMBERS_PAGE_SIZE);
        if (epoch !== list_epoch) return;
        if (!page.ok) {
            members_view.update((view) => ({ ...view, status: 'error', errorCode: page.errorCode, errorMessage: page.errorMessage }));
            return;
        }
        members_view.update((view) => ({
            ...view,
            status: 'ready',
            members: page.members,
            total: page.total,
            nextOffset: page.nextOffset,
            hasMore: page.hasMore,
        }));
    } catch (error) {
        if (epoch !== list_epoch) return;
        members_view.update((view) => ({ ...view, status: 'error', errorMessage: error instanceof Error ? error.message : '' }));
    }
}

/** Appends the next page using the cursor Telegram returned. */
export async function load_more_members(): Promise<void> {
    const view = get(members_view);
    if (view.status !== 'ready' || !view.hasMore || view.loadingMore) return;
    const epoch = list_epoch;
    members_view.update((current) => ({ ...current, loadingMore: true }));
    try {
        const page = await list_drive_members(view.driveId, view.query, view.nextOffset, MEMBERS_PAGE_SIZE);
        if (epoch !== list_epoch) return;
        members_view.update((current) => ({
            ...current,
            loadingMore: false,
            members: page.ok ? [...current.members, ...page.members] : current.members,
            total: page.ok ? page.total : current.total,
            nextOffset: page.ok ? page.nextOffset : current.nextOffset,
            hasMore: page.ok ? page.hasMore : current.hasMore,
        }));
    } catch {
        if (epoch === list_epoch) members_view.update((current) => ({ ...current, loadingMore: false }));
    }
}

function patch_member(members: DriveMemberRow[], userId: number, patch: Partial<DriveMemberRow>): DriveMemberRow[] {
    return members.map((row) => (row.userId === userId ? { ...row, ...patch } : row));
}

function failure_of(error: unknown): DriveOperationOutcome {
    return { ok: false, errorCode: '', errorMessage: error instanceof Error ? error.message : '' };
}

/**
 * Changes one member's role. The row updates at once; a refusal or a thrown
 * error puts the old role back, and the outcome tells the caller what to say.
 */
export async function change_member_role(userId: number, role: AssignableRole): Promise<DriveOperationOutcome> {
    const view = get(members_view);
    const driveId = view.driveId;
    const row = view.members.find((member) => member.userId === userId);
    if (!row || !driveId) return { ok: false, errorCode: 'user_not_found', errorMessage: '' };
    const previous = row.role;

    members_view.update((current) => ({ ...current, members: patch_member(current.members, userId, { role }), busyUserId: userId }));
    let outcome: DriveOperationOutcome;
    try {
        outcome = await set_drive_member_role(driveId, userId, role);
    } catch (error) {
        outcome = failure_of(error);
    }

    members_view.update((current) => {
        if (current.driveId !== driveId) return current;
        const members = outcome.ok ? current.members : patch_member(current.members, userId, { role: previous });
        return { ...current, members, busyUserId: 0 };
    });
    return outcome;
}

/**
 * Removes a member, optionally with a ban. The row disappears at once and comes
 * back at its old place if Telegram refuses.
 */
export async function remove_member(userId: number, ban: boolean): Promise<DriveOperationOutcome> {
    const view = get(members_view);
    const driveId = view.driveId;
    const index = view.members.findIndex((member) => member.userId === userId);
    if (index < 0 || !driveId) return { ok: false, errorCode: 'user_not_found', errorMessage: '' };
    const removed = view.members[index];

    members_view.update((current) => ({
        ...current,
        members: current.members.filter((member) => member.userId !== userId),
        total: Math.max(0, current.total - 1),
        busyUserId: userId,
    }));
    let outcome: DriveOperationOutcome;
    try {
        outcome = await remove_drive_member(driveId, userId, ban);
    } catch (error) {
        outcome = failure_of(error);
    }

    members_view.update((current) => {
        if (current.driveId !== driveId) return current;
        if (outcome.ok) return { ...current, busyUserId: 0 };
        const members = [...current.members];
        members.splice(Math.min(index, members.length), 0, removed);
        return { ...current, members, total: current.total + 1, busyUserId: 0 };
    });
    return outcome;
}

// --- Activation -----------------------------------------------------------

/**
 * Keeps the role cache honest while the dashboard is up: it asks Telegram again
 * when a shared drive is opened and when the window regains focus, and it
 * follows drive:role_changed. Returns the teardown.
 */
export function activate_member_roles(): () => void {
    let previous_id: number | null = null;
    const refresh_active = (): void => {
        const sidebar = get(sidebarState);
        const id = sidebar.activeChannelId ?? 0;
        if (id && is_shared_drive(sidebar, id)) void load_drive_role(id, true);
    };

    const unsubscribe_sidebar = sidebarState.subscribe((sidebar) => {
        const id = sidebar.activeChannelId;
        if (id === previous_id) return;
        previous_id = id;
        if (id && is_shared_drive(sidebar, id)) void load_drive_role(id, true);
    });

    window.addEventListener('focus', refresh_active);

    let off_event: RuntimeUnsubscribe | null = null;
    try {
        off_event = onRuntimeEvent('drive:role_changed', (payload: unknown) => {
            const record = payload !== null && typeof payload === 'object' ? (payload as Record<string, unknown>) : {};
            const driveId = Number(record.drive_id ?? 0);
            if (driveId) void load_drive_role(driveId, true);
        });
    } catch {
        // The event bridge is unavailable in plain browser runs; roles still refresh on switch.
    }

    return () => {
        unsubscribe_sidebar();
        window.removeEventListener('focus', refresh_active);
        off_event?.();
    };
}

// --- Member counts for the drive switcher -----------------------------------

/** The member total per shared drive, read once per open of the switcher. Absent until answered. */
export const drive_member_counts = writable<Record<number, number>>({});

/** Loads how many members a shared drive has. A failed read leaves the count absent, not wrong. */
export async function load_member_count(driveId: number): Promise<void> {
    if (!driveId) return;
    try {
        const page = await list_drive_members(driveId, '', 0, 1);
        drive_member_counts.update((counts) => ({ ...counts, [driveId]: page.total }));
    } catch {
        // Leave the count out; the row still shows the drive and its role.
    }
}
