<script lang="ts">
    import ModalShell from './modal_shell.svelte';
    import { logoutModal, type LogoutMode } from './logout_modal_store';
    import { t_store } from '../../i18n/i18n';

    interface Props {
        onConfirm: (mode: LogoutMode) => void | Promise<void>;
    }

    let { onConfirm }: Props = $props();
    let mode = $state<LogoutMode>('soft');
    let wasOpen = false;

    const view = logoutModal.state;

    // Only the reset choice wipes local data, so only it wears the danger style
    // and the fuller label. A quick logout is an ordinary primary action.
    const destructive = $derived(mode === 'full');

    function close(): void {
        if ($view.busy) return;
        logoutModal.close();
    }

    function confirm(): void {
        if ($view.busy) return;
        void onConfirm(mode);
    }

    $effect(() => {
        if ($view.open && !wasOpen) {
            mode = 'soft';
        }
        wasOpen = $view.open;
    });
</script>

<ModalShell
    hostId="logout-modal"
    open={$view.open}
    title={$t_store('modals.logout_title')}
    titleId="logout-title"
    subtitle={$t_store('modals.logout_subtitle')}
    initialFocus="#logout-cancel"
    restoreFocus="#profile-trigger"
    onClose={close}
>
    <label class="modal-choice-row">
        <input type="radio" name="logout-mode" value="soft" bind:group={mode} disabled={$view.busy} />
        <span>
            <strong>{$t_store('modals.logout_soft_title')}</strong>
            <small>{$t_store('modals.logout_soft_desc')}</small>
        </span>
    </label>
    <label class="modal-choice-row">
        <input type="radio" name="logout-mode" value="full" bind:group={mode} disabled={$view.busy} />
        <span>
            <strong>{$t_store('modals.logout_full_title')}</strong>
            <small>{$t_store('modals.logout_full_desc')}</small>
        </span>
    </label>

    {#snippet actions()}
        <button id="logout-cancel" class="secondary-btn" type="button" disabled={$view.busy} onclick={close}>
            {$t_store('common.cancel')}
        </button>
        <button
            id="logout-confirm"
            class={destructive ? 'primary-btn danger-btn' : 'primary-btn'}
            type="button"
            disabled={$view.busy}
            onclick={confirm}
        >
            {destructive ? $t_store('modals.logout_confirm_reset') : $t_store('modals.logout_confirm')}
        </button>
    {/snippet}
</ModalShell>
