<script lang="ts">
    import ModalShell from './modal_shell.svelte';
    import { newDriveModal } from './new_drive_modal_store';
    import { t_store } from '../../i18n/i18n';

    interface Props {
        onSubmit: (title: string, requireApproval: boolean) => void | Promise<void>;
    }

    let { onSubmit }: Props = $props();
    let title = $state('');
    let requireApproval = $state(false);
    let wasOpen = false;

    const view = newDriveModal.state;

    function close(): void {
        if ($view.busy) return;
        newDriveModal.close();
    }

    function submit(): void {
        const next = title.trim();
        if (!next || $view.busy) return;
        void onSubmit(next, requireApproval);
    }

    function onInputKeydown(event: KeyboardEvent): void {
        if (event.key === 'Enter') {
            event.preventDefault();
            submit();
        }
    }

    $effect(() => {
        if ($view.open && !wasOpen) {
            title = '';
            requireApproval = false;
        }
        wasOpen = $view.open;
    });
</script>

<ModalShell
    hostId="new-drive-modal"
    open={$view.open}
    title={$t_store('modals.new_drive_title')}
    titleId="new-drive-title"
    subtitle={$t_store('modals.new_drive_subtitle')}
    initialFocus="#new-drive-name"
    restoreFocus="#drives-nav"
    onClose={close}
>
    <div class="modal-field">
        <label class="field-label" for="new-drive-name">{$t_store('modals.new_drive_name_label')}</label>
        <input
            id="new-drive-name"
            type="text"
            placeholder={$t_store('modals.new_drive_name_placeholder')}
            autocomplete="off"
            bind:value={title}
            disabled={$view.busy}
            onkeydown={onInputKeydown}
        />
    </div>
    <label class="modal-choice-row">
        <input id="new-drive-require-approval" type="checkbox" bind:checked={requireApproval} disabled={$view.busy} />
        <span>
            <strong>{$t_store('modals.new_drive_approval_title')}</strong>
            <small>{$t_store('modals.new_drive_approval_desc')}</small>
        </span>
    </label>

    {#snippet actions()}
        <button id="new-drive-cancel" class="secondary-btn" type="button" disabled={$view.busy} onclick={close}>
            {$t_store('common.cancel')}
        </button>
        <button
            id="new-drive-create"
            class="primary-btn"
            type="button"
            disabled={$view.busy || !title.trim()}
            onclick={submit}
        >
            {$t_store('modals.new_drive_create')}
        </button>
    {/snippet}
</ModalShell>
