<script lang="ts">
    import ModalShell from './modal_shell.svelte';
    import { closeFolderModalView, folderModalState } from './folder_modal_store';
    import { t_store } from '../../i18n/i18n';

    interface Props {
        onSubmit: (name: string) => void | Promise<void>;
    }

    let { onSubmit }: Props = $props();
    let name = $state('');
    let wasOpen = false;

    function canClose(): boolean {
        return !$folderModalState.inFlight;
    }

    function close(): void {
        if (!canClose()) return;
        closeFolderModalView();
    }

    function submit(): void {
        if ($folderModalState.inFlight) return;
        const next = name.trim();
        if (!next) return;
        void onSubmit(next);
    }

    function onInputKeydown(event: KeyboardEvent): void {
        if (event.key === 'Enter') {
            event.preventDefault();
            submit();
        }
        if (event.key === 'Escape') {
            event.preventDefault();
            close();
        }
    }

    $effect(() => {
        if ($folderModalState.open && !wasOpen) {
            name = '';
        }
        wasOpen = $folderModalState.open;
    });
</script>

<ModalShell
    hostId="folder-modal"
    open={$folderModalState.open}
    title={$t_store('modals.folder_title')}
    titleId="folder-modal-title"
    subtitle={$t_store('modals.folder_subtitle')}
    initialFocus="#new-folder-name"
    restoreFocus="#file-list"
    onClose={close}
>
    <div class="modal-field">
        <label class="field-label" for="new-folder-name">{$t_store('modals.folder_name_label')}</label>
        <input
            id="new-folder-name"
            type="text"
            placeholder={$t_store('modals.folder_name_placeholder')}
            autocomplete="off"
            bind:value={name}
            disabled={$folderModalState.inFlight}
            onkeydown={onInputKeydown}
        />
    </div>

    {#snippet actions()}
        <button id="folder-cancel" class="secondary-btn" type="button" disabled={$folderModalState.inFlight} onclick={close}>
            {$t_store('common.cancel')}
        </button>
        <button
            id="folder-create"
            class="primary-btn"
            type="button"
            disabled={$folderModalState.inFlight || !name.trim()}
            onclick={submit}
        >
            {$t_store('modals.folder_create')}
        </button>
    {/snippet}
</ModalShell>
