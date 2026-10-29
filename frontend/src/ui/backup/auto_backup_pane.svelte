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
    import CloudUploadIcon from 'phosphor-svelte/lib/CloudArrowUpIcon';
    import LockIcon from 'phosphor-svelte/lib/LockKeyIcon';
    import FolderIcon from 'phosphor-svelte/lib/FolderIcon';
    import PauseIcon from 'phosphor-svelte/lib/PauseIcon';
    import PlayIcon from 'phosphor-svelte/lib/PlayIcon';
    import {
        getAutoBackupState, openNativeAppSettings, readNativeDeviceIdentity, setAutoBackupChargingOnly,
        setAutoBackupDeviceIdentity, setAutoBackupEnabled, setAutoBackupFolderName, setAutoBackupPermission,
        setAutoBackupWifiOnly, type AutoBackupPermission, type AutoBackupState,
    } from '../../api/auto_backup';
    import { autoBackupProgress } from './auto_backup_runtime';
    import { requestNativePhotoBackupAccess, readNativePhotoBackupAccess, nativePhotoBackupAvailable } from '../../modules/photo_backup/native_adapter';
    import { pausePhotoBackupNow, resumePhotoBackupNow, retryPhotoBackupNow, photoBackupError, photoBackupState, refreshPhotoBackup, startPhotoBackup } from '../../modules/photo_backup/controller';
    import { describeBackup } from '../gallery/photo_backup_view';
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
        onopenfiles?: (folderID: string, folderName: string) => void;
    }

    let { page = false, progress = null, onchoosephotos, onopenfiles }: Props = $props();
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
    const paused = $derived($photoBackupState?.manualPaused || liveProgress?.paused || false);
    const locked = $derived(Boolean($photoBackupState?.encryptionRequired));
    const running = $derived(Boolean(current?.enabled && !paused && !locked && ($photoBackupState?.status?.phase === 'uploading' || $photoBackupState?.status?.phase === 'scanning' || (liveProgress && liveProgress.total > liveProgress.done))));
    const situation = $derived($photoBackupState?.status && $photoBackupState.settings ? describeBackup($photoBackupState) : null);
    const stillDiscovering = $derived($photoBackupState?.sources?.some((source) => source.enabled && source.scanComplete === false) && $photoBackupState.status?.phase === 'complete');
    const primaryLabel = $derived(locked ? 'photos.action_unlock_back_up' : paused ? 'photos.action_resume' : running ? 'photos.action_pause' : $photoBackupState?.status?.failed ? 'photos.action_retry' : 'auto_backup.back_up_now');
    const statusTitle = $derived(locked ? $t_store('photos.locked_title') : paused ? $t_store('photos.paused_title') : running ? $t_store('auto_backup.backing_up') : stillDiscovering ? $t_store('photos.ready_to_back_up') : situation?.title || (current?.phase === 'up_to_date' ? $t_store('photos.preparing_next') : $t_store(phaseMessageKey(current?.phase ?? 'off'))));

    async function refresh(): Promise<void> {
        try {
            current = await getAutoBackupState();
            const access = await readNativePhotoBackupAccess();
            if (access && ['undetermined', 'granted', 'limited', 'denied'].includes(access) && access !== current.permission) {
                current = await setAutoBackupPermission(access as AutoBackupPermission);
            }
            const identity = await readNativeDeviceIdentity();
            if (identity) current = await setAutoBackupDeviceIdentity(identity);
            await refreshPhotoBackup();
            loadError = '';
        } catch (cause) {
            loadError = cause instanceof Error ? cause.message : '';
        }
    }

    onMount(() => {
        // A previous build could persist enabled=true without obtaining Photos
        // access. Recover that state on entry rather than requiring off/on.
        void refresh().then(async () => {
            if (current?.enabled && current.permission === 'undetermined') await allowAccess();
        });
        const resumed = () => { if (document.visibilityState === 'visible') void refresh(); };
        document.addEventListener('visibilitychange', resumed);
        return () => document.removeEventListener('visibilitychange', resumed);
    });

    /**
     * Asks the phone for the media grant before turning backup on. Without a
     * native grant on the device the backend keeps the switch in the
     * "needs permission" phase, so turning it on still never reads the
     * library unasked.
     */
    async function askAccess(): Promise<AutoBackupPermission> {
        if (!nativePhotoBackupAvailable()) throw new Error($t_store('photos.library_unavailable'));
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
                if (access !== 'granted' && access !== 'limited') return;
                current = await setAutoBackupEnabled(true);
                await refreshPhotoBackup();
                await startPhotoBackup();
            } else {
                current = await setAutoBackupEnabled(false);
            }
        } catch (cause) {
            loadError = cause instanceof Error ? cause.message : '';
        } finally {
            busy = false;
        }
    }

    async function allowAccess(): Promise<void> {
        if (busy) return;
        busy = true;
        loadError = '';
        try {
            const access = await askAccess();
            current = await setAutoBackupPermission(access);
            if (current.enabled && (access === 'granted' || access === 'limited')) {
                await refreshPhotoBackup();
                await startPhotoBackup();
            }
        } catch (cause) {
            loadError = cause instanceof Error ? cause.message : '';
        } finally { busy = false; }
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

    async function controlBackup(): Promise<void> {
        if (busy) return;
        busy = true;
        try {
            if (paused) await resumePhotoBackupNow();
            else if (running) await pausePhotoBackupNow();
            else if ($photoBackupState?.status?.failed) await retryPhotoBackupNow();
            else { await refreshPhotoBackup(); await startPhotoBackup(); }
        } finally { busy = false; }
    }

    function bytesText(value: number): string {
        return formatBytes(value);
    }
</script>

<section class="auto-backup" class:is-page={page} aria-label={$t_store('auto_backup.title')} aria-busy={busy}>
    {#if current === null}
        <p class="ab-muted">{loadError || $t_store('auto_backup.loading')}</p>
    {:else}
        {#if current.enabled && !denied}
            <div class="ab-status-card" class:is-paused={paused}>
                <div class="ab-status-top">
                    <span class="ab-status-symbol" aria-hidden="true">{#if locked}<LockIcon size={24} />{:else}<CloudUploadIcon size={26} />{/if}</span>
                    <div class="ab-grow">
                        <h2 class="ab-status-title" role="status">{statusTitle}</h2>
                        <p class="ab-muted ab-small">{$t_store('auto_backup.drive_folder_path', { folder: current.folderName })}</p>
                    </div>
                </div>
                {#if liveProgress && liveProgress.total > 0}
                    <div class="ab-count"><strong>{liveProgress.done.toLocaleString()}</strong><span>/ {liveProgress.total.toLocaleString()}</span></div>
                    <p class="ab-count-accessible">{$t_store('auto_backup.progress_count', { done: liveProgress.done, total: liveProgress.total })}</p>
                    <ProgressBar value={progressValue} max={100} label={$t_store('auto_backup.backing_up')} />
                    {#if liveProgress.currentName && !paused}
                        <div class="ab-current-file"><span class="ab-file-dot" aria-hidden="true"></span><span>{liveProgress.currentName}</span><small>{bytesText(liveProgress.currentBytes)}</small></div>
                    {/if}
                {:else if running}
                    <ProgressBar indeterminate label={$t_store('photos.preparing_next')} />
                {/if}
                {#if locked}<p class="ab-muted ab-small">{$t_store('photos.locked_body')}</p>{/if}
                <div class="ab-status-actions">
                    <Button variant={running ? 'secondary' : 'primary'} loading={busy} onclick={() => void controlBackup()}>
                        {#if running}<PauseIcon size={16} aria-hidden="true" />{:else}<PlayIcon size={16} aria-hidden="true" />{/if}
                        {$t_store(primaryLabel)}
                    </Button>
                    {#if onopenfiles && current.folderId}
                        <Button variant="secondary" onclick={() => onopenfiles?.(current!.folderId, current!.folderName)}><FolderIcon size={16} aria-hidden="true" />{$t_store('mobile.tab_files')}</Button>
                    {/if}
                </div>
            </div>
        {/if}
        <div class="ab-card">
            <SwitchRow
                title={$t_store('auto_backup.auto_backup')}
                description={$t_store('auto_backup.auto_backup_hint', { device: deviceWord })}
                checked={current.enabled}
                disabled={busy || (denied && !current.enabled)}
                onchange={(value) => void toggleEnabled(value)}
            />
        </div>

        {#if current.permission === 'undetermined'}
            <div class="ab-card" role="status">
                <p class="ab-muted">{$t_store('auto_backup.phase_needs_permission')}</p>
                <Button variant="primary" loading={busy} onclick={() => void allowAccess()}>{$t_store('auto_backup.allow_photos')}</Button>
            </div>
        {:else if denied}
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
        </div>
        <details class="ab-card ab-preferences" open>
            <summary>{$t_store('settings.title')}</summary>
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
        </details>
        <details class="ab-encryption-note">
            <summary><LockIcon size={14} aria-hidden="true" />{$t_store('mobile.account_encryption')}</summary>
            <p class="ab-muted ab-small">{$t_store('modals.enc_setup_help')}</p>
            <p class="ab-muted ab-small">{$t_store('modals.enc_setup_warning')}</p>
        </details>
        {#if !current.enabled || denied}
            <p class="ab-muted ab-small">{$t_store(phaseMessageKey(current.phase))}</p>
        {/if}
        {#if $photoBackupError}<p class="ab-error" role="alert">{$photoBackupError}</p>{/if}
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
        /* minmax(0, 1fr): a single track that cannot grow past the pane. An
           auto track sized to its widest child pushed the cards out to the
           right, so the left gutter read wider than the right. */
        grid-template-columns: minmax(0, 1fr);
        gap: var(--space-3);
        padding: var(--space-3) var(--space-3) var(--space-4);
        color: var(--text-main);
    }
    /* On the phone the account scroller already supplies the 16px side
       gutter and the bottom inset, like every other account drill-in (the
       appearance panel sets padding: 0). The bottom adds the floating tab bar
       on top, so the last row scrolls clear of it. */
    .auto-backup.is-page {
        padding: 0 0 calc(var(--mobile-overlay-bottom, 0px) + var(--space-3));
    }
    .ab-status-card { display: grid; grid-template-columns: minmax(0, 1fr); min-width: 0; gap: var(--space-3); border: 1px solid var(--border); border-radius: var(--radius-lg); background: var(--surface-control); padding: var(--space-4); }
    .ab-status-top { display: flex; align-items: center; gap: var(--space-3); }
    .ab-status-symbol { display: grid; place-items: center; width: 46px; height: 46px; flex-shrink: 0; border-radius: var(--radius-md); color: var(--accent); background: var(--overlay-accent-1); }
    .ab-status-title { margin: 0 0 var(--space-1); font-size: var(--font-size-md); font-weight: var(--weight-strong); }
    .ab-count { display: flex; align-items: baseline; gap: var(--space-2); font-variant-numeric: tabular-nums; }
    .ab-count strong { font-size: 36px; line-height: 1.15; font-weight: 600; letter-spacing: -0.03em; }
    .ab-count span { color: var(--text-muted); font-size: var(--font-size-md); }
    .ab-count-accessible { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
    .ab-status-card :global(.ui-progress-header) { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
    .ab-current-file { display: flex; gap: var(--space-2); align-items: center; min-width: 0; color: var(--text-muted); font-size: var(--font-size-xs); }
    .ab-current-file > span:not(.ab-file-dot) { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1; }
    .ab-current-file small { flex-shrink: 0; }
    .ab-file-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--accent); }
    .ab-status-actions { display: flex; gap: var(--space-2); }
    .ab-status-actions :global(.ui-button) { flex: 1; }
    .ab-preferences summary { padding: var(--space-2) 0; font-size: var(--font-size-sm); font-weight: var(--weight-strong); cursor: pointer; }
    .ab-encryption-note { font-size: var(--font-size-xs); color: var(--text-muted); }
    .ab-encryption-note summary { display: flex; align-items: center; gap: var(--space-2); cursor: pointer; padding: var(--space-2) 0; }
    .ab-encryption-note p { margin: var(--space-2) 0; line-height: 1.5; }
    .ab-card {
        border: 1px solid var(--border);
        border-radius: var(--radius-lg);
        padding: var(--space-2) var(--space-3);
        display: grid;
        grid-template-columns: minmax(0, 1fr);
        min-width: 0;
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
    .ab-denied, .ab-limited {
        display: grid;
        grid-template-columns: minmax(0, 1fr);
        min-width: 0;
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
