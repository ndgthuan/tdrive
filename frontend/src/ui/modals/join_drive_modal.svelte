<script lang="ts">
    import ModalShell from './modal_shell.svelte';
    import { joinDriveModal } from './join_drive_modal_store';
    import { t_store } from '../../i18n/i18n';

    interface Props {
        onSubmit: (inviteLink: string) => void | Promise<void>;
    }

    let { onSubmit }: Props = $props();
    let link = $state('');
    let wasOpen = false;

    const view = joinDriveModal.state;

    function close(): void {
        if ($view.busy) return;
        joinDriveModal.close();
    }

    function submit(): void {
        const next = link.trim();
        if (!next || $view.busy) return;
        void onSubmit(next);
    }

    function onInputKeydown(event: KeyboardEvent): void {
        if (event.key === 'Enter') {
            event.preventDefault();
            submit();
        }
    }

    $effect(() => {
        if ($view.open && !wasOpen) {
            link = '';
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
        <input
            id="join-drive-link"
            type="text"
            placeholder="https://t.me/+..."
            autocomplete="off"
            bind:value={link}
            disabled={$view.busy}
            onkeydown={onInputKeydown}
        />
    </div>

    {#snippet actions()}
        <button id="join-drive-cancel" class="secondary-btn" type="button" disabled={$view.busy} onclick={close}>
            {$t_store('common.cancel')}
        </button>
        <button
            id="join-drive-go"
            class="primary-btn"
            type="button"
            disabled={$view.busy || !link.trim()}
            onclick={submit}
        >
            {$t_store('modals.join_drive_go')}
        </button>
    {/snippet}
</ModalShell>
