<script lang="ts">
    import { t_store } from '../../i18n/i18n';
    import AppearancePanel from '../theme/appearance_panel.svelte';
</script>

<section class="settings-pane" aria-labelledby="settings-appearance-title">
    <header class="settings-pane-header">
        <h3 id="settings-appearance-title" class="settings-pane-title">{$t_store('settings_appearance.title')}</h3>
        <p class="settings-pane-sub">{$t_store('settings_appearance.subtitle')}</p>
    </header>

    <AppearancePanel embedded />
</section>
