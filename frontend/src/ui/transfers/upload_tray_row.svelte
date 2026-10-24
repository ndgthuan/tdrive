<script lang="ts">
    import CheckIcon from 'phosphor-svelte/lib/CheckIcon';
    import ClockIcon from 'phosphor-svelte/lib/ClockIcon';
    import FolderIcon from 'phosphor-svelte/lib/FolderIcon';
    import ArrowCounterClockwiseIcon from 'phosphor-svelte/lib/ArrowCounterClockwiseIcon';
    import XIcon from 'phosphor-svelte/lib/XIcon';
    import { t_store } from '../../i18n/i18n';
    import { fileTypeFamily, fileTypeIcon } from '../file_list/file_type';
    import { formatBytes } from '../../utils';
    import {
        eta_seconds,
        extension_of,
        format_clock,
        format_rate,
        percent_text,
        type TrayItem,
    } from './upload_tray_model';

    interface Props {
        item: TrayItem;
        onCancel: (key: string) => void;
        onRetry: (key: string) => void;
    }

    let { item, onCancel, onRetry }: Props = $props();

    const is_folder = $derived(item.kind === 'folder');
    const family = $derived(fileTypeFamily(extension_of(item.name)));
    const TypeIcon = $derived(fileTypeIcon(family));
    const cancellable = $derived(
        (item.state === 'uploading' || item.state === 'queued') && (item.source === 'upload' || item.source === 'folder'),
    );
    // Before the first progress report there is nothing to measure, so the bar
    // shimmers rather than sitting at zero as if nothing were happening.
    const awaiting = $derived(item.state === 'uploading' && item.percent === 0 && item.speed === 0);
    const percent = $derived(percent_text(item.percent));
    const eta = $derived(eta_seconds(item));
    const show_bar = $derived(item.state === 'uploading');

    const folder_line = $derived.by(() => {
        if (!is_folder) return '';
        const key = item.files_failed > 0 ? 'transfers.folder_progress_failed' : 'transfers.folder_progress';
        return $t_store(key, { done: item.files_done, total: item.files_total, failed: item.files_failed });
    });

    const meta = $derived.by(() => {
        if (item.state === 'queued') return $t_store('transfers.queued');
        if (item.state === 'canceled') return $t_store('transfers.canceled');
        if (item.state === 'failed') return item.error || $t_store('transfers.failed_default');
        if (item.state === 'done') return item.size > 0 ? formatBytes(item.size) : '';
        if (awaiting) return $t_store('transfers.starting');
        const parts = [$t_store('transfers.percent', { percent })];
        if (item.speed > 0) parts.push($t_store('transfers.speed', { speed: format_rate(item.speed) }));
        if (eta !== null) parts.push($t_store('transfers.eta', { time: format_clock(eta) }));
        return parts.join(' · ');
    });
</script>

<div class="tray-row" data-state={item.state} data-kind={item.kind}>
    <span class="tray-row-icon" data-family={is_folder ? 'folder' : family} aria-hidden="true">
        {#if is_folder}
            <FolderIcon size={20} weight="fill" aria-hidden="true" />
        {:else}
            <TypeIcon size={20} aria-hidden="true" />
        {/if}
    </span>

    <div class="tray-row-main">
        <div class="tray-row-name" title={item.name}>{item.name || $t_store('transfers.untitled')}</div>
        {#if is_folder && item.state !== 'done' && item.state !== 'canceled'}
            <div class="tray-row-sub">{folder_line}</div>
        {/if}
        {#if show_bar || awaiting}
            <div
                class="tray-row-bar"
                class:is-awaiting={awaiting}
                role="progressbar"
                aria-label={$t_store('transfers.progress_label', { name: item.name })}
                aria-valuemin={awaiting ? undefined : 0}
                aria-valuemax={awaiting ? undefined : 100}
                aria-valuenow={awaiting ? undefined : percent}
            >
                <span class="tray-row-bar-fill" style={awaiting ? '' : `width: ${percent}%`}></span>
            </div>
        {/if}
        <div class="tray-row-meta" class:is-error={item.state === 'failed'}>{meta}</div>
    </div>

    <div class="tray-row-end">
        {#if item.state === 'done'}
            <span class="tray-row-check" aria-label={$t_store('transfers.done')}>
                <CheckIcon size={14} weight="bold" aria-hidden="true" />
            </span>
        {:else if item.state === 'queued'}
            <span class="tray-row-status" aria-hidden="true"><ClockIcon size={14} aria-hidden="true" /></span>
        {/if}

        {#if item.state === 'failed' && item.retry}
            <button class="tray-row-action" type="button" onclick={() => onRetry(item.key)}>
                <ArrowCounterClockwiseIcon size={13} aria-hidden="true" />
                <span>{$t_store('common.retry')}</span>
            </button>
        {/if}

        {#if cancellable}
            <button
                class="tray-row-cancel"
                type="button"
                aria-label={$t_store('transfers.cancel_item', { name: item.name })}
                title={$t_store('common.cancel')}
                onclick={() => onCancel(item.key)}
            >
                <XIcon size={13} aria-hidden="true" />
            </button>
        {:else if item.state !== 'failed' && item.size > 0 && item.state !== 'done'}
            <span class="tray-row-size">{formatBytes(item.size)}</span>
        {/if}
    </div>
</div>

<style>
    .tray-row {
        display: grid;
        grid-template-columns: 28px minmax(0, 1fr) auto;
        align-items: center;
        gap: var(--space-2-5, 10px);
        padding: var(--space-2, 8px) var(--space-3, 12px);
        min-height: 52px;
        color: var(--color-text-main, #e8e8ea);
        font-size: 13px;
        line-height: 1.3;
    }

    .tray-row-icon {
        box-sizing: border-box;
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 28px;
        height: 28px;
        color: color-mix(in srgb, #7F9CC4 55%, var(--color-text-muted, #8a8a90));
    }

    .tray-row-main {
        display: grid;
        gap: 3px;
        min-width: 0;
    }

    .tray-row-name,
    .tray-row-sub,
    .tray-row-meta {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        min-width: 0;
    }

    .tray-row-name { font-weight: 500; }

    .tray-row-sub,
    .tray-row-meta {
        color: var(--color-text-muted, #8a8a90);
        font-size: 12px;
        font-variant-numeric: tabular-nums;
    }

    .tray-row-meta.is-error {
        color: var(--color-danger, #de6c76);
        white-space: normal;
        display: -webkit-box;
        -webkit-line-clamp: 2;
        line-clamp: 2;
        -webkit-box-orient: vertical;
    }

    .tray-row-bar {
        position: relative;
        height: 3px;
        border-radius: 999px;
        background: rgb(127 156 196 / 0.16);
        overflow: hidden;
    }

    .tray-row-bar-fill {
        position: absolute;
        inset: 0 auto 0 0;
        border-radius: inherit;
        background: #7F9CC4;
        transition: width 180ms ease-out;
    }

    .tray-row-bar.is-awaiting .tray-row-bar-fill {
        inset: 0;
        width: 40%;
        background: linear-gradient(90deg, transparent, rgb(127 156 196 / 0.7), transparent);
        animation: tray-shimmer 1.2s ease-in-out infinite;
    }

    @keyframes tray-shimmer {
        from { transform: translateX(-100%); }
        to { transform: translateX(260%); }
    }

    @media (prefers-reduced-motion: reduce) {
        .tray-row-bar.is-awaiting .tray-row-bar-fill { animation: none; width: 100%; opacity: 0.35; }
        .tray-row-bar-fill { transition: none; }
    }

    .tray-row-end {
        display: inline-flex;
        align-items: center;
        gap: var(--space-1-5, 6px);
        color: var(--color-text-muted, #8a8a90);
    }

    .tray-row-size {
        font-size: 12px;
        font-variant-numeric: tabular-nums;
        white-space: nowrap;
    }

    .tray-row-check {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 20px;
        height: 20px;
        border-radius: 999px;
        color: #7F9CC4;
        border: 1px solid rgb(127 156 196 / 0.45);
    }

    .tray-row-status {
        display: inline-flex;
        color: var(--color-text-muted, #8a8a90);
    }

    .tray-row-action {
        display: inline-flex;
        align-items: center;
        gap: 4px;
        height: 26px;
        padding: 0 var(--space-2, 8px);
        border: 1px solid color-mix(in srgb, var(--color-danger, #de6c76) 45%, transparent);
        border-radius: 6px;
        background: transparent;
        color: var(--color-danger, #de6c76);
        font: inherit;
        font-size: 12px;
        cursor: pointer;
    }

    .tray-row-action:hover,
    .tray-row-action:focus-visible {
        background: color-mix(in srgb, var(--color-danger, #de6c76) 12%, transparent);
    }

    .tray-row-cancel {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 26px;
        height: 26px;
        border: 0;
        border-radius: 6px;
        background: transparent;
        color: var(--color-text-muted, #8a8a90);
        cursor: pointer;
    }

    .tray-row-cancel:hover,
    .tray-row-cancel:focus-visible {
        background: rgb(255 255 255 / 0.07);
        color: var(--color-text-main, #e8e8ea);
    }
</style>
