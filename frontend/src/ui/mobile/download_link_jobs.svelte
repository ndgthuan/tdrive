<!-- Copyright (c) 2026 Nguyễn Dương Gia Thuận -->

<script lang="ts">
    // Link downloads in the phone's Transfers tab. Each row shows one yt-dlp job
    // with its stage and progress; an active job can be cancelled from here.
    import XIcon from 'phosphor-svelte/lib/XIcon';
    import { onMount } from 'svelte';
    import { t_store } from '../../i18n/i18n';
    import { cancel_mobile_download, start_mobile_downloads } from '../../modules/media_download/mobile_jobs';
    import { native_engine_present } from '../../modules/media_download/native_engine';
    import { clamp_percent, is_active_stage, stage_key } from '../downloader/downloader_format';
    import { download_rows, type download_row } from '../downloader/downloader_state';

    const MAX_ROWS = 20;
    const available = native_engine_present();
    const rows = $derived($download_rows.slice(0, MAX_ROWS));

    onMount(() => {
        if (available) start_mobile_downloads();
    });

    function label_of(row: download_row): string {
        return row.title || row.url;
    }

    async function cancel(row: download_row): Promise<void> {
        try {
            await cancel_mobile_download(row.job_id);
        } catch {
            // The job's own stage update shows the outcome; nothing else to report here.
        }
    }
</script>

{#if available && rows.length > 0}
    <section class="jobs" aria-label={$t_store('mobile.download_transfers_title')}>
        <p class="sec">{$t_store('mobile.download_transfers_title')}</p>
        <ul class="list">
            {#each rows as row (row.job_id)}
                <li class="item">
                    <div class="line">
                        <span class="name">{label_of(row)}</span>
                        <span class="stage">{$t_store(stage_key(row.stage))}</span>
                        {#if is_active_stage(row.stage)}
                            <button type="button" class="ghost" aria-label={$t_store('downloader.cancel')} onclick={() => cancel(row)}>
                                <XIcon size={16} aria-hidden="true" />
                            </button>
                        {/if}
                    </div>
                    {#if is_active_stage(row.stage)}
                        <div class="bar" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={clamp_percent(row.percent)}>
                            <i style="width: {clamp_percent(row.percent)}%"></i>
                        </div>
                    {/if}
                    {#if row.error}<p class="error">{row.error}</p>{/if}
                </li>
            {/each}
        </ul>
    </section>
{/if}

<style>
    .jobs {
        display: flex;
        flex-direction: column;
        gap: 8px;
        padding: 0 16px;
    }

    .sec {
        margin: 0;
        font-size: 12px;
        font-weight: 600;
        letter-spacing: 0.06em;
        text-transform: uppercase;
        color: var(--color-text-muted);
    }

    .list {
        list-style: none;
        margin: 0;
        padding: 0;
        background: var(--color-surface-1);
        border-radius: 16px;
        overflow: hidden;
    }

    .item {
        display: flex;
        flex-direction: column;
        gap: 8px;
        padding: 12px 14px;
        border-top: 1px solid var(--color-border-soft);
    }

    .item:first-child {
        border-top: 0;
    }

    .line {
        display: flex;
        align-items: center;
        gap: 10px;
        min-width: 0;
    }

    .name {
        flex: 1;
        min-width: 0;
        font-size: 15px;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    .stage {
        font-size: 13px;
        color: var(--color-text-muted);
        white-space: nowrap;
    }

    .ghost {
        width: 32px;
        height: 32px;
        display: grid;
        place-items: center;
        border: 0;
        border-radius: 16px;
        background: transparent;
        color: var(--color-text-muted);
        cursor: pointer;
        flex: none;
    }

    .bar {
        height: 6px;
        border-radius: 3px;
        background: var(--color-border-soft);
        overflow: hidden;
    }

    .bar i {
        display: block;
        height: 100%;
        background: var(--color-accent);
        border-radius: 3px;
    }

    .error {
        margin: 0;
        font-size: 13px;
        color: var(--color-danger);
    }
</style>
