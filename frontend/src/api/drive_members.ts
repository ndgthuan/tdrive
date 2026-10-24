// Thin wrapper over DriveMembersService. Roles come from Telegram on every call,
// so the frontend never decides a role itself; these functions only reshape the
// answers and keep the Wails envelope (result.ok / result.error) intact.
import {
    CreateInvite as rawCreateInvite,
    GetDefaultRole as rawGetDefaultRole,
    ListMembers as rawListMembers,
    MyRole as rawMyRole,
    RefreshMyRole as rawRefreshMyRole,
    RemoveMember as rawRemoveMember,
    SetDefaultRole as rawSetDefaultRole,
    SetRole as rawSetRole,
} from "../../bindings/TDrive/internal/app/drivemembersservice";
import type { OperationResult } from "../../bindings/TDrive/internal/app/models";
import { asRecord, finiteNumber } from "./shared";
import { invokeBackend } from "./gateway";

/** Roles as Telegram names them. The owner is the creator; managers are admins. */
export type DriveRole = "owner" | "manager" | "editor" | "viewer";

/** A role a caller may assign to someone else. The owner is never assignable. */
export type AssignableRole = "manager" | "editor" | "viewer";

/** The caller's role in one drive and what that role may do there. */
export interface DriveRoleSnapshot {
    ok: boolean;
    errorCode: string;
    errorMessage: string;
    role: DriveRole | "";
    canUpload: boolean;
    canDeleteOthers: boolean;
    canManageMembers: boolean;
    canInvite: boolean;
}

export interface DriveMemberRow {
    userId: number;
    displayName: string;
    username: string;
    role: DriveRole;
    joinedAt: number;
    isSelf: boolean;
}

export interface DriveMembersPage {
    ok: boolean;
    errorCode: string;
    errorMessage: string;
    members: DriveMemberRow[];
    total: number;
    nextOffset: number;
    hasMore: boolean;
}

export interface DriveInvite {
    ok: boolean;
    errorCode: string;
    errorMessage: string;
    link: string;
    defaultRole: string;
}

/** The outcome of a write. ok false carries the stable error code for the UI to map. */
export interface DriveOperationOutcome {
    ok: boolean;
    errorCode: string;
    errorMessage: string;
}

const DRIVE_ROLES: readonly string[] = ["owner", "manager", "editor", "viewer"];

function asDriveRole(value: unknown): DriveRole | "" {
    const role = String(value ?? "");
    return DRIVE_ROLES.includes(role) ? (role as DriveRole) : "";
}

/**
 * Reads the Wails envelope. Read results carry it under `result`; plain writes
 * return the OperationResult itself, so both shapes are accepted.
 */
function envelope(value: unknown): { ok: boolean; errorCode: string; errorMessage: string } {
    const raw = asRecord(value);
    const result = (raw.result !== undefined ? asRecord(raw.result) : raw) as Partial<OperationResult> & Record<string, unknown>;
    const error = asRecord(result.error);
    const hasEnvelope = result.ok !== undefined;
    return {
        ok: hasEnvelope ? Boolean(result.ok) : false,
        errorCode: String(error.code ?? ""),
        errorMessage: String(error.message ?? ""),
    };
}

export function normalizeDriveRoleSnapshot(value: unknown): DriveRoleSnapshot {
    const raw = asRecord(value);
    return {
        ...envelope(raw),
        role: asDriveRole(raw.role),
        canUpload: Boolean(raw.can_upload),
        canDeleteOthers: Boolean(raw.can_delete_others),
        canManageMembers: Boolean(raw.can_manage_members),
        canInvite: Boolean(raw.can_invite),
    };
}

export function normalizeDriveMemberRow(value: unknown): DriveMemberRow {
    const raw = asRecord(value);
    return {
        userId: finiteNumber(raw.user_id),
        displayName: String(raw.display_name ?? ""),
        username: String(raw.username ?? ""),
        role: asDriveRole(raw.role) || "viewer",
        joinedAt: finiteNumber(raw.joined_at),
        isSelf: Boolean(raw.is_self),
    };
}

export function normalizeDriveMembersPage(value: unknown): DriveMembersPage {
    const raw = asRecord(value);
    const members = Array.isArray(raw.members) ? raw.members : [];
    return {
        ...envelope(raw),
        members: members.map(normalizeDriveMemberRow),
        total: finiteNumber(raw.total),
        nextOffset: finiteNumber(raw.next_offset),
        hasMore: Boolean(raw.has_more),
    };
}

export function normalizeDriveInvite(value: unknown): DriveInvite {
    const raw = asRecord(value);
    return {
        ...envelope(raw),
        link: String(raw.link ?? ""),
        defaultRole: String(raw.default_role ?? ""),
    };
}

export function normalizeDriveOperation(value: unknown): DriveOperationOutcome {
    return envelope(value);
}

export async function my_drive_role(driveId: number): Promise<DriveRoleSnapshot> {
    return normalizeDriveRoleSnapshot(await invokeBackend(rawMyRole, driveId));
}

export async function refresh_my_drive_role(driveId: number): Promise<DriveRoleSnapshot> {
    return normalizeDriveRoleSnapshot(await invokeBackend(rawRefreshMyRole, driveId));
}

export async function list_drive_members(driveId: number, query: string, offset: number, limit: number): Promise<DriveMembersPage> {
    return normalizeDriveMembersPage(await invokeBackend(rawListMembers, driveId, query, offset, limit));
}

export async function set_drive_member_role(driveId: number, userId: number, role: AssignableRole): Promise<DriveOperationOutcome> {
    return normalizeDriveOperation(await invokeBackend(rawSetRole, driveId, userId, role));
}

export async function remove_drive_member(driveId: number, userId: number, ban: boolean): Promise<DriveOperationOutcome> {
    return normalizeDriveOperation(await invokeBackend(rawRemoveMember, driveId, userId, ban));
}

export async function create_drive_invite(driveId: number, requireApproval: boolean): Promise<DriveInvite> {
    return normalizeDriveInvite(await invokeBackend(rawCreateInvite, driveId, requireApproval));
}

/** The role newcomers start with, as Telegram reports it. */
export async function get_drive_default_role(driveId: number): Promise<DriveOperationOutcome & { role: string }> {
    const raw = asRecord(await invokeBackend(rawGetDefaultRole, driveId));
    return { ...envelope(raw), role: String(raw.role ?? "") };
}

export async function set_drive_default_role(driveId: number, role: "editor" | "viewer"): Promise<DriveOperationOutcome> {
    return normalizeDriveOperation(await invokeBackend(rawSetDefaultRole, driveId, role));
}
