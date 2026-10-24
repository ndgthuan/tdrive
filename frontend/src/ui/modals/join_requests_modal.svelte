<script lang="ts">
    import ModalShell from './modal_shell.svelte';
    import {
        joinRequestsList,
        joinRequestsModal,
        type JoinRequestRow,
    } from './join_requests_modal_store';
    import { t_store } from '../../i18n/i18n';

    interface Props {
        onAction: (userId: number, approved: boolean) => void | Promise<void>;
        onRetry: () => void;
    }

    let { onAction, onRetry }: Props = $props();

    const view = joinRequestsModal.state;
    const list = joinRequestsList;
    const subtitle = $derived($t_store('modals.join_requests_subtitle', { title: $view.payload?.title ?? $t_store('modals.join_requests_this_drive') }));

    function close(): void {
        joinRequestsModal.close();
    }

    function detailFor(row: JoinRequestRow): string {
        const pieces: string[] = [];
        if (row.username) pieces.push(row.username);
        if (row.requestedAt) pieces.push(new Date(row.requestedAt * 1000).toLocaleString());
        return pieces.join(' · ');
    }
</script>

<ModalShell
    hostId="join-requests-modal"
    open={$view.open}
    title={$t_store('modals.join_requests_title')}
    titleId="join-requests-title"
    {subtitle}
    initialFocus="#join-requests-close"
    restoreFocus="#drives-nav"
    onClose={close}
>
    <div id="join-requests-list" class="join-requests-list">
        {#if $list.status === 'loading'}
            <div class="modal-empty">{$t_store('modals.join_requests_loading')}</div>
        {:else if $list.status === 'error'}
            <div class="modal-error join-requests-error" role="alert">
                <span>{$t_store('modals.join_requests_error', { message: String($list.message ?? '') })}</span>
                <button type="button" class="link-button" onclick={onRetry}>{$t_store('modals.join_requests_retry')}</button>
            </div>
        {:else if $list.rows.length === 0}
            <div class="modal-empty">{$t_store('modals.join_requests_empty')}</div>
        {:else}
            {#each $list.rows as row (row.userId)}
                <div class="join-request-row">
                    <div class="join-request-meta">
                        <div class="join-request-name">{row.displayName}</div>
                        <div class="join-request-detail">{detailFor(row)}</div>
                    </div>
                    <div class="join-request-actions">
                        <button
                            type="button"
                            class="secondary-btn compact-btn"
                            disabled={$list.actingUserId !== 0}
                            onclick={() => void onAction(row.userId, true)}
                        >
                            {$t_store('modals.join_requests_approve')}
                        </button>
                        <button
                            type="button"
                            class="secondary-btn compact-btn danger-text"
                            disabled={$list.actingUserId !== 0}
                            onclick={() => void onAction(row.userId, false)}
                        >
                            {$t_store('modals.join_requests_reject')}
                        </button>
                    </div>
                </div>
            {/each}
        {/if}
    </div>

    {#snippet actions()}
        <button id="join-requests-close" class="secondary-btn" type="button" onclick={close}>{$t_store('common.close')}</button>
    {/snippet}
</ModalShell>

<style>
    /* The humanised reason sits above its own Try again, so the retry is not
       buried at the end of a sentence. */
    .join-requests-error {
        display: flex;
        flex-direction: column;
        align-items: flex-start;
        gap: var(--space-2);
    }
</style>
