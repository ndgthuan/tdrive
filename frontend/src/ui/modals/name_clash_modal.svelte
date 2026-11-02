<script lang="ts">
    import ModalShell from './modal_shell.svelte';
    import { nameClashModal, type NameClashChoice, type NameClashItem } from './name_clash_modal_store';
    import { SvelteMap } from 'svelte/reactivity';
    import { t_store } from '../../i18n/i18n';
    import { formatBytes } from '../../utils';

    interface Props {
        onCancel: () => void;
        onConfirm: (choices: Map<number, NameClashChoice>) => void;
    }

    let { onCancel, onConfirm }: Props = $props();
    let choices = $state<Record<number, NameClashChoice>>({});
    let wasOpen = false;

    const view = nameClashModal.state;
    const items = $derived($view.payload?.items ?? []);
    const body = $derived($t_store('modals.import_clash_body', { count: items.length }));

    // A copy needs a free name, so keep_both and replace are not offered without one.
    function canChoose(item: NameClashItem, choice: NameClashChoice): boolean {
        return choice === 'skip' || item.keptName !== '';
    }

    function applyToAll(event: Event): void {
        const select = event.currentTarget as HTMLSelectElement;
        const choice = select.value as NameClashChoice | '';
        // The select is a one-shot action, so it goes back to its prompt text.
        select.value = '';
        if (choice === '') return;
        const next = { ...choices };
        for (const item of items) {
            next[item.index] = canChoose(item, choice) ? choice : 'skip';
        }
        choices = next;
    }

    function confirm(): void {
        const result = new SvelteMap<number, NameClashChoice>();
        for (const item of items) {
            result.set(item.index, choices[item.index] ?? 'skip');
        }
        onConfirm(result);
    }

    $effect(() => {
        if ($view.open && !wasOpen) {
            // Every clash starts on skip, so a dismissed prompt never sends a file.
            const next: Record<number, NameClashChoice> = {};
            for (const item of $view.payload?.items ?? []) {
                next[item.index] = 'skip';
            }
            choices = next;
        }
        wasOpen = $view.open;
    });
</script>

<ModalShell
    hostId="name-clash-modal"
    open={$view.open}
    title={$t_store('modals.import_clash_title')}
    titleId="name-clash-title"
    subtitle={body}
    initialFocus="#name-clash-apply-all, .name-clash-list select"
    restoreFocus="#file-list"
    onClose={onCancel}
>
    {#if items.length > 1}
        <div class="name-clash-apply">
            <select
                id="name-clash-apply-all"
                class="select-field"
                aria-label={$t_store('modals.import_clash_apply_all')}
                onchange={applyToAll}
            >
                <option value="">{$t_store('modals.import_clash_apply_all')}</option>
                <option value="replace">{$t_store('modals.import_clash_replace')}</option>
                <option value="keep_both">{$t_store('modals.import_clash_keep_both')}</option>
                <option value="skip">{$t_store('modals.import_clash_skip')}</option>
            </select>
        </div>
    {/if}
    <ul class="name-clash-list">
        {#each items as item (item.index)}
            <li class="name-clash-row">
                <div class="name-clash-info">
                    <strong class="name-clash-name">{item.name}</strong>
                    <small>{$t_store('modals.import_clash_sizes', { drive: formatBytes(item.existingSize), device: formatBytes(item.incomingSize) })}</small>
                </div>
                <select class="select-field" aria-label={item.name} bind:value={choices[item.index]}>
                    <option value="replace" disabled={item.keptName === ''}>{$t_store('modals.import_clash_replace')}</option>
                    <option value="keep_both" disabled={item.keptName === ''}>{$t_store('modals.import_clash_keep_both_as', { name: item.keptName })}</option>
                    <option value="skip">{$t_store('modals.import_clash_skip')}</option>
                </select>
            </li>
        {/each}
    </ul>

    {#snippet actions()}
        <button id="name-clash-cancel" class="secondary-btn" type="button" onclick={onCancel}>{$t_store('common.cancel')}</button>
        <button id="name-clash-confirm" class="primary-btn" type="button" onclick={confirm}>{$t_store('modals.import_clash_continue')}</button>
    {/snippet}
</ModalShell>
