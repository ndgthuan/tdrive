<script lang="ts">
    /**
     * Automatic photo and video backup, as the Account page shows it. The
     * toggle turns the phone's backup on; the folder row shows where it goes
     * in My Drive and can be renamed; the two policy switches hold uploads
     * back. A denied grant replaces the controls with the way out.
     *
     * The pane only renders what the backend reports. Progress is not read
     * here: the caller passes it when the upload queue can supply it, and
     * otherwise the progress card stays hidden rather than showing a guess.
     */
    import { onMount } from 'svelte';
    import Button from '../button.svelte';
    import ProgressBar from '../progress_bar.svelte';
    import SwitchRow from '../switch_row.svelte';
    import {
        getAutoBackupState, openNativeAppSettings, readNativeDeviceIdentity, setAutoBackupChargingOnly,
        setAutoBackupDeviceIdentity, setAutoBackupEnabled, setAutoBackupFolderName, setAutoBackupPermission,
        setAutoBackupWifiOnly, backUpAutoBackupNow, type AutoBackupPermission, type AutoBackupState,
    } from '../../api/auto_backup';
    import { autoBackupProgress } from './auto_backup_runtime';
    import { requestNativePhotoBackupAccess, nativePhotoBackupAvailable } from '../../modules/photo_backup/native_adapter';
    import { t_store } from '../../i18n/i18n';
    import {
        folderNameProblemKey, formatBytes, normalizedFolderName, permissionIsLimited,
        permissionNeedsSettings, phaseMessageKey, progressPercent, type AutoBackupProgress,
    } from './auto_backup_model';

    interface Props {
        /** True where the pane is the whole screen (phone, opened from Account). */
        page?: boolean;
        progress?: AutoBackupProgress | null;
        /** Opens the photo selection screen, used by the denied state. */
        onchoosephotos?: () => void;
    }

    let { page = false, progress = null, onchoosephotos }: Props = $props();
    // A caller's snapshot wins; otherwise the runtime's latest queue summary.
    const liveProgress = $derived(progress ?? $autoBackupProgress);

    let current = $state<AutoBackupState | null>(null);
    let loadError = $state('');
    let busy = $state(false);
    let renaming = $state(false);
    let draftName = $state('');
    let nameError = $state('');

    const denied = $derived(current !== null && permissionNeedsSettings(current.permission));
    const limited = $derived(current !== null && permissionIsLimited(current.permission));
    const deviceWord = $derived(current?.platform === 'ios' ? $t_store('auto_backup.device_iphone') : $t_store('auto_backup.device_phone'));
    const settingsWord = $derived(current?.platform === 'ios' ? $t_store('auto_backup.settings_ios') : $t_store('auto_backup.settings_android'));
    const progressValue = $derived(liveProgress ? progressPercent(liveProgress.done, liveProgress.total) : 0);

    async function refresh(): Promise<void> {
        try {
            current = await getAutoBackupState();
            const identity = await readNativeDeviceIdentity();
            if (identity) current = await setAutoBackupDeviceIdentity(identity);
            loadError = '';
        } catch (cause) {
            loadError = cause instanceof Error ? cause.message : '';
        }
    }

    onMount(() => { void refresh(); });

    /**
     * Asks the phone for the media grant before turning backup on. Without a
     * native grant on the device the backend keeps the switch in the
     * "needs permission" phase, so turning it on still never reads the
     * library unasked.
     */
    async function askAccess(): Promise<AutoBackupPermission> {
        if (!nativePhotoBackupAvailable()) return 'undetermined';
        const access = await requestNativePhotoBackupAccess();
        if (access.status === 'granted' || access.status === 'authorized') return 'granted';
        if (access.status === 'limited') return 'limited';
        if (access.status === 'denied') return 'denied';
        return 'undetermined';
    }

    async function toggleEnabled(value: boolean): Promise<void> {
        if (busy || !current) return;
        busy = true;
        try {
            if (value) {
                const access = await askAccess();
                current = await setAutoBackupPermission(access);
                if (access === 'denied') return;
                current = await setAutoBackupEnabled(true);
            } else {
                current = await setAutoBackupEnabled(false);
            }
        } catch (cause) {
            loadError = cause instanceof Error ? cause.message : '';
        } finally {
            busy = false;
        }
    }

    function startRename(): void {
        if (!current) return;
        draftName = current.folderNameCustom ? current.folderName : '';
        nameError = '';
        renaming = true;
    }

    async function saveRename(): Promise<void> {
        const problem = folderNameProblemKey(draftName);
        if (problem) {
            nameError = $t_store(`auto_backup.${problem}`);
            return;
        }
        busy = true;
        try {
            current = await setAutoBackupFolderName(normalizedFolderName(draftName));
            renaming = false;
            nameError = '';
        } catch (cause) {
            nameError = cause instanceof Error ? cause.message : '';
        } finally {
            busy = false;
        }
    }

    async function changeWifi(value: boolean): Promise<void> {
        current = await setAutoBackupWifiOnly(value);
    }

    async function changeCharging(value: boolean): Promise<void> {
        current = await setAutoBackupChargingOnly(value);
    }

    async function openSettings(): Promise<void> {
        await openNativeAppSettings();
    }

    async function backUpNow(): Promise<void> {
        busy = true;
        try { current = await backUpAutoBackupNow(); } finally { busy = false; }
    }

    function bytesText(value: number): string {
        return formatBytes(value);
    }
</script>

<section class="auto-backup" class:is-page={page} aria-label={$t_store('auto_backup.title')} aria-busy={busy}>
    {#if current === null}
        <p class="ab-muted">{loadError || $t_store('auto_backup.loading')}</p>
    {:else}
        <div class="ab-card">
            <SwitchRow
                title={$t_store('auto_backup.auto_backup')}
                description={$t_store('auto_backup.auto_backup_hint', { device: deviceWord })}
                checked={current.enabled}
                disabled={busy || (denied && !current.enabled)}
                onchange={(value) => void toggleEnabled(value)}
            />
        </div>

        {#if denied}
            <div class="ab-denied" role="status">
                <p class="ab-title">{$t_store('auto_backup.denied_title')}</p>
                <p class="ab-muted">{$t_store('auto_backup.denied_detail', { settings: settingsWord })}</p>
                <Button variant="primary" onclick={() => void openSettings()}>
                    {$t_store('auto_backup.denied_open', { settings: settingsWord })}
                </Button>
                {#if onchoosephotos}
                    <Button variant="secondary" onclick={() => onchoosephotos?.()}>{$t_store('auto_backup.choose_photos')}</Button>
                    <p class="ab-muted">{$t_store('auto_backup.denied_choose_hint')}</p>
                {/if}
            </div>
        {:else if limited}
            <div class="ab-limited" role="status">
                <p class="ab-title">{$t_store('auto_backup.limited_title')}</p>
                <p class="ab-muted">{$t_store('auto_backup.limited_detail')}</p>
                <Button variant="secondary" size="sm" onclick={() => void openSettings()}>{$t_store('auto_backup.open_settings')}</Button>
            </div>
        {/if}

        <div class="ab-card ab-rows">
            <div class="ab-row">
                <div class="ab-grow">
                    <p class="ab-label">{$t_store('auto_backup.saved_to')}</p>
                    <p class="ab-muted">{$t_store('auto_backup.drive_folder_path', { folder: current.folderName })}</p>
                    {#if !current.folderNameCustom}
                        <p class="ab-muted ab-small">{$t_store('auto_backup.folder_name_hint', { name: current.defaultFolderName })}</p>
                    {/if}
                </div>
                {#if !renaming}
                    <Button variant="secondary" size="sm" disabled={busy} onclick={startRename}>{$t_store('auto_backup.rename')}</Button>
                {/if}
            </div>
            {#if renaming}
                <div class="ab-rename">
                    <label class="ab-label" for="auto-backup-folder-name">{$t_store('auto_backup.folder_name_label')}</label>
                    <input
                        id="auto-backup-folder-name"
                        class="ab-field"
                        type="text"
                        bind:value={draftName}
                        aria-label={$t_store('auto_backup.rename_folder_label')}
                        aria-invalid={nameError !== '' ? 'true' : undefined}
                        aria-describedby={nameError !== '' ? 'auto-backup-folder-error' : undefined}
                    />
                    {#if nameError}<p id="auto-backup-folder-error" class="ab-error" role="alert">{nameError}</p>{/if}
                    <div class="ab-actions">
                        <Button variant="secondary" size="sm" onclick={() => { renaming = false; nameError = ''; }}>{$t_store('auto_backup.cancel')}</Button>
                        <Button variant="primary" size="sm" loading={busy} onclick={() => void saveRename()}>{$t_store('auto_backup.save')}</Button>
                    </div>
                </div>
            {/if}
            <SwitchRow
                title={$t_store('auto_backup.wifi_only')}
                description={$t_store('auto_backup.wifi_only_hint')}
                checked={current.wifiOnly}
                disabled={busy}
                onchange={(value) => void changeWifi(value)}
            />
            <SwitchRow
                title={$t_store('auto_backup.charging_only')}
                description={$t_store('auto_backup.charging_only_hint')}
                checked={current.chargingOnly}
                disabled={busy}
                onchange={(value) => void changeCharging(value)}
            />
        </div>

        {#if liveProgress && liveProgress.enabled !== false && liveProgress.total > 0}
            <div class="ab-card ab-progress">
                <div class="ab-row">
                    <p class="ab-label ab-grow">{$t_store('auto_backup.backing_up')}</p>
                    <p class="ab-muted">{$t_store('auto_backup.progress_count', { done: liveProgress.done, total: liveProgress.total })}</p>
                </div>
                <ProgressBar value={progressValue} max={100} label={$t_store('auto_backup.backing_up')} />
                <p class="ab-muted ab-small">{$t_store('auto_backup.current_file', { name: liveProgress.currentName, size: bytesText(liveProgress.currentBytes) })}</p>
                <Button variant="primary" size="sm" loading={busy} onclick={() => void backUpNow()}>{$t_store('auto_backup.back_up_now')}</Button>
                <p class="ab-muted ab-small">{$t_store('auto_backup.back_up_later_note')}</p>
            </div>
        {/if}

        <p class="ab-muted ab-small">{$t_store(phaseMessageKey(current.phase))}</p>
        {#each current.errors as message (message)}
            <p class="ab-error" role="status">{message}</p>
        {/each}
        {#if loadError}<p class="ab-error" role="alert">{loadError}</p>{/if}
        <p class="ab-muted ab-small">{$t_store('auto_backup.keep_open')}</p>
    {/if}
</section>

<style>
    .auto-backup {
        box-sizing: border-box;
        display: grid;
        gap: var(--space-3);
        padding: var(--space-3) var(--space-3) var(--space-4);
        color: var(--text-main);
    }
    .auto-backup.is-page { padding: 0 14px 14px; }
    .ab-card {
        border: 1px solid var(--border);
        border-radius: var(--radius-lg);
        padding: var(--space-2) var(--space-3);
        display: grid;
        gap: var(--space-2);
    }
    .ab-rows { padding: var(--space-2) var(--space-3); gap: var(--space-2); }
    .ab-rows .ab-row { padding-left: 0; padding-right: 0; }
    .ab-row { display: flex; align-items: center; gap: var(--space-2); padding: var(--space-2) var(--space-3); }
    .ab-grow { flex: 1; min-width: 0; margin: 0; }
    .ab-label { margin: 0; font-size: var(--font-size-sm); font-weight: var(--weight-strong); }
    .ab-muted { margin: 0; color: var(--text-muted); font-size: var(--font-size-sm); overflow-wrap: anywhere; }
    .ab-small { font-size: var(--font-size-xs); }
    .ab-title { margin: 0; font-weight: var(--weight-strong); }
    .ab-error { margin: 0; color: var(--danger); font-size: var(--font-size-sm); }
    .ab-denied, .ab-limited, .ab-progress {
        display: grid;
        gap: var(--space-2);
        border-radius: var(--radius-lg);
        padding: var(--space-3);
        border: 1px solid var(--color-warning);
    }
    .ab-rename, .ab-actions { display: flex; flex-wrap: wrap; gap: var(--space-2); padding: 0 var(--space-3) var(--space-2); align-items: center; }
    .ab-rename { flex-direction: column; align-items: stretch; }
    .ab-field {
        min-height: 44px;
        border-radius: var(--radius-sm);
        border: 1px solid var(--border);
        background: var(--surface-control);
        color: var(--text-main);
        padding: 0 var(--space-2);
        font: inherit;
    }
    .ab-actions { justify-content: flex-end; padding-left: 0; padding-right: 0; }
</style>
