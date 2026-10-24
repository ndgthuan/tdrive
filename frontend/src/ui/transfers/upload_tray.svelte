<script lang="ts">
    import { onMount } from 'svelte';
    import CaretDownIcon from 'phosphor-svelte/lib/CaretDownIcon';
    import CaretUpIcon from 'phosphor-svelte/lib/CaretUpIcon';
    import XIcon from 'phosphor-svelte/lib/XIcon';
    import { t_store } from '../../i18n/i18n';
    import { panel_open } from '../ai/ai_store';
    import UploadTrayRow from './upload_tray_row.svelte';
    import {
        activate_upload_tray,
        cancel_tray_item,
        dismiss_finished,
        retry_tray_item,
        set_tray_hovered,
        toggle_tray_collapsed,
        tray_collapsed,
        upload_tray,
    } from './upload_tray_store';
    import { percent_text } from './upload_tray_model';

    const phase = $derived($upload_tray.summary.phase);
    const visible = $derived(phase !== 'empty');
    const collapsed = $derived($tray_collapsed);
    // Rows in the order a reader scans them: what is moving, what needs a retry, then what finished.
    // A selection can run to thousands of files. The title carries the true count;
    // the list renders a bounded slice so each progress event stays cheap.
    const MAX_RENDERED_ROWS = 200;
    const rows = $derived([...$upload_tray.rows.active, ...$upload_tray.rows.failed, ...$upload_tray.rows.finished].slice(0, MAX_RENDERED_ROWS));

    const title = $derived.by(() => {
        const count = $upload_tray.summary.count;
        switch (phase) {
            case 'uploading':
                return $t_store('transfers.title_uploading', { count });
            case 'failed':
                return $t_store('transfers.title_failed', { count });
            case 'canceled':
                return $t_store('transfers.title_canceled');
            default:
                return $t_store('transfers.title_complete');
        }
    });

    // The collapsed card still says how far along the work is.
    const overall = $derived.by(() => {
        const moving = $upload_tray.items.filter((item) => item.state === 'uploading' || item.state === 'queued');
        if (moving.length === 0) return 0;
        const sum = moving.reduce((total, item) => total + item.percent, 0);
        return percent_text(sum / moving.length);
    });

    // The tray listens to the runtime events while it is mounted, which is the whole session.
    onMount(activate_upload_tray);

    const finished = $derived($upload_tray.summary.finished);
    const panel_id = 'upload-tray-rows';
</script>

{#if visible}
    <section
        class="upload-tray"
        class:is-collapsed={collapsed}
        class:is-beside-ai={$panel_open}
        class:is-failed={phase === 'failed'}
        data-phase={phase}
        aria-label={$t_store('transfers.tray_label')}
        onmouseenter={() => set_tray_hovered(true)}
        onmouseleave={() => set_tray_hovered(false)}
        onfocusin={() => set_tray_hovered(true)}
        onfocusout={() => set_tray_hovered(false)}
    >
        <header class="upload-tray-head">
            <div class="upload-tray-title-block">
                <span class="upload-tray-title" aria-live="polite">{title}</span>
                {#if collapsed && phase === 'uploading'}
                    <span class="upload-tray-subtitle">{$t_store('transfers.percent', { percent: overall })}</span>
                {/if}
            </div>
            <button
                class="upload-tray-icon-button"
                type="button"
                aria-expanded={collapsed ? 'false' : 'true'}
                aria-controls={panel_id}
                aria-label={collapsed ? $t_store('transfers.expand') : $t_store('transfers.collapse')}
                title={collapsed ? $t_store('transfers.expand') : $t_store('transfers.collapse')}
                onclick={toggle_tray_collapsed}
            >
                {#if collapsed}
                    <CaretUpIcon size={16} aria-hidden="true" />
                {:else}
                    <CaretDownIcon size={16} aria-hidden="true" />
                {/if}
            </button>
            <button
                class="upload-tray-icon-button"
                type="button"
                disabled={!finished}
                aria-label={$t_store('transfers.dismiss')}
                title={finished ? $t_store('transfers.dismiss') : $t_store('transfers.dismiss_disabled')}
                onclick={dismiss_finished}
            >
                <XIcon size={16} aria-hidden="true" />
            </button>
        </header>

        {#if !collapsed}
            <ul id={panel_id} class="upload-tray-rows" aria-label={$t_store('transfers.rows_label')}>
                {#each rows as item (item.key)}
                    <li>
                        <UploadTrayRow {item} onCancel={cancel_tray_item} onRetry={retry_tray_item} />
                    </li>
                {/each}
            </ul>
        {/if}
    </section>
{/if}

<style>
    .upload-tray {
        position: fixed;
        right: 16px;
        bottom: 16px;
        z-index: var(--z-transfer-tray, 350);
        width: min(380px, calc(100vw - 32px));
        max-height: min(60vh, 520px);
        display: flex;
        flex-direction: column;
        overflow: hidden;
        background: var(--color-surface-1, #000);
        border: 1px solid var(--color-border-soft, #2a2a2e);
        border-radius: 12px;
        box-shadow: 0 16px 40px rgb(0 0 0 / 0.45);
        color: var(--color-text-main, #e8e8ea);
        transition: right 200ms ease-out;
    }

    /* The AI panel holds the right 380px while it is open, so the tray docks to its left. */
    .upload-tray.is-beside-ai {
        right: 396px;
    }

    .upload-tray.is-collapsed {
        max-height: none;
    }

    .upload-tray-head {
        display: flex;
        align-items: center;
        gap: var(--space-1-5, 6px);
        min-height: 44px;
        padding: 0 var(--space-1, 4px) 0 var(--space-3, 12px);
        border-bottom: 1px solid transparent;
    }

    .upload-tray:not(.is-collapsed) .upload-tray-head {
        border-bottom-color: var(--color-border-soft, #2a2a2e);
    }

    .upload-tray-title-block {
        flex: 1 1 auto;
        min-width: 0;
        display: flex;
        align-items: baseline;
        gap: var(--space-2, 8px);
    }

    .upload-tray-title {
        font-size: 14px;
        font-weight: 600;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    .upload-tray[data-phase='failed'] .upload-tray-title {
        color: var(--color-danger, #de6c76);
    }

    .upload-tray-subtitle {
        color: var(--color-text-muted, #8a8a90);
        font-size: 12px;
        font-variant-numeric: tabular-nums;
        white-space: nowrap;
    }

    .upload-tray-icon-button {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 30px;
        height: 30px;
        border: 0;
        border-radius: 8px;
        background: transparent;
        color: var(--color-text-muted, #8a8a90);
        cursor: pointer;
    }

    .upload-tray-icon-button:hover:not(:disabled),
    .upload-tray-icon-button:focus-visible {
        background: rgb(255 255 255 / 0.07);
        color: var(--color-text-main, #e8e8ea);
    }

    .upload-tray-icon-button:disabled {
        opacity: 0.35;
        cursor: default;
    }

    .upload-tray-rows {
        list-style: none;
        margin: 0;
        padding: var(--space-1, 4px) 0;
        overflow-y: auto;
        min-height: 0;
    }

    .upload-tray-rows li + li {
        border-top: 1px solid rgb(255 255 255 / 0.04);
    }

    @media (max-width: 640px) {
        .upload-tray,
        .upload-tray.is-beside-ai {
            left: 12px;
            right: 12px;
            width: auto;
        }
    }

    @media (prefers-reduced-motion: reduce) {
        .upload-tray { transition: none; }
    }
</style>
