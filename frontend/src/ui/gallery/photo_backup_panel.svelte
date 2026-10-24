<script lang="ts">
    /**
     * Photo & video backup, as one card. It reads as three things top to bottom:
     * the switch that turns it on, one situation block that says what is
     * happening and offers the one thing to do about it, and the choices
     * underneath. The situation is computed once in photo_backup_view.ts; this
     * file only renders it. Backend refusals arrive through the controller in
     * the backend's own words, never as a wrapped call error.
     */
    import { onMount } from 'svelte';
    import CheckIcon from 'phosphor-svelte/lib/CheckIcon';
    import CircleAlertIcon from 'phosphor-svelte/lib/WarningCircleIcon';
    import CloudUploadIcon from 'phosphor-svelte/lib/CloudArrowUpIcon';
    import FolderPlusIcon from 'phosphor-svelte/lib/FolderPlusIcon';
    import FolderIcon from 'phosphor-svelte/lib/FolderIcon';
    import LockKeyholeIcon from 'phosphor-svelte/lib/LockKeyIcon';
    import PauseIcon from 'phosphor-svelte/lib/PauseIcon';
    import PlayIcon from 'phosphor-svelte/lib/PlayIcon';
    import RotateCwIcon from 'phosphor-svelte/lib/ArrowClockwiseIcon';
    import Trash2Icon from 'phosphor-svelte/lib/TrashIcon';
    import TriangleAlertIcon from 'phosphor-svelte/lib/WarningIcon';
    import Button from '../button.svelte';
    import IconButton from '../icon_button.svelte';
    import ProgressBar from '../progress_bar.svelte';
    import SwitchRow from '../switch_row.svelte';
    import type { PhotoBackupSettings } from '../../api/photo_backup';
    import {
        choosePhotoBackupFolder, deletePhotoBackupSource, pausePhotoBackupNow,
        photoBackupAccessNote, photoBackupBusy, photoBackupError, photoBackupState, refreshPhotoBackup,
        photoBackupFolderPicking, resumePhotoBackupNow, retryPhotoBackupNow, startPhotoBackup, updatePhotoBackupSettings,
    } from '../../modules/photo_backup/controller';
    import { action_label_key, canStart, describeBackup, destinationLabel, queueProgress, summaryLine, type BackupAction } from './photo_backup_view';
    import { current_locale, t_store } from '../../i18n/i18n';

    interface Props {
        /**
         * True where the panel is the whole screen rather than a section of
         * one: on a phone it is opened from Account like Appearance is, and
         * there it owns the gutters and needs no rule separating it from rows
         * that are no longer above it.
         */
        page?: boolean;
    }

    let { page = false }: Props = $props();

    onMount(() => { void refreshPhotoBackup(); });

    const backupState = photoBackupState;
    const busy = photoBackupBusy;
    const error = photoBackupError;
    const deviceAccessNote = photoBackupAccessNote;
    // Whether this host can be asked for a folder at all. The bridge is
    // installed before the app runs, so this is settled once.
    const folderPicking = photoBackupFolderPicking();

    // The copy is worded in the current language, so these re-derive on a language change.
    const situation = $derived.by(() => { void $current_locale; return $backupState ? describeBackup($backupState) : null; });
    const summary = $derived.by(() => { void $current_locale; return $backupState ? summaryLine($backupState.status) : ''; });
    const queue = $derived($backupState ? queueProgress($backupState.status) : null);
    const startable = $derived($backupState ? canStart($backupState) : false);
    const phone = $derived($backupState?.platform === 'android' || $backupState?.platform === 'ios');
    // iOS has no background execution, so backup only runs while TDrive is open.
    // The switch subtitle says so; Android's foreground service keeps the plain copy.
    const whileOpen = $derived($backupState?.platform === 'ios');
    // Only a restriction is worth a line; full access is the expected case.
    const accessNote = $derived(($backupState && !['', 'available', 'granted'].includes($backupState.capabilities.access.status)) ? $backupState.capabilities.access.detail : '');
    // Watched folders are walked all the way down and arrive in the drive with
    // their subfolders intact. An album has no tree, so this only applies where
    // a folder source is actually in the list.
    const nestsFolders = $derived(Boolean($backupState?.sources.some((source) => source.kind === 'folder' || source.kind === 'device-folder')));
    // A phone reads a watched folder through the media library, so it backs up
    // the photos and videos in it rather than every file. Said once, and only
    // where it is true.
    const mediaOnlyFolders = $derived(Boolean($backupState?.sources.some((source) => source.kind === 'device-folder')));
    // The device's own word on the media grant, which the backend cannot see.
    const accessHint = $derived(phone ? $deviceAccessNote : '');

    const handlers: Record<BackupAction, () => Promise<void>> = {
        start: startPhotoBackup, pause: pausePhotoBackupNow, resume: resumePhotoBackupNow, retry: retryPhotoBackupNow,
    };

    function save(change: Partial<PhotoBackupSettings>): void {
        if (!$backupState) return;
        void updatePhotoBackupSettings({ ...$backupState.settings, ...change });
    }

    function disabledFor(action: BackupAction): boolean {
        return $busy || (action === 'start' && !startable);
    }
</script>

<section class="photo-backup" class:is-page={page} aria-label={$t_store('photos.backup_card_label')} aria-busy={$busy}>
    {#if $backupState}
        {@const state = $backupState}
        <div class="pb-master pb-card">
            <SwitchRow
                title={$t_store('photos.switch_title')}
                description={state.settings.enabled ? destinationLabel(state.destination.title) || $t_store('photos.in_this_drive') : whileOpen ? $t_store('photos.switch_desc_open') : $t_store('photos.switch_desc')}
                checked={state.settings.enabled}
                disabled={$busy}
                onchange={(enabled) => save({ enabled })}
            />
        </div>

        {#if state.settings.enabled && situation}
            <div class="pb-situation" data-tone={situation.tone} role="status" aria-live="polite">
                <div class="pb-situation-icon" aria-hidden="true">
                    {#if situation.tone === 'busy'}
                        <span class="pb-spinner"></span>
                    {:else if situation.tone === 'locked'}
                        <LockKeyholeIcon size={16} />
                    {:else if situation.tone === 'danger'}
                        <CircleAlertIcon size={16} />
                    {:else if situation.tone === 'warning'}
                        <TriangleAlertIcon size={16} />
                    {:else if situation.tone === 'success'}
                        <CheckIcon size={16} />
                    {:else}
                        <CloudUploadIcon size={16} />
                    {/if}
                </div>
                <div class="pb-situation-copy">
                    <!-- State and count share the top line: one says what is
                         happening, the other says how much is left, and a
                         glance wants both before it wants anything else. -->
                    <div class="pb-situation-head">
                        <div class="pb-situation-title">{situation.title}</div>
                        {#if summary}<div class="pb-summary">{summary}</div>{/if}
                    </div>
                    {#if situation.body}
                        <div class="pb-situation-body" class:is-file={situation.tone === 'busy' && Boolean(state.status.currentFile)}>{situation.body}</div>
                    {/if}
                    {#if situation.progress}
                        <div class="pb-progress">
                            <!-- The queue's own progress, with the file it is
                                 on named above it. The counts are already on
                                 the line under the title, so the bar carries
                                 no label of its own. -->
                            {#if queue}
                                <ProgressBar value={queue.value} max={queue.max} />
                            {:else}
                                <ProgressBar indeterminate />
                            {/if}
                        </div>
                    {/if}
                    {#if situation.detail}
                        <!-- Closed by default: the cause is for whoever is
                             diagnosing, and it is the one string here whose
                             length nothing in this app controls. -->
                        <details class="pb-detail">
                            <summary>{$t_store('photos.details')}</summary>
                            <p>{situation.detail}</p>
                        </details>
                    {/if}
                </div>
                <!-- The actions sit outside the copy so a phone can run them
                     the full width of the card rather than indenting them
                     past the glyph. -->
                {#if situation.primary || situation.secondary}
                    <div class="pb-actions">
                        {#if situation.primary}
                            {@const primary = situation.primary}
                            <!-- Nothing is owed when the drive is up to date,
                                 so running one now is an option rather than a
                                 call to action, and it is drawn as one. -->
                            <Button variant={situation.tone === 'success' ? 'secondary' : 'primary'} size="sm" disabled={disabledFor(primary)} onclick={() => void handlers[primary]()}>
                                {#if primary === 'retry'}<RotateCwIcon size={14} aria-hidden="true" weight="bold" />{:else}<PlayIcon size={14} aria-hidden="true" weight="bold" />{/if}
                                {$t_store(action_label_key(primary, state.encryptionRequired))}
                            </Button>
                        {/if}
                        {#if situation.secondary}
                            {@const secondary = situation.secondary}
                            <Button variant="secondary" size="sm" disabled={disabledFor(secondary)} onclick={() => void handlers[secondary]()}>
                                <PauseIcon size={14} aria-hidden="true" weight="bold" />
                                {$t_store(action_label_key(secondary, false))}
                            </Button>
                        {/if}
                    </div>
                {/if}
            </div>

            <div class="pb-group" aria-label={$t_store('photos.sources_label')}>
                <div class="pb-group-head">
                    <span class="pb-group-label">{$t_store('photos.backing_up')}</span>
                    <!-- One way in, and it is the system's own picker: the same
                         one the app opens to upload a folder. -->
                    <div class="pb-group-actions">
                        {#if folderPicking}
                            <Button variant="secondary" size="sm" disabled={$busy} onclick={() => void choosePhotoBackupFolder()}>
                                <FolderPlusIcon size={14} aria-hidden="true" weight="bold" />
                                {$t_store('photos.add_folder')}
                            </Button>
                        {/if}
                    </div>
                </div>
                <ul class="pb-sources pb-card" role="list">
                    {#each state.sources as source (source.id)}
                        <li class="pb-source">
                            <FolderIcon class="pb-source-icon" size={16} aria-hidden="true" />
                            <span class="pb-source-name">{source.name}</span>
                            <IconButton label={$t_store('photos.remove_source', { name: source.name })} size="sm" disabled={$busy} onclick={() => void deletePhotoBackupSource(source.id)}>
                                <Trash2Icon size={15} />
                            </IconButton>
                        </li>
                    {:else}
                        <li class="pb-source pb-source-empty">{$t_store('photos.sources_empty')}</li>
                    {/each}
                </ul>
                {#if nestsFolders}
                    <p class="pb-note">{$t_store('photos.subfolders_note')}</p>
                {/if}
                {#if mediaOnlyFolders}
                    <p class="pb-note">{$t_store('photos.media_only_note')}</p>
                {/if}
                {#if accessHint}
                    <p class="pb-note">{accessHint}</p>
                {/if}
                {#if accessNote}
                    <p class="pb-note">{accessNote}</p>
                {/if}
            </div>

            <div class="pb-group" aria-label={$t_store('photos.backup_options_label')}>
                <span class="pb-group-label">{$t_store('photos.include')}</span>
                <div class="pb-card">
                    <SwitchRow title={$t_store('photos.photos_switch')} checked={state.settings.photos} disabled={$busy} onchange={(photos) => save({ photos })} />
                    <SwitchRow title={$t_store('photos.videos_switch')} checked={state.settings.videos} disabled={$busy} onchange={(videos) => save({ videos })} />
                    {#if state.capabilities.wifiOnly.supported}
                        <SwitchRow title={$t_store('photos.wifi_only')} description={$t_store('photos.wifi_only_note')} checked={state.settings.wifiOnly} disabled={$busy} onchange={(wifiOnly) => save({ wifiOnly })} />
                    {/if}
                </div>
            </div>
        {/if}
    {:else if $error}
        <!-- The settings never arrived. The error line below says so; this
             is the one thing to do about it, rather than a loading line that
             will never resolve. -->
        <div class="pb-actions">
            <Button variant="secondary" size="sm" disabled={$busy} onclick={() => void refreshPhotoBackup()}>
                <RotateCwIcon size={14} aria-hidden="true" weight="bold" />
                {$t_store('photos.try_again')}
            </Button>
        </div>
    {:else}
        <p class="pb-loading" role="status">{$t_store('photos.loading_settings')}</p>
    {/if}

    {#if $error}
        <p class="pb-error" role="alert">
            <CircleAlertIcon size={14} aria-hidden="true" weight="bold" />
            <span>{$error}</span>
        </p>
    {/if}
</section>

<style>
    .photo-backup {
        box-sizing: border-box;
        display: grid;
        gap: var(--space-4);
        padding: var(--space-3) var(--space-3) var(--space-4);
        color: var(--text-main);
    }

    .pb-master {
        padding-bottom: var(--space-2);
        border-bottom: 1px solid var(--border);
    }

    /* The situation block. Its tone colours one ring and one glyph; the copy
       stays neutral so a warning never shouts. */
    .pb-situation {
        display: grid;
        grid-template-columns: auto minmax(0, 1fr);
        gap: var(--space-3);
        align-items: start;
    }

    .pb-situation-icon {
        width: 32px;
        height: 32px;
        display: grid;
        place-items: center;
        border: 1px solid var(--border);
        border-radius: var(--radius-full);
        color: var(--text-muted);
        background: var(--surface-control);
    }

    .pb-situation[data-tone='busy'] .pb-situation-icon,
    .pb-situation[data-tone='locked'] .pb-situation-icon {
        color: var(--accent);
        border-color: color-mix(in srgb, var(--accent) 42%, transparent);
        background: color-mix(in srgb, var(--accent) 12%, transparent);
    }

    .pb-situation[data-tone='success'] .pb-situation-icon {
        color: var(--success);
        border-color: color-mix(in srgb, var(--success) 42%, transparent);
        background: color-mix(in srgb, var(--success) 12%, transparent);
    }

    .pb-situation[data-tone='warning'] .pb-situation-icon {
        color: var(--color-warning);
        border-color: color-mix(in srgb, var(--color-warning) 42%, transparent);
        background: color-mix(in srgb, var(--color-warning) 12%, transparent);
    }

    .pb-situation[data-tone='danger'] .pb-situation-icon {
        color: var(--danger);
        border-color: color-mix(in srgb, var(--danger) 42%, transparent);
        background: color-mix(in srgb, var(--danger) 12%, transparent);
    }

    .pb-spinner {
        width: 14px;
        height: 14px;
        border-radius: var(--radius-full);
        border: 2px solid color-mix(in srgb, var(--accent) 30%, transparent);
        border-top-color: var(--accent);
        animation: pb-spin 720ms linear infinite;
    }

    @keyframes pb-spin {
        to { transform: rotate(360deg); }
    }

    .pb-situation-copy {
        min-width: 0;
        display: grid;
        gap: var(--space-1);
        padding-top: 5px;
    }

    /* State on the left, count on the right, baselines aligned. The count is
       allowed to shrink to nothing before the state does: "Backing up" with no
       figure still reads, a figure with no state does not. */
    .pb-situation-head {
        display: flex;
        align-items: baseline;
        justify-content: space-between;
        gap: var(--space-2);
        min-width: 0;
    }

    .pb-situation-title {
        font-size: var(--font-size-sm);
        font-weight: 600;
        line-height: 1.35;
        min-width: 0;
    }

    .pb-situation-head .pb-summary {
        flex: 0 1 auto;
        text-align: right;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .pb-situation-body {
        color: var(--text-muted);
        font-size: var(--font-size-xs);
        line-height: 1.45;
    }

    /* A file name is one line and gets cut from the end, where the counter is. */
    .pb-situation-body.is-file {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        font-variant-numeric: tabular-nums;
    }

    .pb-progress {
        margin-top: var(--space-1);
    }

    .pb-summary {
        color: var(--text-muted);
        font-size: var(--font-size-xs);
        font-variant-numeric: tabular-nums;
    }

    /* The cause, for whoever wants it. Closed it costs one short line; open it
       is the only text here whose length nothing in this app controls, so it
       scrolls rather than pushing the controls off the screen. */
    .pb-detail {
        margin-top: var(--space-1);
        font-size: var(--font-size-xs);
    }

    .pb-detail > summary {
        display: inline-flex;
        align-items: center;
        gap: 4px;
        width: fit-content;
        padding: 2px 0;
        color: var(--text-muted);
        cursor: pointer;
        list-style: none;
        border-radius: var(--radius-sm);
    }

    .pb-detail > summary::-webkit-details-marker { display: none; }

    .pb-detail > summary::before {
        content: '';
        width: 0;
        height: 0;
        border-left: 4px solid currentColor;
        border-top: 3.5px solid transparent;
        border-bottom: 3.5px solid transparent;
        transition: transform var(--motion-fast) var(--ease-standard);
    }

    .pb-detail[open] > summary::before { transform: rotate(90deg); }
    .pb-detail > summary:hover { color: var(--text-main); }
    .pb-detail > summary:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }

    .pb-detail > p {
        margin: var(--space-1) 0 0;
        max-height: 8.5em;
        overflow-y: auto;
        overscroll-behavior: contain;
        color: var(--text-muted);
        line-height: 1.5;
        /* A backend cause is a path and an errno; let it break anywhere rather
           than widen the panel. */
        overflow-wrap: anywhere;
    }

    @media (prefers-reduced-motion: reduce) {
        .pb-detail > summary::before { transition: none; }
    }

    .pb-actions {
        grid-column: 2;
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
        margin-top: var(--space-2);
    }

    .pb-group {
        display: grid;
        gap: var(--space-1);
        padding-top: var(--space-3);
        border-top: 1px solid var(--border);
    }

    .pb-group-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
        min-height: 32px;
    }

    /* Two ways in on a phone -- albums and a folder -- so they wrap rather than
       squeeze the label off the row on a narrow screen. */
    .pb-group-actions {
        display: flex;
        flex-wrap: wrap;
        justify-content: flex-end;
        gap: var(--space-2);
    }

    .pb-group-label {
        color: var(--text-muted);
        font-size: 11px;
        font-weight: var(--weight-strong);
        letter-spacing: 0.04em;
        text-transform: uppercase;
    }

    .pb-sources {
        margin: 0;
        padding: 0;
        list-style: none;
        display: grid;
    }

    .pb-source {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-height: 36px;
        font-size: var(--font-size-sm);
    }

    .pb-source + .pb-source {
        border-top: 1px solid var(--border);
    }

    .pb-source :global(.pb-source-icon) {
        flex: 0 0 auto;
        color: var(--text-muted);
    }

    .pb-source-name {
        flex: 1 1 auto;
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .pb-source-empty {
        color: var(--text-muted);
        font-size: var(--font-size-xs);
    }

    .pb-note,
    .pb-loading {
        margin: 0;
        color: var(--text-muted);
        font-size: var(--font-size-xs);
        line-height: 1.45;
    }

    .pb-error {
        display: flex;
        align-items: flex-start;
        gap: var(--space-2);
        margin: 0;
        color: var(--danger);
        font-size: var(--font-size-xs);
        line-height: 1.45;
    }

    .pb-error :global(svg) {
        flex: 0 0 auto;
        margin-top: 2px;
    }

    @media (prefers-reduced-motion: reduce) {
        .pb-spinner {
            animation: none;
        }
    }

    /* Phone: the card's own rhythm -- 14px gutters, 52px rows, the mobile type
       scale -- and every control a full-height tap target. */
    :global(html.mobile) .photo-backup {
        gap: var(--space-3);
        padding: 0 14px 14px;
        border-top: 1px solid var(--color-border-soft);
    }

    :global(html.mobile) .photo-backup.is-page {
        padding: 0;
        border-top: 0;
    }

    :global(html.mobile) .pb-master {
        padding-bottom: 0;
        border-bottom: 0;
    }

    /* The same card the account list is made of: a solid surface with a
       hairline, not a pale wash over the page. A translucent overlay read as a
       different app, and the quiet action inside it looked disabled against
       the lighter ground. */
    :global(html.mobile) .pb-situation {
        padding: var(--space-3);
        border: 1px solid var(--border);
        border-radius: var(--radius-lg);
        background: var(--surface-control);
    }

    :global(html.mobile) .pb-situation-title {
        font-size: var(--mobile-type-body);
    }

    /* A phone has no room for a state and a count on one line: "Ready to back
       up" wrapped onto two while "41 backed up · 14 waiting" was cut off mid
       word beside it. Stacked, both are read in full and the block is shorter
       than the wrap it replaces. */
    :global(html.mobile) .pb-situation-head {
        flex-direction: column;
        align-items: stretch;
        gap: 2px;
    }

    :global(html.mobile) .pb-situation-head .pb-summary {
        text-align: left;
        white-space: normal;
        overflow: visible;
    }

    /* One action fills the width; two share it. Either way they line up with
       the card rather than sitting in the middle of it, and every one of them
       is a full-width tap target. */
    :global(html.mobile) .pb-actions {
        grid-column: 1 / -1;
        display: grid;
        grid-auto-flow: column;
        grid-auto-columns: minmax(0, 1fr);
        gap: var(--space-2);
        margin-top: var(--space-3);
    }

    :global(html.mobile) .pb-situation-body,
    :global(html.mobile) .pb-summary,
    :global(html.mobile) .pb-note,
    :global(html.mobile) .pb-source-empty {
        font-size: var(--mobile-type-meta);
    }

    /* The label keeps its own row: beside two buttons it was wrapping to two
       lines on a 390px screen and the buttons were staggering down the side
       of it. */
    :global(html.mobile) .pb-group-head {
        display: grid;
        gap: var(--space-2);
        min-height: 0;
    }

    :global(html.mobile) .pb-group-actions {
        display: grid;
        grid-auto-flow: column;
        grid-auto-columns: minmax(0, 1fr);
        justify-content: stretch;
        gap: var(--space-2);
    }

    /* Each group is a card with its label above it: the shape every other list
       on this tab is built from. On a desktop the panel is a section of a menu
       and keeps its rules and gutters. */
    :global(html.mobile) .pb-card {
        padding: 0 var(--space-3);
        border-radius: var(--radius-lg);
        background: var(--surface-control);
    }

    :global(html.mobile) .pb-group {
        gap: var(--space-2);
        padding-top: 0;
        border-top: 0;
    }

    :global(html.mobile) .pb-source-empty {
        padding: var(--space-3) 0;
    }

    :global(html.mobile) .pb-source {
        min-height: 48px;
        font-size: var(--mobile-type-body);
    }

    :global(html.mobile) .pb-actions :global(.ui-button),
    :global(html.mobile) .pb-group-head :global(.ui-button) {
        min-height: 44px;
        padding: 0 var(--space-4);
        font-size: var(--mobile-type-meta);
    }

    /* A full tap target, without the outline: on a phone the row is the thing
       being read and a boxed bin beside one folder name drew the eye first. */
    :global(html.mobile) .pb-source :global(.ui-icon-button) {
        width: 44px;
        height: 44px;
        border-color: transparent;
    }
</style>
