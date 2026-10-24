import { afterEach, describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import Avatar from './avatar.svelte';
import Breadcrumb from './breadcrumb.svelte';
import ProfileMenu from './profile_menu.svelte';
import UploadMenu from './upload_menu.svelte';
import { breadcrumbPath, type BreadcrumbDrag } from './breadcrumb_store';
import { encryptionEntryVisible, profileLoaded, profileUser } from './profile_store';

const noop = () => {};

const drag: BreadcrumbDrag = {
    isActive: () => false,
    canDrop: () => false,
    highlight: noop,
    leave: noop,
    dropOn: noop,
    registerRoot: noop,
};

const breadcrumbProps = { onNavigate: noop, onBack: noop, drag };
const profileProps = { onOpen: noop, onEncryptionSettings: noop, onLogout: noop };
const uploadProps = { onFiles: noop, onFolder: noop };

afterEach(() => {
    breadcrumbPath.set([]);
    profileUser.set(null);
    profileLoaded.set(false);
    encryptionEntryVisible.set(false);
});

describe('Breadcrumb', () => {
    it('renders only a disabled root at the drive root', () => {
        const { body } = render(Breadcrumb, { props: breadcrumbProps });

        expect(body).toContain('My Drive');
        expect(body).not.toContain('breadcrumb-sep');
        expect(body).toContain('opacity: 0.35'); // back button dimmed
    });

    it('renders crumbs with separators and an enabled back button', () => {
        breadcrumbPath.set([
            { id: 'd:a', name: 'Docs' },
            { id: 'd:b', name: 'Taxes & <stuff>' },
        ]);

        const { body } = render(Breadcrumb, { props: breadcrumbProps });

        expect(body).toContain('Docs');
        expect(body).toContain('Taxes &amp; &lt;stuff>');
        expect((body.match(/breadcrumb-sep/g) || [])).toHaveLength(2);
        expect(body).toContain('opacity: 1');
        expect(body).toContain('data-index="1"');
    });
});

describe('Avatar', () => {
    it('renders a photo background when available', () => {
        const { body } = render(Avatar, { props: { user: { photoBase64: 'Zm9v' } } });
        expect(body).toContain('data:image/jpeg;base64,Zm9v');
    });

    it('falls back to neutral initials with no hue, whatever the user id', () => {
        const ada = render(Avatar, { props: { user: { displayName: 'Ada Lovelace', userId: 1 } } }).body;
        const grace = render(Avatar, { props: { user: { displayName: 'Grace Hopper', userId: 2 } } }).body;

        expect(ada).toContain('>AL</span>');
        expect(grace).toContain('>GH</span>');
        expect(ada).not.toMatch(/background(-color)?:\s*#/);
        expect(ada).not.toContain('style=');
        expect(grace).not.toContain('style=');
    });
});

describe('ProfileMenu', () => {
    it('shows the loading header until the profile hydrates', () => {
        const { body } = render(ProfileMenu, { props: profileProps });

        expect(body).toContain('Loading account…');
        expect(body).not.toContain('profile-menu-encryption-settings');
        expect(body).not.toContain('id="mount-drive-button"');
        expect(body).not.toContain('id="mount-selection-modal"');
        expect(body).toContain('Log out');
    });

    it('renders name, handle, and the encryption entry when enabled', () => {
        profileUser.set({ displayName: 'Ada L.', username: 'ada', userId: 7 });
        profileLoaded.set(true);
        encryptionEntryVisible.set(true);

        const { body } = render(ProfileMenu, { props: profileProps });

        expect(body).toContain('Ada L.');
        expect(body).toContain('@ada');
        expect(body).toContain('Encryption settings');
    });

    it('does not render mount controls below encryption settings', () => {
        encryptionEntryVisible.set(true);

        const { body } = render(ProfileMenu, { props: profileProps });
        const encryptionIndex = body.indexOf('id="profile-menu-encryption-settings"');
        const logoutIndex = body.indexOf('id="profile-menu-logout"');

        expect(encryptionIndex).toBeGreaterThanOrEqual(0);
        expect(logoutIndex).toBeGreaterThan(encryptionIndex);
        expect(body).not.toContain('id="mount-drive-button"');
    });

    it('lists Settings and log out, with updates and appearance moved into Settings', () => {
        const { body } = render(ProfileMenu, { props: { ...profileProps, updaterAvailable: false } });

        expect(body).toContain('id="profile-menu-settings"');
        expect(body).not.toContain('id="profile-menu-updates"');
        expect(body).not.toContain('id="profile-menu-appearance"');
        expect(body).toContain('id="profile-menu-logout"');
    });
});

describe('UploadMenu', () => {
    it('renders the trigger and a hidden menu with both items', () => {
        const { body } = render(UploadMenu, { props: uploadProps });

        expect(body).toContain('id="upload-btn"');
        expect(body).toContain('aria-expanded="false"');
        expect(body).toContain('display: none');
        expect(body).toContain('id="upload-menu-files"');
        expect(body).toContain('id="upload-menu-folder"');
    });

    it('offers only file uploads without a folder handler', () => {
        const { body } = render(UploadMenu, { props: { onFiles: noop } });

        expect(body).toContain('id="upload-menu-files"');
        expect(body).not.toContain('id="upload-menu-folder"');
    });

    it('adds New folder below a separator and labels the trigger "New"', () => {
        const { body } = render(UploadMenu, { props: { onFiles: noop, onFolder: noop, onNewFolder: noop } });

        expect(body).toContain('id="upload-menu-new-folder"');
        // Order: upload files, upload folder, separator, then New folder.
        const files = body.indexOf('id="upload-menu-files"');
        const folder = body.indexOf('id="upload-menu-folder"');
        const sep = body.indexOf('upload-menu-sep');
        const newFolder = body.indexOf('id="upload-menu-new-folder"');
        expect(files).toBeLessThan(folder);
        expect(folder).toBeLessThan(sep);
        expect(sep).toBeLessThan(newFolder);

        // The trigger now covers both upload and create, so it reads "New".
        const trigger = body.match(/id="upload-btn"[\s\S]*?<\/button>/)?.[0] ?? '';
        expect(trigger).toContain('New');
        expect(trigger).not.toContain('Upload');
    });
});
