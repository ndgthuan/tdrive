<script lang="ts">
    import { humanizeBackendError } from '../../modules/errors';
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
    let preparingRestart = $state(false);

    const version = $derived($updateState.currentVersion || $appVersionInfo?.version || '');
    const platform = $derived(formatPlatform($appVersionInfo));
    const latest = $derived($updateState.latest);
    const phase = $derived($updateState.phase);
    const percent = $derived(progressPercent($updateState.downloadedBytes, $updateState.totalBytes));
    const skipped = $derived(latest ? isVersionSkipped(latest.version, $updatePrefs.skippedVersion) : false);
    const managed = $derived(Boolean($appVersionInfo?.storeManaged));
    const devBuild = $derived(phase === 'disabled');
    const checking = $derived(phase === 'checking');
    const lastChecked = $derived(formatChecked($updateState.checkedAt));

    function formatChecked(ms: number): string {
        if (!ms) return $t_store('misc.not_checked');
        const date = new Date(ms);
        if (Number.isNaN(date.getTime())) return $t_store('misc.not_checked');
        return $t_store('misc.checked_at', { time: date.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' }) });
    }

    async function beginRestart(): Promise<void> {
        if (preparingRestart) return;
        preparingRestart = true;
        try {
            risks = await getRestartRisks();
        } finally {
            preparingRestart = false;
        }
        if (risks.length === 0) {
            void installUpdate();
            return;
        }
        confirming = true;
    }

    function confirmRestart(): void {
        confirming = false;
        void installUpdate();
    }
</script>

<section class="settings-pane" aria-labelledby="settings-updates-title">
    <header class="settings-pane-header">
        <h3 id="settings-updates-title" class="settings-pane-title">{$t_store('settings_updates.title')}</h3>
        <p class="settings-pane-sub">{$t_store('settings_updates.subtitle')}</p>
    </header>

    <dl class="settings-facts">
        <div class="settings-fact">
            <dt>{$t_store('settings_updates.current_version')}</dt>
            <dd>{version || $t_store('misc.version_unknown')}</dd>
        </div>
        {#if platform}
            <div class="settings-fact">
                <dt>{$t_store('settings.platform')}</dt>
                <dd>{platform}</dd>
            </div>
        {/if}
    </dl>

    <div class="update-status" role="status" aria-live="polite">
        {#if managed}
            <p class="settings-note">{$t_store('misc.managed_by_store')}</p>
        {:else if devBuild}
            <p class="settings-note">{$t_store('misc.dev_build')}</p>
        {:else if checking}
            <p class="update-line">{$t_store('misc.checking')}</p>
        {:else if phase === 'up_to_date'}
            <p class="update-line">{$t_store('misc.up_to_date')}</p>
        {:else if phase === 'downloading'}
            <div class="update-row">
                <span class="update-line">{$t_store('settings_updates.downloading', { version: latest?.version ?? '' })}</span>
                <button class="settings-btn ghost" type="button" onclick={cancelUpdateDownload}>{$t_store('misc.cancel')}</button>
            </div>
            <div class="update-progress" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent}>
                <div class="update-progress-fill" style={`width:${percent}%`}></div>
            </div>
            <p class="settings-note">
                {formatBytes($updateState.downloadedBytes)}{#if $updateState.totalBytes > 0} / {formatBytes($updateState.totalBytes)}{/if} · {percent}%
            </p>
        {:else if phase === 'ready'}
            <p class="update-line">{$t_store('misc.ready_title')}</p>
            <p class="settings-note">{$t_store('settings_updates.ready_detail', { version: latest?.version ?? '' })}</p>
            {#if confirming}
                <ul class="update-risks">
                    {#each risks as risk (risk)}<li>{risk}</li>{/each}
                </ul>
                <div class="update-row">
                    <button class="settings-btn primary" type="button" onclick={confirmRestart}>{$t_store('settings_updates.restart_confirm')}</button>
                    <button class="settings-btn" type="button" onclick={() => (confirming = false)}>{$t_store('misc.cancel')}</button>
                </div>
            {:else}
                <div>
                    <button class="settings-btn primary" type="button" onclick={beginRestart} disabled={preparingRestart}>
                        {$t_store('settings_updates.restart')}
                    </button>
                </div>
            {/if}
        {:else if phase === 'available'}
            {#if $updateState.installable && !skipped}
                <p class="update-line">{$t_store('misc.available_title', { version: latest?.version ?? '' })}</p>
                <p class="settings-note">{$t_store('settings_updates.available_detail')}</p>
                <div>
                    <button class="settings-btn primary" type="button" onclick={() => void downloadUpdate()}>
                        {$t_store('settings_updates.download')}{#if latest?.assetSize} · {formatBytes(latest.assetSize)}{/if}
                    </button>
                </div>
            {:else}
                <p class="settings-note">{$updateState.installHint || $t_store('misc.newer_available')}</p>
            {/if}
        {:else}
            <p class="settings-note">{$t_store('misc.check_prompt')}</p>
        {/if}

        {#if !managed && $updateState.error && !checking}
            <p class="settings-note error" role="alert">{humanizeBackendError($updateState.error)}</p>
        {/if}
    </div>

    {#if !managed && !devBuild}
        <div class="settings-switch-row">
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
            <span class="settings-note">{lastChecked}</span>
            <button
                class="settings-btn primary"
                type="button"
                onclick={() => void checkForUpdates({ explicit: true })}
                disabled={checking}
            >
                {$t_store('settings_updates.check_button')}
            </button>
        </div>
    {/if}
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
        min-height: 40px;
        padding: 0 var(--space-4);
    }

    .settings-fact + .settings-fact { border-top: 1px solid var(--color-border-soft); }
    .settings-fact dt { color: var(--color-text-muted); font-size: var(--type-base); }
    .settings-fact dd { margin: 0; color: var(--color-text); font-size: var(--type-base); font-weight: var(--weight-medium); }

    .update-status { display: flex; flex-direction: column; gap: var(--space-2); min-height: 0; }

    .update-line {
        margin: 0;
        color: var(--color-text);
        font-size: var(--type-base);
        font-weight: var(--weight-medium);
    }

    .update-row { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); }

    .update-progress {
        height: 6px;
        border-radius: var(--radius-pill);
        background: var(--overlay-white-2);
        overflow: hidden;
    }

    .update-progress-fill {
        height: 100%;
        border-radius: var(--radius-pill);
        background: var(--color-accent);
        transition: width var(--motion-med) var(--ease-standard);
    }

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
        margin-top: auto;
    }

    @media (prefers-reduced-motion: reduce) {
        .update-progress-fill { transition: none; }
    }
</style>
