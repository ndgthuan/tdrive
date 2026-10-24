<script module lang="ts">
    import { readable } from 'svelte/store';

    /**
     * One clock for every row on screen, rather than a timer per row: a phone
     * left on the Transfers tab used to watch fifty stamps freeze at whatever
     * they said when the tab opened, and fifty intervals is not the way to
     * unfreeze them. `readable` starts the interval with the first subscriber
     * and clears it with the last, so a closed panel ticks nothing at all.
     *
     * 30s is the coarsest tick the labels below can hide -- the finest thing
     * they distinguish is "just now" from a count of seconds -- and the whole
     * list re-reads one number when it fires.
     */
    const RELATIVE_TICK_MS = 30_000;

    const clock = readable(Date.now(), (set) => {
        set(Date.now());
        const timer = setInterval(() => set(Date.now()), RELATIVE_TICK_MS);
        return () => clearInterval(timer);
    });
</script>

<script lang="ts">
    import { onDestroy } from 'svelte';
    import CheckIcon from 'phosphor-svelte/lib/CheckIcon';
    import CircleXIcon from 'phosphor-svelte/lib/XCircleIcon';
    import InfoIcon from 'phosphor-svelte/lib/InfoIcon';
    import TriangleAlertIcon from 'phosphor-svelte/lib/WarningIcon';
    import type { NoticeEvent } from './notif_store';
    import { t_store, type Translator } from '../../i18n/i18n';

    interface Props {
        event: NoticeEvent;
        listItem?: boolean;
    }

    let { event, listItem = false }: Props = $props();
    let copied = $state(false);
    let copiedTimer: ReturnType<typeof setTimeout> | null = null;

    const clipable = $derived(event.level === 'error' && Boolean(event.body));

    function formatRelative(ts: number, now: number, tr: Translator): string {
        if (!ts) return '';
        const diffSec = Math.floor((now - ts) / 1000);
        if (diffSec < 30) return tr('activity.age_just_now');
        if (diffSec < 60) return tr('activity.age_seconds', { count: diffSec });
        const m = Math.floor(diffSec / 60);
        if (m < 60) return tr('activity.age_minutes', { count: m });
        const h = Math.floor(m / 60);
        if (h < 24) return tr('activity.age_hours', { count: h });
        const d = Math.floor(h / 24);
        if (d < 7) return tr('activity.age_days', { count: d });
        return new Date(ts).toLocaleDateString();
    }

    async function copyBody(): Promise<void> {
        if (!clipable) return;
        try {
            await navigator.clipboard.writeText(`${event.title}\n${event.body}`);
        } catch {
            return;
        }
        copied = true;
        if (copiedTimer) clearTimeout(copiedTimer);
        copiedTimer = setTimeout(() => {
            copied = false;
            copiedTimer = null;
        }, 800);
    }
    onDestroy(() => {
        if (copiedTimer) clearTimeout(copiedTimer);
    });
</script>

<div
    class={`notif-row notif-row-event level-${event.level}${copied ? ' notif-copied' : ''}`}
    role={listItem ? 'listitem' : undefined}
>
    <span class="notif-row-icon" data-kind={event.level} aria-hidden="true">
        {#if event.level === 'success'}
            <CheckIcon size={14} aria-hidden="true" weight="bold" />
        {:else if event.level === 'error'}
            <CircleXIcon size={14} aria-hidden="true" weight="bold" />
        {:else if event.level === 'warning'}
            <TriangleAlertIcon size={14} aria-hidden="true" weight="bold" />
        {:else}
            <InfoIcon size={14} aria-hidden="true" weight="bold" />
        {/if}
    </span>
    <div class="notif-row-body">
        <div class="notif-row-title">
            {event.title}
            {#if (event.repeats ?? 1) > 1}
                <!-- Said once, with a count, rather than said again. -->
                <span class="notif-row-repeats">&times;{event.repeats}</span>
            {/if}
        </div>
        {#if event.body}
            <div class="notif-row-sub">{event.body}</div>
        {/if}
    </div>
    <div class="notif-row-meta">{formatRelative(event.ts, $clock, $t_store)}</div>
    {#if clipable}
        <button
            class="notif-row-copy"
            type="button"
            aria-label={copied ? $t_store('activity.copied_details_aria') : $t_store('activity.copy_details_aria')}
            onclick={() => void copyBody()}
        >
            {copied ? $t_store('activity.copied_details') : $t_store('activity.copy_details')}
        </button>
    {/if}
</div>
