<script lang="ts">
    import ModalShell from './modal_shell.svelte';
    import { importOptionsModal } from './import_options_modal_store';
    import { t_store, format_number } from '../../i18n/i18n';

    interface Props {
        onCancel: () => void;
        onConfirm: (choice: { encrypt: boolean; extract: boolean }) => void;
        onToggle: (encrypt: boolean, extract: boolean) => void;
    }

    let { onCancel, onConfirm, onToggle }: Props = $props();
    let extract = $state(false);
    let encrypt = $state(false);
    let wasOpen = false;

    const view = importOptionsModal.state;

    function humanBytes(n: number): string {
        if (!Number.isFinite(n) || n <= 0) return '0 B';
        const units = ['B', 'KB', 'MB', 'GB', 'TB'];
        let value = n;
        let i = 0;
        while (value >= 1024 && i < units.length - 1) {
            value /= 1024;
            i++;
        }
        const rounded = value < 10 && i > 0 ? value.toFixed(1) : String(Math.round(value));
        return `${rounded} ${units[i]}`;
    }

    const summary = $derived.by(() => {
        const plan = $view.payload?.plan;
        if (!plan) return '';
        const parts = [$t_store('modals.import_files', { count: plan.files })];
        if (plan.folders > 0) parts.push($t_store('modals.import_folders', { count: plan.folders }));
        if (plan.archives > 0) parts.push($t_store('modals.import_archives', { count: plan.archives }));
        let text = parts.join('  ·  ');
        if (plan.bytes > 0) text += `  ·  ${humanBytes(plan.bytes)}`;
        return text;
    });

    const note = $derived.by(() => {
        const plan = $view.payload?.plan;
        if (!plan) return '';
        const reportedErrors = Number(plan.errorCount);
        const errors = Number.isFinite(reportedErrors)
            ? Math.max(0, Math.floor(reportedErrors))
            : (Array.isArray(plan.errors) ? plan.errors.length : 0);
        const notes: string[] = [];
        if (plan.oversize > 0) {
            notes.push($t_store('modals.import_note_oversize', { count: plan.oversize, limit: humanBytes(plan.maxBytes) }));
        }
        if (plan.ignored > 0) {
            notes.push($t_store('modals.import_note_ignored', { count: plan.ignored }));
        }
        if (errors > 0) {
            notes.push($t_store('modals.import_note_errors', { count: errors }));
        }
        if (plan.limitExceeded) {
            notes.push($t_store('modals.import_note_limit', { max: format_number(plan.maxItems) }));
        }
        return notes.join(' ');
    });

    function toggled(): void {
        onToggle(encrypt, extract);
    }

    $effect(() => {
        if ($view.open && !wasOpen) {
            extract = false;
            encrypt = false;
        }
        wasOpen = $view.open;
    });
</script>

<ModalShell
    hostId="import-options-modal"
    open={$view.open}
    title={$t_store('modals.import_title')}
    titleId="import-options-title"
    subtitle={summary}
    initialFocus="#import-options-extract"
    restoreFocus="#file-list"
    onClose={onCancel}
>
    {#if $view.payload?.hasArchives}
        <label id="import-options-extract-row" class="modal-choice-row">
            <input id="import-options-extract" type="checkbox" bind:checked={extract} onchange={toggled} />
            <span>
                <strong>{$t_store('modals.import_extract_title')}</strong>
                <small>{$t_store('modals.import_extract_desc')}</small>
            </span>
        </label>
    {/if}
    {#if $view.payload?.personal}
        <label id="import-options-encrypt-row" class="modal-choice-row">
            <input id="import-options-encrypt" type="checkbox" bind:checked={encrypt} onchange={toggled} />
            <span>
                <strong>{$t_store('modals.upload_encrypt_title')}</strong>
                <small>{$t_store('modals.upload_encrypt_desc')}</small>
            </span>
        </label>
    {/if}
    {#if note}
        <p id="import-options-note" class="import-options-note">{note}</p>
    {/if}

    {#snippet actions()}
        <button id="import-options-cancel" class="secondary-btn" type="button" onclick={onCancel}>{$t_store('common.cancel')}</button>
        <button
            id="import-options-confirm"
            class="primary-btn"
            type="button"
            disabled={$view.payload?.plan.limitExceeded || $view.payload?.replanning}
            aria-busy={$view.payload?.replanning}
            onclick={() => onConfirm({ encrypt, extract })}
        >
            {$view.payload?.replanning ? $t_store('modals.import_checking') : $t_store('modals.import_title')}
        </button>
    {/snippet}
</ModalShell>
