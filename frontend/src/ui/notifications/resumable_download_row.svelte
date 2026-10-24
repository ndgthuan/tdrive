<script lang="ts">
    import ArrowDownIcon from 'phosphor-svelte/lib/ArrowDownIcon';
    import { isAndroidPlatform } from '../../api';
    import type { ResumableDownload } from '../../api/resumable_downloads';
    import { discardDownload, pauseDownload, resumePausedDownload, saveRecoveredDownload } from '../../modules/resumable_downloads';
    import { canSaveToDownloads } from '../../modules/android_downloads';
    import { formatBytes } from '../../utils';
    import { formatEta, formatSizePair } from './transfer_view';
    import { t_store } from '../../i18n/i18n';

    interface Props { job: ResumableDownload; mobile?: boolean }
    let { job, mobile = false }: Props = $props();
    let busy = $state(false);
    let confirmingDiscard = $state(false);
    let confirmationStatus = $state<string | null>(null);

    $effect(() => {
        if (job.status === confirmationStatus) return;
        confirmationStatus = job.status;
        confirmingDiscard = false;
    });

    const percent = $derived(job.totalBytes > 0 ? Math.min(100, job.verifiedBytes / job.totalBytes * 100) : 0);
    const stateLabel = $derived(
        job.status === 'downloading' ? $t_store('activity.dir_downloading')
        : job.status === 'waiting_network' ? $t_store('activity.state_waiting_network')
        : job.status === 'waiting_unlock' ? $t_store('activity.state_waiting_unlock')
        : job.status === 'needs_destination' ? $t_store('activity.state_needs_destination')
        : job.status === 'source_changed' ? $t_store('activity.state_source_changed')
        : job.status === 'verifying' ? $t_store('activity.state_verifying')
        : job.status === 'saving' ? $t_store('activity.state_saving')
        : job.status === 'completed' ? $t_store('activity.state_completed')
        : job.status === 'error' ? $t_store('activity.needs_attention')
        : $t_store('activity.status_paused'),
    );
    const rate = $derived(job.status === 'downloading' && job.speed > 0 ? `${formatBytes(job.speed)}/s` : '');
    const eta = $derived(job.status === 'downloading' && job.speed > 0 && percent >= 5
        && (job.totalBytes - job.verifiedBytes) / job.speed <= 24 * 60 * 60
        ? formatEta((job.totalBytes - job.verifiedBytes) / job.speed)
        : '');
    const detail = $derived(`${$t_store('activity.download_detail', { state: stateLabel, size: formatSizePair(job.verifiedBytes, job.totalBytes) })}${rate ? ` · ${rate}` : ''}${eta ? ` · ${eta}` : ''}`);
    const guidance = $derived(
        job.status === 'waiting_network' ? $t_store('activity.guide_waiting_network')
        : job.status === 'waiting_unlock' ? $t_store('activity.guide_waiting_unlock')
        : job.status === 'needs_destination' ? $t_store('activity.guide_needs_destination')
        : job.status === 'source_changed' ? $t_store('activity.guide_source_changed')
        : job.status === 'completed' && isAndroidPlatform()
            ? canSaveToDownloads() ? $t_store('activity.guide_completed_android') : $t_store('activity.guide_cannot_save')
        : job.error,
    );

    async function act(action: () => Promise<void>): Promise<void> {
        if (busy) return;
        busy = true;
        try { await action(); }
        finally { busy = false; }
    }
</script>

<div class:mobile class="download-row" data-phase={job.status} role={mobile ? 'listitem' : undefined}>
    <span class="glyph" aria-hidden="true"><ArrowDownIcon size={mobile ? 15 : 14} /></span>
    <div class="name" title={job.name}>{job.name || $t_store('activity.download_default')}</div>
    <div
        class="track"
        role="progressbar"
        aria-label={$t_store('activity.download_bytes_aria', { name: job.name || $t_store('activity.download_default') })}
        aria-valuemin="0"
        aria-valuemax="100"
        aria-valuenow={Math.round(percent)}
    ><div class="fill" style={`width:${percent}%`} aria-hidden="true"></div></div>
    <div class="detail">{detail}</div>
    {#if guidance}<p class="note">{guidance}</p>{/if}
    <div class="actions">
        {#if job.status === 'downloading' || job.status === 'verifying' || job.status === 'saving'}
            <button type="button" disabled={busy} aria-label={$t_store('activity.pause_aria', { name: job.name })} onclick={() => void act(() => pauseDownload(job.jobId))}>{$t_store('activity.pause')}</button>
        {:else if job.status === 'completed'}
            {#if isAndroidPlatform() && canSaveToDownloads()}
                <button type="button" disabled={busy} aria-label={$t_store('activity.save_aria', { name: job.name })} onclick={() => void act(() => saveRecoveredDownload(job.jobId))}>{$t_store('activity.save_downloads')}</button>
            {/if}
        {:else if job.status !== 'source_changed'}
            <button type="button" disabled={busy} aria-label={job.status === 'needs_destination' ? $t_store('activity.choose_location_aria', { name: job.name }) : $t_store('activity.resume_aria', { name: job.name })} onclick={() => void act(() => resumePausedDownload(job.jobId))}>
                {job.status === 'needs_destination' ? $t_store('activity.choose_location') : job.status === 'waiting_network' || job.status === 'error' ? $t_store('activity.retry_now') : $t_store('activity.resume')}
            </button>
        {/if}
        {#if job.status !== 'downloading' && job.status !== 'verifying' && job.status !== 'saving' && job.status !== 'completed'}
            {#if confirmingDiscard}
                <button type="button" disabled={busy} onclick={() => confirmingDiscard = false}>{$t_store('activity.keep')}</button>
                <button type="button" class="danger" disabled={busy} aria-label={$t_store('activity.discard_bytes_aria', { name: job.name })} onclick={() => void act(() => discardDownload(job.jobId))}>{$t_store('activity.discard_bytes')}</button>
            {:else}
                <button type="button" class="danger" disabled={busy} aria-label={$t_store('activity.discard_aria', { name: job.name })} onclick={() => confirmingDiscard = true}>{$t_store('activity.discard')}</button>
            {/if}
        {/if}
    </div>
</div>

<style>
    .download-row { display: grid; grid-template-columns: 22px minmax(0, 1fr); gap: 7px 10px; padding: 10px 12px; align-items: center; }
    .glyph { display: grid; place-items: center; width: 22px; height: 22px; color: var(--color-accent); }
    .name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: var(--weight-medium); }
    .track, .detail, .note, .actions { grid-column: 2 / -1; }
    .track { height: 5px; border-radius: var(--radius-pill); background: var(--overlay-neutral-2); overflow: hidden; }
    .fill { height: 100%; border-radius: inherit; background: var(--color-accent); transition: width var(--motion-med) var(--ease-standard); }
    .detail, .note { margin: 0; color: var(--color-text-muted); font-size: 12px; line-height: 1.45; }
    .detail { font-variant-numeric: tabular-nums; }
    .actions { display: flex; flex-wrap: wrap; gap: 6px; }
    .actions button { min-height: 28px; padding: 4px 8px; border: 0; border-radius: var(--radius-md); background: var(--overlay-accent-1); color: var(--color-accent); font: inherit; cursor: pointer; }
    .actions button.danger { background: var(--overlay-danger-1); color: var(--color-danger); }
    .actions button:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 2px; }
    .actions button:disabled { opacity: .55; cursor: wait; }
    .mobile { grid-template-columns: 28px minmax(0, 1fr); gap: 7px 12px; padding: 12px 14px; }
    .mobile .glyph { width: 28px; height: 28px; border-radius: var(--radius-pill); background: var(--overlay-accent-1); }
    .mobile .name { font-size: var(--mobile-type-body); }
    .mobile .detail { font-size: var(--mobile-type-meta); }
    .mobile .note { font-size: var(--mobile-type-caption); }
    .mobile .actions button { min-height: 44px; padding: 0 12px; font-size: var(--mobile-type-meta); }
    @media (prefers-reduced-motion: reduce) { .fill { transition: none; } }
</style>
