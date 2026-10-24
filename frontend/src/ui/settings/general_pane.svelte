<script lang="ts">
    import { t_store } from '../../i18n/i18n';
    import { formatPlatform } from '../updates/update_model';
    import { appVersionInfo } from '../updates/update_store';

    const version = $derived($appVersionInfo?.version ?? '');
    const platform = $derived(formatPlatform($appVersionInfo));
</script>

<section class="settings-pane" aria-labelledby="settings-general-title">
    <header class="settings-pane-header">
        <h3 id="settings-general-title" class="settings-pane-title">{$t_store('settings.general_title')}</h3>
        <p class="settings-pane-sub">{$t_store('settings.general_subtitle')}</p>
    </header>

    <dl class="settings-facts">
        <div class="settings-fact">
            <dt>{$t_store('settings.app_version')}</dt>
            <dd>{version || '—'}</dd>
        </div>
        <div class="settings-fact">
            <dt>{$t_store('settings.platform')}</dt>
            <dd>{platform || '—'}</dd>
        </div>
    </dl>
</section>

<style>
    .settings-facts {
        display: flex;
        flex-direction: column;
        margin: 0;
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-md);
        background: var(--color-surface-1);
    }

    .settings-fact {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
        min-height: 44px;
        padding: 0 var(--space-4);
    }

    .settings-fact + .settings-fact { border-top: 1px solid var(--color-border-soft); }

    .settings-fact dt {
        color: var(--color-text-muted);
        font-size: var(--type-base);
        white-space: nowrap;
    }

    .settings-fact dd {
        margin: 0;
        min-width: 0;
        color: var(--color-text);
        font-size: var(--type-base);
        font-weight: var(--weight-medium);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
</style>
