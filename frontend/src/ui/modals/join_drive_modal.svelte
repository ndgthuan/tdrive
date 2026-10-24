<script lang="ts">
    import ModalShell from './modal_shell.svelte';
    import { joinDriveModal } from './join_drive_modal_store';
    import { t_store } from '../../i18n/i18n';

    interface Props {
        onSubmit: (inviteLink: string) => void | Promise<void>;
    }

    let { onSubmit }: Props = $props();
    let link = $state('');
    let previewed = $state<'none' | 'valid' | 'invalid'>('none');
    let wasOpen = false;

    const view = joinDriveModal.state;

    // Telegram invite links: t.me/+hash, or the older t.me/joinchat/hash.
    const INVITE_PATTERN = /^(https?:\/\/)?(t\.me|telegram\.me|telegram\.dog)\/(\+|joinchat\/)[A-Za-z0-9_-]+\/?$/i;

    function close(): void {
        if ($view.busy) return;
        joinDriveModal.close();
    }

    function preview(): void {
        const next = link.trim();
        if (!next || $view.busy) return;
        previewed = INVITE_PATTERN.test(next) ? 'valid' : 'invalid';
    }

    function submit(): void {
        const next = link.trim();
        if (!next || $view.busy || previewed !== 'valid') return;
        void onSubmit(next);
    }

    async function paste(): Promise<void> {
        try {
            const text = await navigator.clipboard.readText();
            link = text.trim();
            previewed = 'none';
        } catch {
            // Clipboard access can be refused; the field still takes typed input.
        }
    }

    function onInputKeydown(event: KeyboardEvent): void {
        if (event.key === 'Enter') {
            event.preventDefault();
            if (previewed === 'valid') submit();
            else preview();
        }
    }

    $effect(() => {
        if ($view.open && !wasOpen) {
            link = '';
            previewed = 'none';
        }
        wasOpen = $view.open;
    });
</script>

<ModalShell
    hostId="join-drive-modal"
    open={$view.open}
    title={$t_store('modals.join_drive_title')}
    titleId="join-drive-title"
    subtitle={$t_store('modals.join_drive_subtitle')}
    initialFocus="#join-drive-link"
    restoreFocus="#drives-nav"
    onClose={close}
>
    <div class="modal-field">
        <label class="field-label" for="join-drive-link">{$t_store('modals.join_drive_link_label')}</label>
        <div class="join-drive-input-row">
            <input
                id="join-drive-link"
                type="text"
                placeholder="https://t.me/+..."
                autocomplete="off"
                bind:value={link}
                disabled={$view.busy}
                oninput={() => (previewed = 'none')}
                onkeydown={onInputKeydown}
            />
            <button id="join-drive-paste" class="secondary-btn" type="button" disabled={$view.busy} onclick={paste}>
                {$t_store('modals.join_drive_paste')}
            </button>
        </div>
    </div>

    {#if previewed === 'invalid'}
        <p id="join-drive-invalid" class="modal-hint modal-hint-error" role="alert">
            {$t_store('modals.join_drive_invalid')}
        </p>
    {:else if previewed === 'valid'}
        <div id="join-drive-preview" class="join-drive-preview">
            <strong>{$t_store('modals.join_drive_preview')}</strong>
            <span>{$t_store('modals.join_drive_preview_hint')}</span>
        </div>
    {/if}

    {#snippet actions()}
        <button id="join-drive-cancel" class="secondary-btn" type="button" disabled={$view.busy} onclick={close}>
            {$t_store('common.cancel')}
        </button>
        {#if previewed === 'valid'}
            <button
                id="join-drive-go"
                class="primary-btn"
                type="button"
                disabled={$view.busy || !link.trim()}
                onclick={submit}
            >
                {$t_store('modals.join_drive_go')}
            </button>
        {:else}
            <button
                id="join-drive-preview-btn"
                class="primary-btn"
                type="button"
                disabled={$view.busy || !link.trim()}
                onclick={preview}
            >
                {$t_store('modals.join_drive_preview')}
            </button>
        {/if}
    {/snippet}
</ModalShell>

<style>
    .join-drive-input-row {
        display: flex;
        gap: 8px;
        align-items: stretch;
    }
    .join-drive-input-row input {
        flex: 1;
        min-width: 0;
    }
    .join-drive-preview {
        display: flex;
        flex-direction: column;
        gap: 4px;
        padding: 12px 14px;
        border-radius: 12px;
        background: var(--color-surface-2);
        font-size: 14px;
        color: var(--color-text-muted);
    }
    .join-drive-preview strong {
        color: var(--color-text);
        font-weight: 600;
    }
</style>
