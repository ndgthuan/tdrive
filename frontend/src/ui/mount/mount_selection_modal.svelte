<script lang="ts">
    import ModalShell from '../modals/modal_shell.svelte';
    import { mountSelection } from './mount_selection_store';
    import { t_store } from '../../i18n/i18n';

    const view = mountSelection.state;
</script>

<ModalShell
    hostId="mount-selection-modal"
    open={$view.open}
    title={$t_store('mount.selection_title')}
    titleId="mount-selection-title"
    cardClass="mount-selection-card"
    subtitle={$t_store('mount.selection_subtitle')}
    initialFocus=".mount-drive-choice input"
    restoreFocus="#profile-trigger"
    onClose={mountSelection.close}
>
    <fieldset class="mount-drive-list">
        <legend class="visually-hidden">{$t_store('mount.drives_legend')}</legend>
        {#each $view.drives as drive (drive.id)}
            <label class="mount-drive-choice">
                <input
                    type="checkbox"
                    value={drive.id}
                    checked={$view.selectedIds.includes(drive.id)}
                    onchange={() => mountSelection.toggle(drive.id)}
                />
                <span class="mount-drive-copy">
                    <span class="mount-drive-title">{drive.title}</span>
                    <span class="mount-drive-mode">
                        {drive.kind === 'personal' ? $t_store('mount.kind_personal') : $t_store('mount.kind_shared')}
                    </span>
                </span>
            </label>
        {/each}
    </fieldset>

    {#snippet actions()}
        <button
            id="mount-selection-cancel"
            class="secondary-btn"
            type="button"
            onclick={mountSelection.close}
        >
            {$t_store('mount.cancel')}
        </button>
        <button
            id="mount-selection-confirm"
            class="primary-btn"
            type="button"
            disabled={$view.selectedIds.length === 0}
            onclick={mountSelection.confirm}
        >
            {$t_store('mount.mount_action')}
        </button>
    {/snippet}
</ModalShell>

<style>
    /* The mount sheet as the design draws it: a surface-2 card, a 20px title,
       and checkbox cards 40px tall with a 16px box. The phone sheet keeps its
       own layout. */
    :global(.mount-selection-card:not(.modal-sheet)) {
        max-width: 470px;
        padding: var(--space-6);
        border-color: var(--color-border-soft);
        border-radius: var(--radius-lg);
        background: var(--color-surface-2);
    }

    :global(.mount-selection-card:not(.modal-sheet) > .modal-title) {
        margin-bottom: 0;
        font-size: var(--type-xl);
        letter-spacing: -0.01em;
    }

    :global(.mount-selection-card:not(.modal-sheet) > .modal-subtitle) {
        margin: var(--space-1) 0 var(--space-4);
        font-size: var(--type-sm);
        line-height: 1.5;
    }

    .mount-drive-list {
        display: grid;
        max-height: min(360px, 48vh);
        gap: var(--space-2);
        padding: 0;
        margin: 0 0 var(--space-4);
        overflow-y: auto;
        border: 0;
    }

    .mount-drive-choice {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-width: 0;
        min-height: 40px;
        padding: var(--space-1) var(--space-3);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-sm);
        color: var(--text-main);
        background: transparent;
        cursor: pointer;
    }

    .mount-drive-choice:hover { background: var(--color-surface-3); }
    .mount-drive-choice:has(input:checked) { background: var(--overlay-accent-1); }

    .mount-drive-choice:focus-within {
        border-color: var(--accent);
        box-shadow: var(--focus-ring);
    }

    /* A 16px box drawn here; the native checkbox stays the control. */
    .mount-drive-choice input {
        appearance: none;
        -webkit-appearance: none;
        display: grid;
        width: 16px;
        height: 16px;
        flex: 0 0 auto;
        place-content: center;
        margin: 0;
        border: 1px solid var(--color-border);
        border-radius: 4px;
        background: transparent;
        cursor: pointer;
    }

    .mount-drive-choice input::after {
        width: 4px;
        height: 8px;
        border: solid var(--color-on-accent);
        border-width: 0 2px 2px 0;
        content: '';
        transform: translateY(-1px) rotate(45deg) scale(0);
        transition: transform var(--motion-fast) var(--ease-standard);
    }

    .mount-drive-choice input:checked {
        border-color: var(--accent);
        background: var(--accent);
    }

    .mount-drive-choice input:checked::after { transform: translateY(-1px) rotate(45deg) scale(1); }
    .mount-drive-choice input:focus-visible { outline: none; box-shadow: var(--focus-ring); }

    .mount-drive-copy {
        display: flex;
        min-width: 0;
        flex-direction: column;
        gap: 1px;
        line-height: 1.2;
    }

    .mount-drive-title {
        overflow: hidden;
        color: var(--color-text);
        font-size: var(--type-base);
        font-weight: var(--weight-semibold);
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .mount-drive-mode {
        color: var(--text-muted);
        font-size: var(--type-xs);
    }

    .visually-hidden {
        position: absolute;
        width: 1px;
        height: 1px;
        padding: 0;
        margin: -1px;
        overflow: hidden;
        clip: rect(0, 0, 0, 0);
        white-space: nowrap;
        border: 0;
    }

    :global(html:not(.mobile) #mount-selection-cancel),
    :global(html:not(.mobile) #mount-selection-confirm) {
        min-height: var(--control-md);
        height: var(--control-md);
        padding: 0 var(--space-3);
        font-size: var(--type-sm);
    }

    :global(#mount-selection-confirm:disabled) {
        cursor: not-allowed;
        opacity: 0.55;
        transform: none;
    }
</style>
