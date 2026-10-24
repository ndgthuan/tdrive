<script lang="ts">
    import ChevronLeftIcon from 'phosphor-svelte/lib/CaretLeftIcon';
    import { breadcrumbPath, type BreadcrumbDrag } from './breadcrumb_store';
    import { t_store } from '../../i18n/i18n';

    interface Props {
        // index -1 targets the root ("My Drive"), otherwise a folderPath index.
        onNavigate: (index: number) => void;
        onBack: () => void;
        drag: BreadcrumbDrag;
    }

    let { onNavigate, onBack, drag }: Props = $props();

    interface CrumbItem {
        id: string;
        name: string;
        index: number;
    }

    const items = $derived<CrumbItem[]>([
        { id: '', name: $t_store('chrome.my_drive'), index: -1 },
        ...$breadcrumbPath.map((f, i) => ({ id: f.id, name: f.name, index: i })),
    ]);
    const atRoot = $derived($breadcrumbPath.length === 0);

    function registerRoot(el: HTMLElement) {
        drag.registerRoot(el);
    }

    function onDragOver(event: DragEvent, item: CrumbItem): void {
        if (!drag.isActive()) return;
        const el = event.currentTarget as HTMLElement;
        const allowed = drag.canDrop(item.id);
        drag.highlight(el, allowed);
        if (event.dataTransfer) event.dataTransfer.dropEffect = allowed ? 'move' : 'none';
        if (allowed) event.preventDefault();
    }

    function onDragLeave(event: DragEvent): void {
        const el = event.currentTarget as HTMLElement;
        if (event.relatedTarget && el.contains(event.relatedTarget as Node)) return;
        drag.leave(el);
    }

    function onDrop(event: DragEvent, item: CrumbItem): void {
        if (!drag.isActive() || !drag.canDrop(item.id)) return;
        event.preventDefault();
        event.stopPropagation();
        drag.dropOn(event.currentTarget as HTMLElement, item.id);
    }
</script>

<button
    id="breadcrumb-back"
    class="icon-btn back-btn"
    type="button"
    title={$t_store('common.back')}
    aria-label={$t_store('common.back')}
    disabled={atRoot}
    style={`opacity: ${atRoot ? '0.35' : '1'}`}
    onclick={onBack}
>
    <ChevronLeftIcon size={18} aria-hidden="true" />
</button>
<div id="breadcrumb-path" class="breadcrumb-path">
    {#each items as item, idx (item.index === -1 ? 'root' : item.id)}
        {#if idx > 0}
            <span class="breadcrumb-sep">/</span>
        {/if}
        {#if item.index === -1}
            <button
                type="button"
                class="breadcrumb-link"
                data-index="-1"
                use:registerRoot
                onclick={() => onNavigate(-1)}
                ondragover={(event) => onDragOver(event, item)}
                ondragleave={onDragLeave}
                ondrop={(event) => onDrop(event, item)}
            >
                {item.name}
            </button>
        {:else}
            <button
                type="button"
                class="breadcrumb-link"
                data-index={String(item.index)}
                onclick={() => onNavigate(item.index)}
                ondragover={(event) => onDragOver(event, item)}
                ondragleave={onDragLeave}
                ondrop={(event) => onDrop(event, item)}
            >
                {item.name}
            </button>
        {/if}
    {/each}
</div>
