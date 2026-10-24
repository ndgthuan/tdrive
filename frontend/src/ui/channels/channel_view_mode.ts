/** How the source's posts are laid out on the desktop: square tiles or a table of rows. */
export type SourceViewMode = 'grid' | 'list';

export const SOURCE_VIEW_STORAGE_KEY = 'tdrive_source_view';

/** The saved layout, or the grid when nothing usable is saved or storage is blocked. */
export function load_source_view_mode(): SourceViewMode {
    try {
        return globalThis.localStorage?.getItem(SOURCE_VIEW_STORAGE_KEY) === 'list' ? 'list' : 'grid';
    } catch {
        return 'grid';
    }
}

/** Remembers the layout for the next visit. A private window may refuse storage, and the layout still applies for this visit. */
export function save_source_view_mode(mode: SourceViewMode): void {
    try {
        globalThis.localStorage?.setItem(SOURCE_VIEW_STORAGE_KEY, mode);
    } catch {
        // Storage is optional; the choice holds until the view is left.
    }
}
