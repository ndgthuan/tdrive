<script lang="ts">
    import DownloadSimpleIcon from 'phosphor-svelte/lib/DownloadSimpleIcon';
    import { t_store } from '../../i18n/i18n';
    import DownloaderDialog from './downloader_dialog.svelte';
    import { downloader_open, open_downloader } from './downloader_state';
</script>

<button
    id="downloader-open-btn"
    type="button"
    class="downloader-toggle"
    class:is-on={$downloader_open}
    aria-label={$t_store('downloader.button_label')}
    aria-haspopup="dialog"
    aria-expanded={$downloader_open ? 'true' : 'false'}
    aria-controls="downloader-modal"
    title={$t_store('downloader.button_title')}
    onclick={open_downloader}
>
    <DownloadSimpleIcon size={16} aria-hidden="true" />
</button>

<DownloaderDialog />

<style>
    .downloader-toggle {
        width: 32px;
        height: 32px;
        display: inline-grid;
        place-items: center;
        flex: none;
        padding: 0;
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
        background: transparent;
        color: var(--text-muted);
        cursor: pointer;
    }

    .downloader-toggle:hover {
        background: var(--color-surface-2);
        color: var(--text-main);
    }

    .downloader-toggle.is-on {
        background: color-mix(in srgb, var(--color-accent) 18%, transparent);
        border-color: transparent;
        color: var(--color-accent);
    }

    .downloader-toggle:focus-visible {
        outline: none;
        box-shadow: var(--focus-ring, 0 0 0 2px var(--color-accent));
    }
</style>
