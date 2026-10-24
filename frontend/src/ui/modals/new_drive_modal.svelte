<script lang="ts">
    import ModalShell from './modal_shell.svelte';
    import { newDriveModal } from './new_drive_modal_store';
    import { t_store } from '../../i18n/i18n';

    export type NewDriveRole = 'editor' | 'viewer';

    interface Props {
        onSubmit: (title: string, requireApproval: boolean, defaultRole: NewDriveRole) => void | Promise<void>;
    }

    let { onSubmit }: Props = $props();
    let title = $state('');
    let requireApproval = $state(false);
    // Telegram only sets editor or viewer as a drive's default role here, so
    // the picker offers those two and nothing the backend would refuse.
    let defaultRole = $state<NewDriveRole>('viewer');
    let wasOpen = false;

    const view = newDriveModal.state;
    const ROLES: NewDriveRole[] = ['editor', 'viewer'];

    function close(): void {
        if ($view.busy) return;
        newDriveModal.close();
    }

    function submit(): void {
        const next = title.trim();
        if (!next || $view.busy) return;
        void onSubmit(next, requireApproval, defaultRole);
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
            defaultRole = 'viewer';
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
    <p class="modal-hint">{$t_store('modals.new_drive_owner_note')}</p>
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
    <div class="modal-field" role="group" aria-labelledby="new-drive-role-label">
        <span id="new-drive-role-label" class="field-label">{$t_store('modals.new_drive_role_label')}</span>
        <div class="new-drive-role-chips">
            {#each ROLES as role (role)}
                <button
                    id="new-drive-role-{role}"
                    class="new-drive-role-chip"
                    class:is-selected={defaultRole === role}
                    type="button"
                    aria-pressed={defaultRole === role}
                    disabled={$view.busy}
                    onclick={() => (defaultRole = role)}
                >
                    {role === 'editor' ? $t_store('modals.new_drive_role_editor') : $t_store('modals.new_drive_role_viewer')}
                </button>
            {/each}
        </div>
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

<style>
    .new-drive-role-chips {
        display: flex;
        gap: 8px;
        flex-wrap: wrap;
    }
    .new-drive-role-chip {
        height: 36px;
        padding: 0 16px;
        border-radius: 18px;
        background: var(--color-surface-2);
        color: var(--color-text-muted);
        font: inherit;
        font-size: 14px;
    }
    .new-drive-role-chip.is-selected {
        background: var(--color-accent-soft, var(--color-surface-3));
        color: var(--color-accent);
        font-weight: 600;
    }
</style>
