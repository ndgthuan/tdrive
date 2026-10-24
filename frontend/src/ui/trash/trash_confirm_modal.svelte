<script lang="ts">
    /**
     * The one question asked before anything leaves the trash for good. It sits
     * over the trash dialog; modal-a11y treats nested dialogs as one stack, so
     * this owns the keyboard, Escape and Android BACK while it is up.
     */
    import FileIcon from 'phosphor-svelte/lib/FileIcon';
    import FolderIcon from 'phosphor-svelte/lib/FolderIcon';
    import ModalShell from '../modals/modal_shell.svelte';
    import { t_store, type Translator } from '../../i18n/i18n';
    import { trashConfirmModal, type TrashConfirmation } from './trash_confirm_store';

    interface Props {
        onConfirm: () => void | Promise<void>;
    }

    let { onConfirm }: Props = $props();

    const view = trashConfirmModal.state;
    const target = $derived($view.payload);

    function close(): void {
        if ($view.busy) return;
        trashConfirmModal.close();
    }

    function confirm(): void {
        if ($view.busy) return;
        // The controller closes this dialog before the async work starts.
        void onConfirm();
    }

    // The translator is passed in from the template's $t_store, so the text
    // follows the language rather than being read once.
    function title(confirmation: TrashConfirmation, tr: Translator): string {
        return confirmation.kind === 'empty' ? tr('trash.confirm_empty_title') : tr('trash.confirm_delete_title');
    }

    function subtitle(confirmation: TrashConfirmation, tr: Translator): string {
        if (confirmation.kind === 'empty') {
            return tr('trash.confirm_empty_body', { what: confirmation.summary || tr('trash.everything_here') });
        }
        return confirmation.isFolder
            ? tr('trash.confirm_folder_body')
            : tr('trash.confirm_file_body');
    }
</script>

<ModalShell
    hostId="trash-confirm-modal"
    open={$view.open}
    title={target ? title(target, $t_store) : ''}
    titleId="trash-confirm-title"
    subtitle={target ? subtitle(target, $t_store) : ''}
    initialFocus="#trash-confirm-cancel"
    restoreFocus="#trash-empty-btn"
    onClose={close}
>
    {#if target?.kind === 'purge'}
        <!-- What is about to go, as the row it is: a bare name in a frame read
             as a text field waiting to be typed into. -->
        <div class="trash-confirm-target">
            <span class="trash-confirm-icon" aria-hidden="true">
                {#if target.isFolder}
                    <FolderIcon size={16} />
                {:else}
                    <FileIcon size={16} />
                {/if}
            </span>
            <span class="trash-confirm-name" title={target.name}>{target.name}</span>
            <span class="trash-confirm-kind">{target.isFolder ? $t_store('trash.kind_folder') : $t_store('trash.kind_file')}</span>
        </div>
    {/if}

    {#snippet actions()}
        <button id="trash-confirm-cancel" class="secondary-btn" type="button" disabled={$view.busy} onclick={close}>
            {$t_store('trash.cancel')}
        </button>
        <button id="trash-confirm-accept" class="primary-btn danger-btn" type="button" disabled={$view.busy} onclick={confirm}>
            {target?.kind === 'empty' ? $t_store('trash.empty_trash_action') : $t_store('trash.delete_permanently_action')}
        </button>
    {/snippet}
</ModalShell>

<style>
    /* The target reads as one row of the list it came from, and stands clear of
       the buttons: it used to sit right on top of them. */
    .trash-confirm-target {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-width: 0;
        margin: 2px 0 var(--space-4);
        padding: var(--space-3);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        background: var(--surface-control);
    }

    .trash-confirm-icon {
        flex: 0 0 auto;
        display: inline-flex;
        color: var(--text-muted);
    }

    .trash-confirm-name {
        flex: 1 1 auto;
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        font-size: var(--font-size-sm);
        font-weight: var(--weight-semibold);
    }

    .trash-confirm-kind {
        flex: 0 0 auto;
        color: var(--text-muted);
        font-size: var(--font-size-xs);
    }

    :global(html.mobile) .trash-confirm-name {
        font-size: var(--mobile-type-body);
    }
</style>
