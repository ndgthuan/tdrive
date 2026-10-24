<script lang="ts">
    import ArrowUpIcon from 'phosphor-svelte/lib/ArrowUpIcon';
    import type { ResumableUpload } from '../../api/resumable_uploads';
    import { checkUploadStatus, chooseSourceAndResume, discardUpload, pauseUpload, resumeUpload } from '../../modules/resumable_uploads';
    import { formatSizePair } from './transfer_view';
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
        job.status === 'uploading' ? $t_store('activity.dir_uploading')
        : job.status === 'paused' ? $t_store('activity.status_paused')
        : job.status === 'waiting_network' ? $t_store('activity.state_waiting_network')
        : job.status === 'needs_source' ? $t_store('activity.state_needs_source')
        : job.status === 'uncertain' ? $t_store('activity.state_uncertain')
        : job.status === 'uncertain_manifest' ? $t_store('activity.state_uncertain_manifest')
        : job.status === 'restart_required' ? $t_store('activity.state_restart_required')
        : job.status === 'canceling' ? $t_store('activity.state_canceling')
        : $t_store('activity.status_paused'),
    );
    const detail = $derived($t_store('activity.upload_detail', { state: stateLabel, size: formatSizePair(job.confirmedBytes, job.size) }));
    const guidance = $derived(
        job.status === 'needs_source'
            ? $t_store('activity.upload_guide_needs_source')
            : job.status === 'waiting_network'
                ? $t_store('activity.upload_guide_waiting_network')
            : job.status === 'uncertain_manifest'
                ? $t_store('activity.upload_guide_uncertain_manifest')
                : job.status === 'uncertain'
                    ? $t_store('activity.upload_guide_uncertain')
                : job.status === 'restart_required'
                    ? $t_store('activity.upload_guide_restart')
                : '',
    );

    async function act(action: () => Promise<void>): Promise<void> {
        if (busy) return;
        busy = true;
        try { await action(); }
        finally { busy = false; }
    }
</script>

<div class="notif-row notif-row-transfer resumable-row" data-phase={job.status}>
    <span class="notif-row-icon" data-kind="upload" aria-hidden="true">
        <ArrowUpIcon size={14} aria-hidden="true" weight="bold" />
    </span>
    <div class="notif-row-body">
        <div class="notif-row-title" title={job.name}>{job.name || $t_store('activity.large_upload')}</div>
        <div
            class="notif-row-progress"
            role="progressbar"
            aria-label={$t_store('activity.upload_confirmed_aria', { name: job.name || $t_store('activity.large_upload') })}
            aria-valuemin="0"
            aria-valuemax="100"
            aria-valuenow={Math.round(percent)}
        >
            <div class="notif-row-progress-fill" style={`width:${percent}%`} aria-hidden="true"></div>
        </div>
        {#if guidance}<div class="notif-row-note">{guidance}</div>{/if}
    </div>
    <div class="notif-row-meta"><div class="notif-row-state">{detail}</div></div>
    {#if job.status !== 'canceling'}
        <div class="actions">
            {#if job.status === 'paused' || job.status === 'uncertain' || job.status === 'waiting_network'}
                <button class="notif-row-copy" type="button" disabled={busy} aria-label={job.status === 'waiting_network' ? $t_store('activity.retry_aria', { name: job.name }) : $t_store('activity.resume_aria', { name: job.name })} onclick={() => resumeUpload(job.jobId)}>{job.status === 'waiting_network' ? $t_store('activity.retry_now') : $t_store('activity.resume')}</button>
            {:else if job.status === 'needs_source'}
                <button class="notif-row-copy" type="button" disabled={busy} onclick={() => chooseSourceAndResume(job.jobId)}>{$t_store('activity.choose_file')}</button>
            {:else if job.status === 'uploading'}
                <button class="notif-row-copy" type="button" disabled={busy} onclick={() => void act(() => pauseUpload(job.jobId))}>{$t_store('activity.pause')}</button>
            {:else if job.status === 'uncertain_manifest'}
                <button class="notif-row-copy" type="button" disabled={busy} onclick={() => void act(() => checkUploadStatus(job.jobId))}>{busy ? $t_store('activity.checking') : $t_store('activity.check_retry')}</button>
            {/if}
            {#if job.status !== 'uploading' && job.status !== 'uncertain_manifest'}
                {#if confirmingDiscard}
                    <button class="notif-row-copy" type="button" disabled={busy} onclick={() => confirmingDiscard = false}>{$t_store('activity.keep')}</button>
                    <button class="notif-row-copy danger" type="button" disabled={busy} onclick={() => void act(() => discardUpload(job.jobId))}>{$t_store('activity.discard_parts')}</button>
                {:else}
                    <button class="notif-row-copy danger" type="button" disabled={busy} onclick={() => confirmingDiscard = true}>{$t_store('activity.discard')}</button>
                {/if}
            {/if}
        </div>
    {/if}
</div>

<style>
    .resumable-row { grid-template-columns: 22px minmax(0, 1fr) auto; row-gap: 8px; }
    .resumable-row .notif-row-meta { grid-column: 2 / -1; margin-left: 0; align-items: flex-start; }
    .resumable-row .notif-row-body { grid-column: 2 / -1; }
    .resumable-row .notif-row-progress { margin-top: 2px; }
    .resumable-row .notif-row-state { color: var(--color-text-soft); font-variant-numeric: tabular-nums; }
    .actions { grid-column: 2 / -1; display: flex; flex-wrap: wrap; gap: 6px; }
    .actions .notif-row-copy { min-height: 28px; }
    .actions .danger { color: var(--color-danger); }
    .actions .danger:hover { background: var(--overlay-danger-1); }
    .actions .notif-row-copy:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 2px; }
    .actions .notif-row-copy:disabled { opacity: .55; cursor: wait; }
</style>
