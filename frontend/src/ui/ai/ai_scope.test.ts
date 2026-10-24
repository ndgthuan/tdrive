// Copyright (c) 2026 Nguyễn Dương Gia Thuận

import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import { state } from '../../state';
import { sidebarState, type SidebarState } from '../sidebar/sidebar_store';
import type { DriveChannel } from '../../types';
import { build_scope, pick_scope, scope_drive, scope_override } from './ai_scope';

const personal: DriveChannel = { id: 1, title: 'TDrive', kind: 'personal', isActive: true, inviteLink: '' };
const temp: DriveChannel = { id: 77, title: 'temp', kind: 'shared', isActive: false, inviteLink: '' };
const travel: DriveChannel = { id: 78, title: 'Travel', kind: 'shared', isActive: false, inviteLink: '' };

function open_drive(drive: DriveChannel, folder = ''): void {
    state.activeChannel = { id: drive.id, title: drive.title, kind: drive.kind };
    state.currentFolderId = folder;
    sidebarState.set({
        personal: [personal],
        shared: [temp, travel],
        pending: [],
        activeChannelId: drive.id,
        virtualView: null,
    } satisfies SidebarState);
}

beforeEach(() => {
    scope_override.set(null);
    open_drive(personal);
});

afterEach(() => {
    scope_override.set(null);
    state.activeChannel = null;
    state.currentFolderId = '';
});

describe('AI scope defaults', () => {
    it('defaults to the open drive and its open folder', () => {
        open_drive(personal, 'folder-9');
        expect(build_scope()).toEqual({ channel_id: 1, folder_id: 'folder-9' });
        expect(get(scope_drive).drive?.title).toBe('TDrive');
    });

    it('follows a switch to a shared drive, with its folder', () => {
        open_drive(temp, 'shared-folder');
        expect(build_scope()).toEqual({ channel_id: 77, folder_id: 'shared-folder' });
        expect(get(scope_drive).drive?.title).toBe('temp');
    });
});

describe('AI scope picks', () => {
    it('sends another drive with its root, not the open folder', () => {
        open_drive(personal, 'folder-9');
        pick_scope(77, false);
        expect(build_scope()).toEqual({ channel_id: 77, folder_id: '' });
        expect(get(scope_drive).drive?.title).toBe('temp');
    });

    it('keeps a pick only while the same drive stays open', () => {
        open_drive(personal);
        pick_scope(78, false);
        expect(build_scope().channel_id).toBe(78);

        open_drive(temp);
        expect(build_scope()).toEqual({ channel_id: 77, folder_id: '' });
        expect(get(scope_drive).drive?.title).toBe('temp');
    });

    it('takes the open folder back for the open drive', () => {
        open_drive(temp, 'shared-folder');
        pick_scope(77, false);
        expect(build_scope().folder_id).toBe('');
        pick_scope(77, true);
        expect(build_scope()).toEqual({ channel_id: 77, folder_id: 'shared-folder' });
    });

    it('never sends a folder from the open drive with a different drive', () => {
        open_drive(personal, 'folder-9');
        pick_scope(78, true);
        expect(build_scope()).toEqual({ channel_id: 78, folder_id: '' });
    });
});
