<script lang="ts">
    import { onDestroy } from 'svelte';
    import ModalShell from './modal_shell.svelte';
    import { shareDriveModal } from './share_drive_modal_store';
    import { t_store } from '../../i18n/i18n';

    let linkEl = $state<HTMLElement | null>(null);
    let copyStatus = $state<'idle' | 'copied' | 'failed'>('idle');
    let copiedTimer: ReturnType<typeof setTimeout> | null = null;
    let wasOpen = false;

    const view = shareDriveModal.state;
    const link = $derived($view.payload?.link ?? '');
    const subtitle = $derived(
        $view.payload?.approvalRequired
            ? $t_store('modals.share_subtitle_approval')
            : $t_store('modals.share_subtitle_open'),
    );

    function close(): void {
        shareDriveModal.close();
    }

    function selectLink(): void {
        if (!linkEl) return;
        const selection = window.getSelection();
        if (!selection) return;
        const range = document.createRange();
        range.selectNodeContents(linkEl);
        selection.removeAllRanges();
        selection.addRange(range);
    }

    async function copy(): Promise<void> {
        let succeeded = false;
        try {
            await navigator.clipboard.writeText(link);
            succeeded = true;
        } catch {
            // Clipboard API can be unavailable in the webview; fall back to the
            // selection-based copy without focusing an editable control and
            // summoning the software keyboard.
            selectLink();
            succeeded = typeof document.execCommand === 'function' && document.execCommand('copy');
        }
        copyStatus = succeeded ? 'copied' : 'failed';
        if (copiedTimer) clearTimeout(copiedTimer);
        copiedTimer = setTimeout(() => {
            copyStatus = 'idle';
            copiedTimer = null;
        }, 1200);
    }

    $effect(() => {
        if ($view.open && !wasOpen) {
            copyStatus = 'idle';
        }
        wasOpen = $view.open;
    });

    onDestroy(() => {
        if (copiedTimer) clearTimeout(copiedTimer);
    });
</script>

<ModalShell
    hostId="share-drive-modal"
    open={$view.open}
    title={$t_store('modals.share_title')}
    titleId="share-drive-title"
    {subtitle}
    initialFocus="#share-drive-copy"
    restoreFocus="#drives-nav"
    onClose={close}
>
    <div
        id="share-drive-link"
        class="share-drive-link"
        aria-label={$t_store('modals.share_link_aria')}
        bind:this={linkEl}
    >{link}</div>

    {#snippet actions()}
        <button id="share-drive-close" class="secondary-btn" type="button" onclick={close}>{$t_store('common.close')}</button>
        <button id="share-drive-copy" class="primary-btn" type="button" onclick={() => void copy()}>
            {copyStatus === 'copied' ? $t_store('modals.share_copied') : copyStatus === 'failed' ? $t_store('modals.share_copy_failed') : $t_store('modals.share_copy_link')}
        </button>
    {/snippet}
</ModalShell>
