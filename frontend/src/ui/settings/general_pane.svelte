<!-- Copyright (c) 2026 Nguyễn Dương Gia Thuận -->

<script lang="ts">
    import { openExternalUrl } from '../../api';
    import { humanizeBackendError } from '../../modules/errors';
    import { author, notice_text, repository_url, upstream_name } from './about_notice';
    import { formatBytes } from '../../utils';
    import { t_store } from '../../i18n/i18n';
    import {
        cancelUpdateDownload,
        checkForUpdates,
        downloadUpdate,
        getRestartRisks,
        installUpdate,
    } from '../../modules/updates';
    import { formatPlatform, isVersionSkipped, progressPercent } from '../updates/update_model';
    import {
        appVersionInfo,
        setAutoDownload,
        updatePrefs,
        updateState,
    } from '../updates/update_store';

    let confirming = $state(false);
    let risks = $state<string[]>([]);
    let preparing_restart = $state(false);
    let show_licenses = $state(false);

    const version = $derived($appVersionInfo?.version ?? '');
    const platform = $derived(formatPlatform($appVersionInfo));
    const latest = $derived($updateState.latest);
    const phase = $derived($updateState.phase);
    const percent = $derived(progressPercent($updateState.downloadedBytes, $updateState.totalBytes));
    const skipped = $derived(latest ? isVersionSkipped(latest.version, $updatePrefs.skippedVersion) : false);
    const managed = $derived(Boolean($appVersionInfo?.storeManaged));
    const dev_build = $derived(phase === 'disabled');
    const checking = $derived(phase === 'checking');

    async function begin_restart(): Promise<void> {
        if (preparing_restart) return;
        preparing_restart = true;
        try {
            risks = await getRestartRisks();
        } finally {
            preparing_restart = false;
        }
        if (risks.length === 0) {
            void installUpdate();
            return;
        }
        confirming = true;
    }

    function confirm_restart(): void {
        confirming = false;
        void installUpdate();
    }
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

    {#if managed}
        <p class="settings-note">{$t_store('misc.managed_by_store')}</p>
    {:else if dev_build}
        <p class="settings-note">{$t_store('misc.dev_build')}</p>
    {:else}
        <div class="update-status" role="status" aria-live="polite">
            {#if checking}
                <p class="update-line">{$t_store('misc.checking')}</p>
            {:else if phase === 'up_to_date'}
                <p class="update-line">{$t_store('misc.up_to_date')}</p>
            {:else if phase === 'downloading'}
                <div class="update-row">
                    <span class="update-line">{$t_store('settings_updates.downloading', { version: latest?.version ?? '' })} · {percent}%</span>
                    <button class="settings-btn ghost" type="button" onclick={cancelUpdateDownload}>{$t_store('misc.cancel')}</button>
                </div>
            {:else if phase === 'ready'}
                <p class="update-line">{$t_store('misc.ready_title')}</p>
                <p class="settings-note">{$t_store('settings_updates.ready_detail', { version: latest?.version ?? '' })}</p>
                {#if confirming}
                    <ul class="update-risks">
                        {#each risks as risk (risk)}<li>{risk}</li>{/each}
                    </ul>
                    <div class="update-row">
                        <button class="settings-btn primary" type="button" onclick={confirm_restart}>{$t_store('settings_updates.restart_confirm')}</button>
                        <button class="settings-btn" type="button" onclick={() => (confirming = false)}>{$t_store('misc.cancel')}</button>
                    </div>
                {:else}
                    <div>
                        <button class="settings-btn primary" type="button" onclick={begin_restart} disabled={preparing_restart}>
                            {$t_store('settings_updates.restart')}
                        </button>
                    </div>
                {/if}
            {:else if phase === 'available' && $updateState.installable && !skipped}
                <p class="update-line">{$t_store('misc.available_title', { version: latest?.version ?? '' })}</p>
                <p class="settings-note">{$t_store('settings_updates.available_detail')}</p>
                <div>
                    <button class="settings-btn primary" type="button" onclick={() => void downloadUpdate()}>
                        {$t_store('settings_updates.download')}{#if latest?.assetSize} · {formatBytes(latest.assetSize)}{/if}
                    </button>
                </div>
            {:else if phase === 'available'}
                <p class="settings-note">{$updateState.installHint || $t_store('misc.newer_available')}</p>
            {:else}
                <p class="settings-note">{$t_store('misc.check_prompt')}</p>
            {/if}

            {#if $updateState.error && !checking}
                <p class="settings-note error" role="alert">{humanizeBackendError($updateState.error)}</p>
            {/if}
        </div>

        <div class="update-footer">
            <span id="settings-auto-update-label">{$t_store('settings_updates.auto_update_label')}</span>
            <button
                class="settings-switch"
                type="button"
                role="switch"
                aria-checked={$updatePrefs.autoDownload}
                aria-labelledby="settings-auto-update-label"
                onclick={() => setAutoDownload(!$updatePrefs.autoDownload)}
            ></button>
        </div>

        <div class="update-footer">
            <button
                class="settings-btn ghost"
                type="button"
                onclick={() => void checkForUpdates({ explicit: true })}
                disabled={checking}
            >
                {$t_store('settings_updates.check_button')}
            </button>
        </div>
    {/if}

    <div class="about-block">
        <h4 class="about-title">{$t_store('settings.about_title')}</h4>
        <dl class="settings-facts">
            <div class="settings-fact">
                <dt>{$t_store('settings.about_name')}</dt>
                <dd>TDrive</dd>
            </div>
            <div class="settings-fact">
                <dt>{$t_store('settings.app_version')}</dt>
                <dd>{version || '—'}</dd>
            </div>
        </dl>
        <p class="settings-note">{$t_store('settings.about_made_by', { author })}</p>
        <p class="settings-note">{$t_store('settings.about_based_on', { upstream: upstream_name })}</p>
        <div class="update-row">
            <button class="settings-btn ghost" type="button" onclick={() => openExternalUrl(repository_url)}>
                {$t_store('settings.about_github')}
            </button>
            <button
                class="settings-btn ghost"
                type="button"
                aria-expanded={show_licenses}
                aria-controls="about-licenses"
                onclick={() => (show_licenses = !show_licenses)}
            >
                {$t_store('settings.about_licenses')}
            </button>
        </div>
        {#if show_licenses}
            <section id="about-licenses" class="about-licenses" aria-labelledby="about-licenses-title">
                <h5 id="about-licenses-title" class="about-title">{$t_store('settings.about_licenses_title')}</h5>
                <pre class="about-notice">{notice_text}</pre>
                <div>
                    <button class="settings-btn" type="button" onclick={() => (show_licenses = false)}>{$t_store('settings.close')}</button>
                </div>
            </section>
        {/if}
    </div>
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

    .update-status { display: flex; flex-direction: column; gap: var(--space-2); }

    .update-line {
        margin: 0;
        color: var(--color-text);
        font-size: var(--type-base);
        font-weight: var(--weight-medium);
    }

    .update-row { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); }

    .update-risks {
        margin: 0;
        padding-left: 18px;
        color: var(--color-text-muted);
        font-size: var(--type-sm);
        line-height: var(--leading-snug);
    }

    .update-footer {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
    }

    .about-block {
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
        padding-top: var(--space-4);
        border-top: 1px solid var(--color-border-soft);
    }

    .about-title {
        margin: 0;
        color: var(--color-text);
        font-size: var(--type-base);
        font-weight: var(--weight-semibold);
    }

    .about-licenses {
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
    }

    .about-notice {
        max-height: 320px;
        margin: 0;
        padding: var(--space-3) var(--space-4);
        overflow: auto;
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-md);
        background: var(--color-surface-1);
        color: var(--color-text-muted);
        font-family: inherit;
        font-size: var(--type-sm);
        line-height: var(--leading-snug);
        white-space: pre-wrap;
        overflow-wrap: anywhere;
    }
</style>
