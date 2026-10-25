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
    import { openAccountDetail } from './mobile_shell_store';
    const available = native_engine_present();

    onMount(() => {
        if (available) start_mobile_downloads();
    });
</script>

{#if available}
    <button type="button" class="account-row" data-account-detail="download" onclick={() => openAccountDetail('download')}>
        <DownloadSimpleIcon class="account-row-icon" size={20} aria-hidden="true" />
        <span class="account-row-label">{$t_store('downloader.button_label')}</span>
        <CaretRightIcon class="account-row-chevron" size={18} aria-hidden="true" />
    </button>
{/if}
