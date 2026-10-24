<script lang="ts">
    import CircleXIcon from 'phosphor-svelte/lib/XCircleIcon';
    import XIcon from 'phosphor-svelte/lib/XIcon';
    import type { FailedUploadsPayload } from '../modals/failed_uploads_store';
    import { t_store, type Translator } from '../../i18n/i18n';
    interface Props {
        reports: FailedUploadsPayload[];
        // The bell panel and the phone Transfers tab each own their section
        // chrome; the rows match whichever list they sit beside.
        variant?: 'panel' | 'mobile';
        onReview: (report: FailedUploadsPayload) => void;
        onDismiss: (report: FailedUploadsPayload) => void;
    }
    let { reports, variant = 'panel', onReview, onDismiss }: Props = $props();

    function summary(report: FailedUploadsPayload, tr: Translator): string {
        const retryable = report.failures.filter((failure) => failure.retryable).length;
        return retryable ? tr('activity.failed_summary', { drive: report.driveTitle, count: retryable }) : report.driveTitle;
    }
</script>

{#if reports.length}
    <section aria-label={$t_store('activity.upload_failures_aria')} class="failed-upload-reports" data-variant={variant}>
        {#if variant === 'mobile'}
            <div class="transfers-section-head"><h2 class="mobile-section-label">{$t_store('activity.needs_attention')}</h2></div>
        {:else}
            <h3 class="notif-section-label">{$t_store('activity.needs_attention')}</h3>
        {/if}
        <div class={variant === 'mobile' ? 'transfers-group' : 'notif-section'} role="list">
            {#each reports as report (report)}
                <div class="notif-row failed-upload-report" role="listitem">
                    <span class="notif-row-icon" data-kind="error" aria-hidden="true">
                        <CircleXIcon size={14} aria-hidden="true" weight="bold" />
                    </span>
                    <div class="notif-row-body">
                        <div class="notif-row-title">
                            {$t_store('activity.failed_count', { count: report.failures.length })}
                        </div>
                        <div class="notif-row-sub">{summary(report, $t_store)}</div>
                    </div>
                    <div class="failed-upload-actions">
                        <button class="failed-upload-review" type="button" aria-label={$t_store('activity.review_aria')} onclick={() => onReview(report)}>{$t_store('activity.review')}</button>
                        <button class="notif-row-cancel" type="button" aria-label={$t_store('activity.dismiss_failed_aria', { drive: report.driveTitle })} onclick={() => onDismiss(report)}>
                            <XIcon size={14} aria-hidden="true" weight="bold" />
                        </button>
                    </div>
                </div>
            {/each}
        </div>
    </section>
{/if}

<style>
    .failed-upload-reports[data-variant='panel'] { margin-bottom: 14px; }
    .failed-upload-report { grid-template-columns: 22px minmax(0, 1fr) auto; }
    .failed-upload-actions { display: flex; align-items: center; gap: 4px; }
    .failed-upload-review {
        min-height: 26px;
        padding: 3px 9px;
        border: 1px solid var(--overlay-accent-2);
        border-radius: var(--radius-sm);
        background: var(--overlay-accent-1);
        color: var(--accent);
        font: inherit;
        font-size: 0.76rem;
        font-weight: var(--weight-semibold);
        cursor: pointer;
        transition: background-color var(--motion-med) var(--ease-standard);
    }
    @media (hover: hover) {
        .failed-upload-review:hover { background: var(--overlay-accent-2); }
    }
    .failed-upload-review:focus-visible,
    .failed-upload-actions .notif-row-cancel:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
    .failed-upload-actions .notif-row-cancel { display: inline-flex; align-items: center; justify-content: center; cursor: pointer; }
    /* Larger touch targets on phones. */
    [data-variant='mobile'] .failed-upload-review { min-height: 36px; padding: 0 14px; font-size: 0.8125rem; }
    [data-variant='mobile'] .failed-upload-actions .notif-row-cancel { width: 40px; height: 40px; }
</style>
