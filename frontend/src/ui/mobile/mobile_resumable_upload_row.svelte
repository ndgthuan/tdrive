<script lang="ts">
    import ArrowUpIcon from 'phosphor-svelte/lib/ArrowUpIcon';
    import type { ResumableUpload } from '../../api/resumable_uploads';
    import {
        checkUploadStatus, chooseSourceAndResume, discardUpload, pauseUpload, resumeUpload,
    } from '../../modules/resumable_uploads';
    import { formatSizePair } from '../notifications/transfer_view';
    import { t_store } from '../../i18n/i18n';

    interface Props { job: ResumableUpload }
    let { job }: Props = $props();
    let busy = $state(false);
    let confirmingDiscard = $state(false);
    let confirmationStatus = $state<string | null>(null);

    $effect(() => {
        if (job.status === confirmationStatus) return;
        confirmationStatus = job.status;
        confirmingDiscard = false;
    });

    const percent = $derived(job.size > 0 ? Math.min(100, job.confirmedBytes / job.size * 100) : 0);
    const stateLabel = $derived(
        job.status === 'uploading' ? $t_store('mobile.upload_state_uploading')
        : job.status === 'paused' ? $t_store('mobile.upload_state_paused')
        : job.status === 'waiting_network' ? $t_store('mobile.upload_state_waiting_network')
        : job.status === 'needs_source' ? $t_store('mobile.upload_state_needs_source')
        : job.status === 'uncertain' ? $t_store('mobile.upload_state_uncertain')
        : job.status === 'uncertain_manifest' ? $t_store('mobile.upload_state_uncertain_manifest')
        : job.status === 'restart_required' ? $t_store('mobile.upload_state_restart_required')
        : job.status === 'canceling' ? $t_store('mobile.upload_state_canceling')
        : $t_store('mobile.upload_state_paused'),
    );
    const guidance = $derived(
        job.status === 'needs_source'
            ? $t_store('mobile.upload_guidance_needs_source')
            : job.status === 'waiting_network'
                ? $t_store('mobile.upload_guidance_waiting_network')
            : job.status === 'uncertain_manifest'
                ? $t_store('mobile.upload_guidance_uncertain_manifest')
                : job.status === 'uncertain'
                    ? $t_store('mobile.upload_guidance_uncertain')
                    : job.status === 'restart_required'
                        ? $t_store('mobile.upload_guidance_restart_required')
                        : '',
    );
    const display_name = $derived(job.name || $t_store('mobile.upload_untitled'));

    async function act(action: () => Promise<void>): Promise<void> {
        if (busy) return;
        busy = true;
        try { await action(); }
        finally { busy = false; }
    }
</script>

<div class="row resumable-row" data-phase={job.status} role="listitem">
    <span class="glyph" aria-hidden="true"><ArrowUpIcon size={15} /></span>
    <div class="name" title={job.name}>{display_name}</div>
    <div
        class="track"
        role="progressbar"
        aria-label={$t_store('mobile.upload_progress_label', { name: display_name })}
        aria-valuemin="0"
        aria-valuemax="100"
        aria-valuenow={Math.round(percent)}
    >
        <div class="fill" style={`width:${percent}%`} aria-hidden="true"></div>
    </div>
    <div class="detail">{$t_store('mobile.upload_detail', { state: stateLabel, size: formatSizePair(job.confirmedBytes, job.size) })}</div>
    {#if guidance}<p class="note">{guidance}</p>{/if}
    {#if job.status !== 'canceling'}
        <div class="actions">
            {#if job.status === 'paused' || job.status === 'uncertain' || job.status === 'waiting_network'}
                <button type="button" disabled={busy} aria-label={$t_store('mobile.upload_resume_aria', { name: job.name })} onclick={() => resumeUpload(job.jobId)}>{job.status === 'waiting_network' ? $t_store('mobile.upload_retry_now') : $t_store('mobile.upload_resume')}</button>
            {:else if job.status === 'needs_source'}
                <button type="button" disabled={busy} aria-label={$t_store('mobile.upload_choose_file_aria', { name: job.name })} onclick={() => chooseSourceAndResume(job.jobId)}>{$t_store('mobile.upload_choose_file')}</button>
            {:else if job.status === 'uploading'}
                <button type="button" disabled={busy} aria-label={$t_store('mobile.upload_pause_aria', { name: job.name })} onclick={() => void act(() => pauseUpload(job.jobId))}>{$t_store('mobile.upload_pause')}</button>
            {:else if job.status === 'uncertain_manifest'}
                <button type="button" disabled={busy} aria-label={$t_store('mobile.upload_check_aria', { name: job.name })} onclick={() => void act(() => checkUploadStatus(job.jobId))}>{busy ? $t_store('mobile.upload_checking') : $t_store('mobile.upload_check')}</button>
            {/if}
            {#if job.status !== 'uploading' && job.status !== 'uncertain_manifest'}
                {#if confirmingDiscard}
                    <button type="button" disabled={busy} aria-label={$t_store('mobile.upload_keep_aria', { name: job.name })} onclick={() => confirmingDiscard = false}>{$t_store('mobile.upload_keep')}</button>
                    <button type="button" class="danger" disabled={busy} aria-label={$t_store('mobile.upload_discard_parts_aria', { name: job.name })} onclick={() => void act(() => discardUpload(job.jobId))}>{$t_store('mobile.upload_discard_parts')}</button>
                {:else}
                    <button type="button" class="danger" disabled={busy} aria-label={$t_store('mobile.upload_discard_aria', { name: job.name })} onclick={() => confirmingDiscard = true}>{$t_store('mobile.upload_discard')}</button>
                {/if}
            {/if}
        </div>
    {/if}
</div>

<style>
    .row {
        display: grid;
        grid-template-columns: 28px minmax(0, 1fr);
        align-items: start;
        column-gap: 12px;
        row-gap: 7px;
        padding: 12px 14px;
    }
    .glyph {
        display: grid;
        place-items: center;
        width: 28px;
        height: 28px;
        margin-top: 1px;
        border-radius: var(--radius-pill);
        background: var(--overlay-accent-1);
        color: var(--color-accent);
    }
    .row[data-phase='restart_required'] .glyph,
    .row[data-phase='uncertain_manifest'] .glyph {
        background: var(--overlay-danger-1);
        color: var(--color-danger);
    }
    .name {
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        font-size: var(--mobile-type-body);
        font-weight: var(--weight-medium);
        color: var(--color-text);
    }
    .track, .detail, .note, .actions { grid-column: 2 / -1; }
    .track {
        height: 5px;
        border-radius: var(--radius-pill);
        background: var(--overlay-neutral-2);
        overflow: hidden;
    }
    .fill {
        height: 100%;
        border-radius: inherit;
        background: var(--color-accent);
        transition: width var(--motion-med) var(--ease-standard);
    }
    .detail, .note {
        margin: 0;
        font-size: var(--mobile-type-meta);
        color: var(--color-text-muted);
    }
    .detail { font-variant-numeric: tabular-nums; }
    .note { font-size: var(--mobile-type-caption); line-height: 1.45; }
    .actions { display: flex; flex-wrap: wrap; gap: 4px 8px; margin-top: 1px; }
    .actions button {
        min-height: 44px;
        padding: 0 12px;
        border: 0;
        border-radius: var(--radius-md);
        background: var(--overlay-accent-1);
        color: var(--color-accent);
        font: inherit;
        font-size: var(--mobile-type-meta);
        font-weight: var(--weight-medium);
        cursor: pointer;
    }
    .actions button.danger { background: var(--overlay-danger-1); color: var(--color-danger); }
    .actions button:active { filter: brightness(.94); }
    .actions button:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 2px; }
    .actions button:disabled { opacity: .55; cursor: wait; }
    @media (prefers-reduced-motion: reduce) { .fill { transition: none; } }
</style>
