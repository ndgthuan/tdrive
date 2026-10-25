<script lang="ts">
    import { onMount } from 'svelte';
    import ChevronRightIcon from 'phosphor-svelte/lib/CaretRightIcon';
    import { t_store } from '../../i18n/i18n';
    import { author, notice_text, upstream_name } from '../settings/about_notice';
    import { getAppVersion } from '../../api/updates';
    import { copy_and_report } from '../../modules/diagnostics';
    import tdriveLogo from '../../assets/images/tdrive_logo.png';

    // The About page of the phone's Account tab, as the board draws it: the app
    // mark and version, the attribution line, the one-sentence promise, and the
    // rows that open the notices and the diagnostics.
    let version = $state('');
    let licensesOpen = $state(false);

    onMount(() => {
        void getAppVersion()
            .then((info) => { version = info.version; })
            .catch(() => { version = ''; });
    });
</script>

<section class="about-panel" aria-label={$t_store('mobile.account_about')}>
    <div class="about-hero">
        <img class="about-mark" src={tdriveLogo} alt="" width="72" height="72" />
        <div class="about-name">TDrive</div>
        <div class="about-version">
            {version ? $t_store('mobile.about_version', { version }) : $t_store('mobile.about_version_unknown')}
        </div>
    </div>
    <p class="about-made-by">{$t_store('mobile.about_made_by', { author, upstream: upstream_name })}</p>
    <p class="about-promise">{$t_store('mobile.about_files_note')}</p>

    <div class="account-card">
        <button type="button" class="account-row" aria-expanded={licensesOpen} onclick={() => { licensesOpen = !licensesOpen; }}>
            <span class="account-row-label">{$t_store('mobile.about_licenses')}</span>
            <ChevronRightIcon class="account-row-chevron" size={18} aria-hidden="true" />
        </button>
        {#if licensesOpen}
            <pre class="about-licenses">{notice_text}</pre>
        {/if}
        <button type="button" class="account-row" onclick={() => { void copy_and_report(); }}>
            <span class="account-row-label">{$t_store('mobile.copy_diagnostics')}</span>
            <ChevronRightIcon class="account-row-chevron" size={18} aria-hidden="true" />
        </button>
    </div>
</section>

<style>
    .about-panel { display: flex; flex-direction: column; gap: var(--space-3); padding: var(--space-4) 0 var(--space-6); }
    .about-hero { display: flex; flex-direction: column; align-items: center; gap: var(--space-2); padding: var(--space-4) 0; }
    .about-mark {
        display: grid; place-items: center; width: 72px; height: 72px; border-radius: 18px;
        object-fit: contain;
    }
    .about-name { font-size: 1.5rem; font-weight: 700; color: var(--color-text); }
    .about-version { font-size: 0.875rem; color: var(--color-text-muted); }
    .about-made-by, .about-promise { margin: 0 var(--space-2); font-size: 0.875rem; color: var(--color-text-muted); text-align: center; }
    .about-licenses {
        max-height: 40vh; margin: 0; padding: var(--space-3); overflow: auto; white-space: pre-wrap;
        font: inherit; font-size: 0.75rem; color: var(--color-text-muted); background: var(--color-surface-1);
    }
</style>
