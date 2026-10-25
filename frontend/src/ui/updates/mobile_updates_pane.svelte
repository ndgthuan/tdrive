<script lang="ts">
    import { onMount } from 'svelte';
    import Button from '../button.svelte';
    import { appVersionInfo, updateState } from './update_store';
    import { checkForUpdates, openReleasePage } from '../../modules/updates';
    import { t_store } from '../../i18n/i18n';
    const busy = $derived($updateState.phase === 'checking');
    onMount(() => { void checkForUpdates(); });
</script>

<section class="mobile-updates" aria-label={$t_store('settings_updates.title')}>
    <p>{$t_store('mobile.about_version', { version: $appVersionInfo?.version || '—' })}</p>
    <div role="status" aria-live="polite">
        {#if busy}<p>{$t_store('misc.checking')}</p>
        {:else if $updateState.latest}<p>{$t_store('misc.available_title', { version: $updateState.latest.version })}</p>
        {:else if $updateState.phase === 'up_to_date'}<p>{$t_store('misc.up_to_date')}</p>{/if}
        {#if $updateState.error}<p role="alert">{$updateState.error}</p>{/if}
    </div>
    <p class="hint">{$t_store($appVersionInfo?.os === 'ios' ? 'settings_updates.ios_install_hint' : 'settings_updates.android_install_hint')}</p>
    <Button disabled={busy} onclick={() => void checkForUpdates({ explicit: true })}>{$t_store('settings_updates.check_button')}</Button>
    <Button variant="primary" onclick={openReleasePage}>{$t_store('settings_updates.open_downloads')}</Button>
</section>

<style>
    .mobile-updates { display: grid; gap: var(--space-4); font-size: var(--mobile-type-body); }
    p { margin: 0; overflow-wrap: anywhere; }
    .hint { color: var(--color-text-muted); line-height: 1.5; }
</style>
