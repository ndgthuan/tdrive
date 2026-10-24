<script lang="ts">
    import ArrowDownIcon from 'phosphor-svelte/lib/ArrowDownIcon';
    import ArrowUpIcon from 'phosphor-svelte/lib/ArrowUpIcon';
    import FolderPlusIcon from 'phosphor-svelte/lib/FolderPlusIcon';
    import ImagesIcon from 'phosphor-svelte/lib/ImagesIcon';
    import Link2Icon from 'phosphor-svelte/lib/LinkSimpleIcon';
    import SearchIcon from 'phosphor-svelte/lib/MagnifyingGlassIcon';
    import Trash2Icon from 'phosphor-svelte/lib/TrashIcon';
    import { isMobilePlatform, listChannelSourceTopics, listChannelMedia, listMountableDrives } from '../api';
    import tdriveLogo from '../assets/images/tdrive_logo.png';
    import { openEncryptionSettingsModal } from '../modules/modals/encryption_settings';
    import { openLogoutModal } from '../modules/modals/logout';
    import { breadcrumbDrag, navigateBack, navigateToIndex } from '../modules/navigation';
    import { cancelTransfersInDirection, cancelUploadFile, clearHistory } from '../modules/notif_bell';
    import { ensureProfileLoaded } from '../modules/profile_menu';
    import { askEmptyTrash, openTrash, trashBusyKey, trashEntries } from '../modules/trash/controller';
    import {
        channelPhoto,
        loadChannelSources,
        openChannelPicker,
        openChannelPost,
        openInTelegram,
        recentChannelPages,
        showChannel,
        showChannelActions,
        showPostActions,
    } from '../modules/channel_sources';
    import { clearSelection, openSelectedItemsDelete, openSelectedItemsDownload, openSelectedItemsMove, selectAllRows } from '../modules/selection';
    import { chooseFilesForCurrentFolder, chooseFolderForCurrentFolder } from '../modules/transfers';
    import { openNewFolderModal } from '../modules/modals/folder';
    import {
        handleDriveClick,
        handlePendingClick,
        showPendingActionsMenu,
        showSharedActionsMenu,
    } from '../modules/sidebar';
    import { setFileSortKey, fileSortState } from './file_list/file_sort_store';
    import { file_list_column_label } from './file_list/column_mode_store';
    import { t_store, type Translator } from '../i18n/i18n';
    import type { FileSortKey } from './file_list/file_sort';
    import Breadcrumb from './chrome/breadcrumb.svelte';
    import ProfileMenu from './chrome/profile_menu.svelte';
    import UploadMenu from './chrome/upload_menu.svelte';
    import PhotosModeBar from './gallery/photos_mode_bar.svelte';
    import PhotosSurface from './gallery/photos_surface.svelte';
    import NotifBell from './notifications/notif_bell.svelte';
    import AiPanel from './ai/ai_panel.svelte';
    import AiPanelToggle from './ai/ai_panel_toggle.svelte';
    import SelectionBar from './selection/selection_bar.svelte';
    import { selectedFileRowKeys } from './file_list/row_state_store';
    import { selectionBarState, selectionModeOn } from './selection/selection_bar_store';
    import DriveList from './sidebar/drive_list.svelte';
    import MountControl from './mount/mount_control.svelte';
    import FeatureLayer from './app/feature_layer.svelte';
    import ChannelNav from './channels/channel_nav.svelte';
    import { sidebarState } from './sidebar/sidebar_store';
    import ChannelView from './channels/channel_view.svelte';
    import { channelSources, openChannel } from './channels/channel_store';
    import { sourceKey } from './channels/channel_model';

    interface Props {
        dashboardVisible: boolean;
    }

    let { dashboardVisible }: Props = $props();

    // OS mounts are desktop-only; the phone builds ship no WebDAV connector.
    // The desktop list's selection mode swaps the column labels for a count and
    // Select all / Clear. The phone keeps its own header.
    const selecting_on_desktop = $derived(!isMobilePlatform() && $selectionModeOn);
    const selection_count_label = $derived($t_store('shell.selected', { count: $selectionBarState.count }));
    const mountAvailable = !isMobilePlatform();
    // The AI assistant panel is a desktop surface; phones have no side column for it.
    const aiAvailable = !isMobilePlatform();
    // Desktop-only chrome: phones update through their app stores.
    const updaterAvailable = !isMobilePlatform();
    // The Share section only appears once there is something in it: a shared
    // drive, or a join request still waiting for approval (its row lives here too).
    const has_shared_drives = $derived($sidebarState.shared.length > 0 || $sidebarState.pending.length > 0);

    const SORT_COLUMN_KEY: Partial<Record<FileSortKey, string>> = {
        name: 'shell.column_name',
        date: 'shell.column_date',
        size: 'shell.column_size',
    };

    // Takes the translator so the template re-runs when the language changes.
    function sortButtonLabel(translate: Translator, key: FileSortKey): string {
        const column = translate(SORT_COLUMN_KEY[key] ?? 'shell.column_name');
        const active = $fileSortState.key === key;
        if (!active) return translate('shell.sort_by', { column });
        return translate($fileSortState.direction === 'asc' ? 'shell.sort_by_desc' : 'shell.sort_by_asc', { column });
    }

    function sortAriaValue(key: FileSortKey): 'ascending' | 'descending' | 'none' {
        if ($fileSortState.key !== key) return 'none';
        return $fileSortState.direction === 'asc' ? 'ascending' : 'descending';
    }

</script>

{#snippet sortIndicator(key: FileSortKey)}
    {#if $fileSortState.key === key}
        {#if $fileSortState.direction === 'asc'}
            <ArrowUpIcon class="sort-direction-up" size={12} aria-hidden="true" weight="bold" />
        {:else}
            <ArrowDownIcon class="sort-direction-down" size={12} aria-hidden="true" weight="bold" />
        {/if}
    {/if}
{/snippet}

<!-- Semantic dashboard chrome remains mounted after runtime startup. The typed
     root owns visibility; FeatureLayer owns the surfaces imperative controllers
     still fill in (the file list, the gallery, the selection bar, the upload
     menu) plus the modal layer.

     The live chrome this shell renders itself -- the drive lists, the bell, the
     account menu, the breadcrumb -- is gated on dashboardVisible so it lives
     exactly as long as the dashboard does, which is what it did when a portal
     mounted it. Built earlier it would sit behind the login screen with its
     window listeners, its registered drag root and the last session's drive rows
     still live under a hidden container, and logging out would stop clearing any
     of that. -->
<div
    id="success-screen"
    class="dashboard-container"
    hidden={!dashboardVisible}
    aria-hidden={dashboardVisible ? undefined : 'true'}
>
    <aside class="sidebar">
        <div class="logo">
            <img class="logo-mark" src={tdriveLogo} alt="" width="44" height="28" />
            <span>TDrive</span>
        </div>

        <nav id="drives-nav" class="drives-nav" tabindex="-1" aria-label={$t_store('shell.drives_label')}>
            <div class="drives-scroll">
                <div class="drives-section">
                    <!-- The personal drive is the first row, named My Drive as
                         the breadcrumb root is, so it needs no caption of its own.
                         The .drives-list wrapper is the column the rows stack in
                         and the sibling the Photos row takes its top margin
                         from, so it stays put; only the id a portal aimed at is
                         gone. -->
                    <div class="drives-list">
                        {#if dashboardVisible}
                            <DriveList kind="personal" onDriveClick={handleDriveClick} />
                        {/if}
                    </div>
                    <div class="drives-list">
                        <button id="nav-photos" class="drive-item nav-photos-item" type="button" title={$t_store('shell.photos')}>
                            <ImagesIcon class="icon" size={18} aria-hidden="true" />
                            <span class="drive-item-title">{$t_store('shell.photos')}</span>
                        </button>
                        <!-- Trash sits with the drive's other destinations,
                             where a user who just lost a file looks for it,
                             rather than behind the account menu. -->
                        <button
                            id="nav-trash"
                            class="drive-item"
                            type="button"
                            title={$t_store('shell.trash')}
                            onclick={openTrash}
                        >
                            <Trash2Icon class="icon" size={18} aria-hidden="true" />
                            <span class="drive-item-title">{$t_store('shell.trash')}</span>
                        </button>
                    </div>
                </div>

                {#if dashboardVisible && has_shared_drives}
                    <div class="drives-section">
                        <div class="drives-section-title">{$t_store('shell.share_section')}</div>
                        <div class="drives-list">
                            <DriveList
                                kind="shared"
                                onDriveClick={handleDriveClick}
                                onDriveActions={showSharedActionsMenu}
                                onPendingClick={handlePendingClick}
                                onPendingActions={showPendingActionsMenu}
                            />
                        </div>
                    </div>
                {/if}
                <!-- Telegram channels the user added: read-only places beside
                     the drives, opened in the main area without changing the
                     active drive. -->
                {#if dashboardVisible}
                    <div class="drives-section">
                        <ChannelNav
                            sources={$channelSources}
                            activeKey={$openChannel ? sourceKey($openChannel) : null}
                            loadPhoto={channelPhoto}
                            onSelect={showChannel}
                            onActions={showChannelActions}
                            onAdd={openChannelPicker}
                            onRetry={loadChannelSources}
                        />
                    </div>
                {/if}
            </div>

            <div class="drives-actions">
                <button id="open-new-drive" class="drive-action-btn" type="button" title={$t_store('shell.create_shared_drive_title')}>
                    <FolderPlusIcon class="icon" size={16} aria-hidden="true" />
                    {$t_store('shell.create_shared_drive')}
                </button>
                <button id="open-join-drive" class="drive-action-btn" type="button" title={$t_store('shell.join_with_link_title')}>
                    <Link2Icon class="icon" size={16} aria-hidden="true" />
                    {$t_store('shell.join_with_link')}
                </button>
                {#if dashboardVisible && mountAvailable}
                    <MountControl variant="sidebar" loadDrives={listMountableDrives} />
                {/if}
            </div>
        </nav>

        <div class="storage-info">
            <p>{$t_store('shell.storage')}<span id="storage-used">{$t_store('shell.storage_default')}</span></p>
        </div>
    </aside>

    <main class="main-content" class:channel-mode={dashboardVisible && $openChannel !== null} class:has-selection={$selectedFileRowKeys.size > 0}>
        <header>
            <div class="search-bar">
                <SearchIcon class="search-icon" size={16} aria-hidden="true" />
                <input id="search-input" type="text" placeholder={$t_store('shell.search_files_placeholder')} autocomplete="off" spellcheck="false" aria-label={$t_store('shell.search_files')}>
            </div>

            <div class="header-actions">
                {#if dashboardVisible && aiAvailable}
                    <AiPanelToggle />
                {/if}
                {#if dashboardVisible}
                    <NotifBell onCancelDirection={cancelTransfersInDirection} onCancelFile={cancelUploadFile} onClearHistory={clearHistory} />
                {/if}

                <!-- .upload-menu-wrap is position: relative, and it is what the
                     menu's own absolutely positioned popover anchors to, so the
                     wrapper stays exactly where it was; only the id a portal
                     aimed at is gone. Held back until the dashboard is up so the
                     menu's window and document listeners live no longer than the
                     dashboard does, which is what a portal that only ran on the
                     dashboard view gave it. -->
                <div class="upload-menu-wrap">
                    {#if dashboardVisible}
                        <UploadMenu
                            onFiles={chooseFilesForCurrentFolder}
                            onFolder={chooseFolderForCurrentFolder}
                            onNewFolder={openNewFolderModal}
                        />
                    {/if}
                </div>
                <!-- The account menu is a plain child of the header: its popover
                     is positioned against .header-actions, which is the nearest
                     positioned ancestor either way, so nothing about where it
                     lands changes by rendering it here. Held back until the
                     dashboard is up because the menu opens itself when something
                     asks for the updates panel, and that request can come from
                     the login screen -- mounted earlier, the user would arrive at
                     the dashboard with the account menu already hanging open. -->
                {#if dashboardVisible}
                    <ProfileMenu
                        {updaterAvailable}
                        onOpen={ensureProfileLoaded}
                        onEncryptionSettings={openEncryptionSettingsModal}
                        onLogout={openLogoutModal}
                    />
                {/if}
            </div>
        </header>

        <div class="drive-breadcrumb">
            <!-- The back button and the crumb trail are flex items of this row.
                 The host they used to arrive in was display:contents, so it was
                 never a box of its own and taking it away moves nothing. -->
            {#if dashboardVisible}
                <Breadcrumb onNavigate={navigateToIndex} onBack={navigateBack} drag={breadcrumbDrag} />
            {/if}
            <div id="gallery-title" class="gallery-title">{$t_store('shell.photos')}</div>
            <PhotosModeBar />
            <div id="trash-title" class="trash-title">{$t_store('shell.trash')}</div>
            <!-- Destructive and irreversible, so it only ever opens the
                 confirmation. With nothing to empty it is absent rather than
                 disabled: a dead control invites a click that cannot do
                 anything. It is disabled only while a mutation is in flight. -->
            {#if $trashEntries.length > 0}
                <button
                    id="trash-empty-btn"
                    class="trash-empty-btn"
                    type="button"
                    disabled={$trashBusyKey !== ''}
                    onclick={askEmptyTrash}
                >
                    <Trash2Icon size={15} aria-hidden="true" />
                    <span>{$t_store('shell.empty_trash')}</span>
                </button>
            {/if}
        </div>

        <div class="file-table-header" role="row" aria-rowindex="1">
          {#if selecting_on_desktop}
            <div class="col-selection" role="columnheader" aria-colindex="1">
                <span class="selection-header-count" aria-live="polite">{selection_count_label}</span>
                <button class="selection-header-btn" type="button" data-testid="select-all" onclick={() => selectAllRows()}>{$t_store('shell.select_all')}</button>
                <button class="selection-header-btn" type="button" data-testid="clear-selection" onclick={() => clearSelection()}>{$t_store('shell.clear')}</button>
            </div>
          {:else}
            <div class="col-name" role="columnheader" aria-colindex="1" aria-sort={sortAriaValue('name')}>
                <button
                    class:active={$fileSortState.key === 'name'}
                    class="file-sort-button"
                    type="button"
                    aria-label={sortButtonLabel($t_store, 'name')}
                    onclick={() => setFileSortKey('name')}
                >
                    <span>{$t_store('shell.column_name')}</span>
                    <span class="file-sort-indicator" aria-hidden="true">{@render sortIndicator('name')}</span>
                </button>
            </div>
            <div class="col-date" role="columnheader" aria-colindex="2" aria-sort={sortAriaValue('date')}>
                <button
                    class:active={$fileSortState.key === 'date'}
                    class="file-sort-button"
                    type="button"
                    aria-label={sortButtonLabel($t_store, 'date')}
                    onclick={() => setFileSortKey('date')}
                >
                    <span>{$file_list_column_label}</span>
                    <span class="file-sort-indicator" aria-hidden="true">{@render sortIndicator('date')}</span>
                </button>
            </div>
            <div class="col-size" role="columnheader" aria-colindex="3" aria-sort={sortAriaValue('size')}>
                <button
                    class:active={$fileSortState.key === 'size'}
                    class="file-sort-button"
                    type="button"
                    aria-label={sortButtonLabel($t_store, 'size')}
                    onclick={() => setFileSortKey('size')}
                >
                    <span>{$t_store('shell.column_size')}</span>
                    <span class="file-sort-indicator" aria-hidden="true">{@render sortIndicator('size')}</span>
                </button>
            </div>
            <div class="col-actions" role="columnheader" aria-colindex="4">
                <span>{$t_store('shell.column_actions')}</span>
            </div>
          {/if}
        </div>

        <div id="file-list" class="file-list-box" data-file-drop-target role="grid" aria-label={$t_store('shell.files_label')} aria-multiselectable="true" aria-colcount="4" tabindex="-1"></div>

        <!-- The selection bar floats over the bottom centre of this pane. It is
             a child of the pane, not of the header or the Actions column, so it
             can never cover the title or the column labels. The controller reveals
             it by flipping its inline display once a row is picked. -->
        <div id="selection-bar" class="selection-bar" style="display: none;" role="status" aria-live="polite">
            {#if dashboardVisible}
                <SelectionBar
                    onDownload={openSelectedItemsDownload}
                    onMove={openSelectedItemsMove}
                    onDelete={openSelectedItemsDelete}
                    onClear={clearSelection}
                />
            {/if}
        </div>

        <!-- #gallery-view is the gallery's scroll container, not a seam: the
             component measures it, the thumbnail controller uses it as its
             IntersectionObserver root, and the click delegation, the preview
             modal's return-to-grid and the phone's scroll-to-top all look it up
             by this id. So the element stays exactly as it was and only the
             portal that filled it from elsewhere is gone. Held back until the
             dashboard is up so the grid's window listeners and resize observer
             live no longer than the dashboard does. -->
        <div id="gallery-view" class="gallery-view" tabindex="-1" aria-label={$t_store('shell.photos')}>
            {#if dashboardVisible}
                <PhotosSurface />
            {/if}
        </div>
        <!-- Keyed by connection so re-adding a channel starts a fresh view
             instead of paging with the old connection. -->
        {#if dashboardVisible && $openChannel}
            {#key sourceKey($openChannel)}
                <ChannelView
                    source={$openChannel}
                    fetchMedia={listChannelMedia}
                        fetchTopics={listChannelSourceTopics}
                    onOpenPost={openChannelPost}
                    onOpenTelegram={openInTelegram}
                    onPostActions={showPostActions}
                    recentPages={recentChannelPages}
                />
            {/key}
        {/if}
    </main>

    {#if dashboardVisible && aiAvailable}
        <AiPanel />
    {/if}
</div>

<FeatureLayer />
