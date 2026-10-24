<script lang="ts">
    import DotsIcon from 'phosphor-svelte/lib/DotsThreeIcon';
    import PlusIcon from 'phosphor-svelte/lib/PlusIcon';
    import { onMount } from 'svelte';
    import { delete_model, list_models, Provider, set_default_model, type ModelView } from '../../api/ai';
    import { humanizeBackendError } from '../../modules/errors';
    import { t, t_store } from '../../i18n/i18n';
    import { provider_brand_label, provider_mark } from './ai_model_draft';
    import AiModelForm from './ai_model_form.svelte';

    type view = { kind: 'list' } | { kind: 'form'; model: ModelView | null };

    let models = $state<ModelView[]>([]);
    let loading = $state(true);
    let error = $state('');
    let view = $state<view>({ kind: 'list' });
    let openMenuId = $state('');
    let confirmId = $state('');
    let busyId = $state('');

    onMount(() => {
        void refresh();
    });

    async function refresh(): Promise<void> {
        loading = true;
        try {
            models = await list_models();
            error = '';
        } catch (cause) {
            error = humanizeBackendError(cause) || t('settings_ai.load_failed');
        } finally {
            loading = false;
        }
    }

    // Reads the store, so the template re-renders when the language changes.
    function providerName(provider: Provider): string {
        return provider_brand_label(provider) || $t_store('settings_ai.provider_custom');
    }

    function visionLabel(model: ModelView): string {
        return model.vision_enabled ? $t_store('settings_ai.vision_yes') : $t_store('settings_ai.vision_no');
    }

    function openForm(model: ModelView | null): void {
        openMenuId = '';
        confirmId = '';
        error = '';
        view = { kind: 'form', model };
    }

    async function formDone(): Promise<void> {
        view = { kind: 'list' };
        await refresh();
    }

    function toggleMenu(id: string): void {
        confirmId = '';
        openMenuId = openMenuId === id ? '' : id;
    }

    async function makeDefault(model: ModelView): Promise<void> {
        openMenuId = '';
        busyId = model.id;
        try {
            await set_default_model(model.id);
            await refresh();
        } catch (cause) {
            error = humanizeBackendError(cause) || t('settings_ai.default_failed');
        } finally {
            busyId = '';
        }
    }

    async function removeModel(model: ModelView): Promise<void> {
        busyId = model.id;
        try {
            await delete_model(model.id);
            confirmId = '';
            await refresh();
        } catch (cause) {
            error = humanizeBackendError(cause) || t('settings_ai.delete_failed');
        } finally {
            busyId = '';
        }
    }
</script>

{#if view.kind === 'form'}
    <AiModelForm
        model={view.model}
        firstModel={models.length === 0}
        onDone={formDone}
        onCancel={() => (view = { kind: 'list' })}
    />
{:else}
    <section class="settings-pane" aria-labelledby="settings-ai-title">
        <header class="ai-header">
            <div class="ai-header-copy">
                <h3 id="settings-ai-title" class="settings-pane-title">{$t_store('settings_ai.title')}</h3>
                <p class="settings-pane-sub">{$t_store('settings_ai.subtitle')}</p>
            </div>
            <button type="button" class="settings-btn primary ai-add" onclick={() => openForm(null)}>
                <PlusIcon size={16} aria-hidden="true" />
                {$t_store('settings_ai.add_model')}
            </button>
        </header>

        {#if error}
            <p class="settings-note error" role="alert">{error}</p>
        {/if}

        {#if loading}
            <p class="settings-note" role="status">{$t_store('settings_ai.loading')}</p>
        {:else if models.length === 0}
            <p class="settings-note ai-empty">{$t_store('settings_ai.empty')}</p>
        {:else}
            <ul class="ai-list" aria-label={$t_store('settings_ai.title')}>
                {#each models as model (model.id)}
                    <li class="ai-card-wrap">
                        <div class="ai-card">
                            <div class="ai-mark" aria-hidden="true">{provider_mark(model.provider)}</div>
                            <div class="ai-card-main">
                                <div class="ai-card-title">
                                    <span class="ai-card-name">{model.display_name}</span>
                                    {#if model.is_default}
                                        <span class="ai-badge">{$t_store('settings_ai.default_badge')}</span>
                                    {/if}
                                </div>
                                <div class="ai-card-sub">{providerName(model.provider)} · {model.model}</div>
                                <div class="ai-card-meta">
                                    <span>{visionLabel(model)}</span>
                                    <span aria-hidden="true">·</span>
                                    <span>
                                        {model.has_api_key ? $t_store('settings_ai.key_present') : $t_store('settings_ai.key_missing')}
                                    </span>
                                </div>
                            </div>
                            <button type="button" class="settings-btn ghost" onclick={() => openForm(model)}>
                                {$t_store('settings_ai.edit')}
                            </button>
                            <div class="ai-more">
                                <button
                                    type="button"
                                    class="settings-icon-btn"
                                    aria-label={$t_store('settings_ai.more')}
                                    aria-haspopup="menu"
                                    aria-expanded={openMenuId === model.id}
                                    disabled={busyId === model.id}
                                    onclick={() => toggleMenu(model.id)}
                                >
                                    <DotsIcon size={16} aria-hidden="true" />
                                </button>
                                {#if openMenuId === model.id}
                                    <div class="ai-menu" role="menu">
                                        {#if !model.is_default}
                                            <button type="button" role="menuitem" class="ai-menu-item" onclick={() => void makeDefault(model)}>
                                                {$t_store('settings_ai.set_default')}
                                            </button>
                                        {/if}
                                        <button
                                            type="button"
                                            role="menuitem"
                                            class="ai-menu-item danger"
                                            onclick={() => { openMenuId = ''; confirmId = model.id; }}
                                        >
                                            {$t_store('settings_ai.delete')}
                                        </button>
                                    </div>
                                {/if}
                            </div>
                        </div>
                        {#if confirmId === model.id}
                            <div class="ai-confirm" role="group" aria-label={$t_store('settings_ai.delete_confirm_action')}>
                                <span>{$t_store('settings_ai.delete_confirm', { name: model.display_name })}</span>
                                <div class="ai-confirm-actions">
                                    <button type="button" class="settings-btn" onclick={() => (confirmId = '')}>{$t_store('settings_ai.cancel')}</button>
                                    <button type="button" class="settings-btn danger" disabled={busyId === model.id} onclick={() => void removeModel(model)}>
                                        {$t_store('settings_ai.delete_confirm_action')}
                                    </button>
                                </div>
                            </div>
                        {/if}
                    </li>
                {/each}
            </ul>
        {/if}

        <p class="settings-note ai-footnote">{$t_store('settings_ai.default_note')}</p>
    </section>
{/if}

<style>
    .ai-header {
        display: flex;
        align-items: flex-start;
        gap: var(--space-3);
        padding-right: 40px;
    }

    .ai-header-copy { flex: 1; min-width: 0; }
    .ai-add { flex: none; }

    .ai-list {
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
        margin: 0;
        padding: 0;
        list-style: none;
    }

    .ai-card-wrap { display: flex; flex-direction: column; gap: 6px; }

    .ai-card {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-width: 0;
        padding: 12px 14px;
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-md);
        background: var(--color-surface-1);
    }

    .ai-mark {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        flex: none;
        width: var(--control-md);
        height: var(--control-md);
        color: var(--color-text-muted);
        background: var(--overlay-white-1);
        border-radius: var(--radius-sm);
        font-size: var(--type-sm);
        font-weight: var(--weight-semibold);
    }

    .ai-card-main { display: flex; flex: 1; flex-direction: column; gap: 2px; min-width: 0; }

    .ai-card-title {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        min-width: 0;
        color: var(--color-text);
        font-size: var(--type-base);
        font-weight: var(--weight-semibold);
        white-space: nowrap;
    }

    .ai-card-name { overflow: hidden; text-overflow: ellipsis; }

    .ai-badge {
        display: inline-flex;
        align-items: center;
        flex: none;
        height: 20px;
        padding: 0 6px;
        color: var(--color-text);
        background: var(--overlay-accent-2);
        border-radius: var(--radius-sm);
        font-size: var(--type-2xs);
        font-weight: var(--weight-semibold);
        white-space: nowrap;
    }

    .ai-card-sub,
    .ai-card-meta {
        display: flex;
        gap: var(--space-1-5);
        min-width: 0;
        color: var(--color-text-muted);
        font-size: var(--type-xs);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }


    .ai-more { position: relative; flex: none; }

    .ai-menu {
        position: absolute;
        top: calc(100% + 4px);
        right: 0;
        z-index: var(--z-popover);
        display: flex;
        flex-direction: column;
        min-width: 160px;
        padding: var(--space-1);
        background: var(--color-surface-2);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-md);
        box-shadow: var(--shadow-lg);
    }

    .ai-menu-item {
        height: var(--control-md);
        padding: 0 var(--space-2);
        color: var(--color-text);
        background: transparent;
        border: 0;
        border-radius: var(--radius-sm);
        font-size: var(--type-base);
        text-align: left;
        white-space: nowrap;
        cursor: pointer;
    }

    .ai-menu-item:hover { background: var(--overlay-white-1); }
    .ai-menu-item.danger { color: var(--color-danger); }

    .ai-confirm {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
        margin-left: 14px;
        padding: 10px 12px;
        color: var(--color-text);
        background: var(--overlay-white-1);
        border-radius: var(--radius-md);
        font-size: var(--type-sm);
    }

    .ai-confirm-actions { display: flex; gap: var(--space-2); flex: none; }

    .ai-empty { padding: var(--space-4) 0; }
    .ai-footnote { margin-top: auto; }
</style>
