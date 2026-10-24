<script lang="ts">
    // What Photos is showing, and how to change it. Two toggle buttons rather
    // than a tablist: the grid and the timeline are not labelled tab panels,
    // they are one region re-rendered, and a pressed pair says exactly that
    // with the semantics the rest of the app already uses for a segment.
    import ChevronLeftIcon from 'phosphor-svelte/lib/CaretLeftIcon';
    import { showPhotos } from '../../modules/gallery';
    import { albumsWorthShowing } from './album_view';
    import { albumsView, galleryView, photosMode } from './gallery_store';
    import { t_store } from '../../i18n/i18n';

    interface Props {
        /**
         * The phone names the open album and offers Back in its top bar, where
         * every other drill-in puts them, so this bar leaves both out there.
         */
        albumInTopBar?: boolean;
    }

    let { albumInTopBar = false }: Props = $props();

    const tiles = $derived($albumsView.status === 'ready' ? $albumsView.tiles : []);
    // With no structure to show, the switch would only offer the view that is
    // already on screen. A dead control invites a click that does nothing.
    const switchable = $derived(albumsWorthShowing(tiles));
    const mode = $derived($photosMode);
    // The timeline's live total, shown beside the title the way the design
    // puts the item count next to "Photos".
    const photoTotal = $derived($galleryView.status === 'ready' ? $galleryView.source.timeline.totalCount : null);
    const photoCountLabel = $derived(photoTotal === null ? '' : $t_store('photos.photo_count', { count: photoTotal }));
    // The count is the grid's live figure for this folder, not the one the
    // tile carried when it was tapped: a refresh that lands photos in the
    // folder while it is open should move it.
    const albumCount = $derived(
        mode.kind === 'album'
            ? (tiles.find((tile) => tile.folderId === mode.tile.folderId)?.countLabel ?? mode.tile.countLabel)
            : '',
    );
</script>

<div class="photos-mode-bar">
    {#if mode.kind === 'timeline' && photoTotal !== null}
        <span class="photos-count">{photoCountLabel}</span>
    {/if}
    {#if mode.kind === 'album'}
        {#if !albumInTopBar}
            <button class="album-back" type="button" onclick={() => void showPhotos({ kind: 'albums' })}>
                <ChevronLeftIcon size={16} aria-hidden="true" />
                <span class="album-back-name">{mode.tile.name}</span>
            </button>
            <span class="album-back-count">{albumCount}</span>
        {/if}
    {:else if switchable}
        <div class="segmented photos-modes" role="group" aria-label={$t_store('photos.photos_view_label')}>
            <button
                type="button"
                aria-pressed={mode.kind === 'albums'}
                onclick={() => void showPhotos({ kind: 'albums' })}
            >{$t_store('photos.albums_tab')}</button>
            <button
                type="button"
                aria-pressed={mode.kind === 'timeline'}
                onclick={() => void showPhotos({ kind: 'timeline' })}
            >{$t_store('photos.all_photos')}</button>
        </div>
    {/if}
</div>
