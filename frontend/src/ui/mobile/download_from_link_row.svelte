<!-- Copyright (c) 2026 Nguyễn Dương Gia Thuận -->

<script lang="ts">
    // Entry point for the phone's link downloader. Mount it where the Files or
    // Account screen lists its actions; it renders nothing without a native engine.
    import CaretRightIcon from 'phosphor-svelte/lib/CaretRightIcon';
    import DownloadSimpleIcon from 'phosphor-svelte/lib/DownloadSimpleIcon';
    import { onMount } from 'svelte';
    import { t_store } from '../../i18n/i18n';
    import { native_engine_present } from '../../modules/media_download/native_engine';
    import { start_mobile_downloads } from '../../modules/media_download/mobile_jobs';
    import DownloadFromLinkSheet from './download_from_link_sheet.svelte';

    let sheet_open = $state(false);
    const available = native_engine_present();

    onMount(() => {
        if (available) start_mobile_downloads();
    });
</script>

{#if available}
    <button type="button" class="link-row" aria-haspopup="dialog" aria-expanded={sheet_open} onclick={() => (sheet_open = true)}>
        <span class="icon" aria-hidden="true"><DownloadSimpleIcon size={20} /></span>
        <span class="text">
            <span class="title">{$t_store('downloader.button_label')}</span>
            <span class="sub">{$t_store('downloader.subtitle')}</span>
        </span>
        <CaretRightIcon size={16} aria-hidden="true" />
    </button>
    <DownloadFromLinkSheet open={sheet_open} onclose={() => (sheet_open = false)} />
{/if}

<style>
    .link-row {
        display: flex;
        align-items: center;
        gap: 12px;
        width: 100%;
        min-height: 54px;
        padding: 8px 14px;
        border: 0;
        background: #1e2227;
        border-radius: 16px;
        color: #e8eaed;
        font: inherit;
        text-align: left;
        cursor: pointer;
    }

    .icon {
        width: 36px;
        height: 36px;
        display: grid;
        place-items: center;
        border-radius: 10px;
        background: #1e2a3d;
        color: #7f9cc4;
        flex: none;
    }

    .text {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
    }

    .title {
        font-size: 16px;
        font-weight: 500;
    }

    .sub {
        font-size: 13px;
        color: #a0a7b1;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
</style>
