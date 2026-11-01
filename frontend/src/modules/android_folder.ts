/**
 * Asking Android for a folder.
 *
 * Wails refuses directory selection on Android, and its reason is sound as far
 * as it goes: the Storage Access Framework returns document-tree URIs, and its
 * dialog API has nowhere to put one. So this does not go through that API. The
 * app's own Android bridge walks the chosen tree and answers with a manifest,
 * then copies files into the app cache on demand, a handful at a time.
 *
 * The manifest is the whole point. Copying the tree up front is simpler, but a
 * 5 GB folder then needs 5 GB of free cache and a long silent wait before a
 * single byte is uploaded. With the manifest the cache only ever holds the
 * window currently going up.
 */

import { callBridge, hasBridgeMethod } from './android_bridge';

/** One file in the picked tree. Nothing is copied until it is materialized. */
export interface AndroidFolderFile {
    /** Opaque to us; the bridge resolves it back to a document URI. */
    id: string;
    /** Path below the picked folder, "/" separated. */
    rel: string;
    /** Size in bytes, or -1 when the provider does not say. A file of unknown size is never skipped. */
    size: number;
    /** Modification time in unix milliseconds, 0 when the provider does not say. */
    modified: number;
}

export interface AndroidFolderManifest {
    root: string;
    files: AndroidFolderFile[];
    /** Temporary camera files the bridge left out of the walk. */
    skipped: number;
}

/**
 * Camera and gallery copies that are not the user's files: a capture still being
 * written, or one thrown away. The bridge filters these out already; this is the
 * same rule, kept here so a manifest from any other source cannot bring them in.
 * It mirrors isTemporaryMediaName in MainActivity.java and the Go importer.
 */
export function isTemporaryMediaName(name: string): boolean {
    return /^\.(?:pending|trashed)-[0-9]+-.+/i.test(name);
}

/** Whether this build can ask for a folder at all. */
export function canPickFolder(): boolean {
    return hasBridgeMethod('pickFolder');
}

/** A size the provider reported, or -1 when it did not report one. */
function knownSize(value: unknown): number {
    if (value === null || value === undefined || value === '') return -1;
    const size = Number(value);
    return Number.isFinite(size) && size >= 0 ? Math.floor(size) : -1;
}

function parseManifest(raw: string): AndroidFolderManifest | null {
    if (!raw) return null;
    const parsed = JSON.parse(raw) as { root?: unknown; files?: unknown; skipped?: unknown };
    const entries = Array.isArray(parsed.files) ? parsed.files : [];
    const files: AndroidFolderFile[] = [];
    let skipped = Math.max(0, Math.floor(Number(parsed.skipped) || 0));
    for (const entry of entries) {
        const file = entry as { id?: unknown; rel?: unknown; size?: unknown; modified?: unknown };
        const id = String(file.id ?? '');
        const rel = String(file.rel ?? '');
        if (!id || !rel) continue;
        if (isTemporaryMediaName(rel.split('/').pop() ?? '')) {
            skipped += 1;
            continue;
        }
        files.push({
            id,
            rel,
            size: knownSize(file.size),
            modified: Math.max(0, Number(file.modified) || 0),
        });
    }
    return { root: String(parsed.root ?? ''), files, skipped };
}

/**
 * Opens the system folder picker and resolves with what is in the chosen tree,
 * or with null if it was dismissed. The walk happens on the bridge side and is
 * slow on a deep tree, so this can take seconds after the dialog closes.
 */
export function pickAndroidFolder(): Promise<AndroidFolderManifest | null> {
    return callBridge('pickFolder', [], 'This build cannot open a folder picker.').then(parseManifest);
}

/** What the bridge made of one batch of ids. */
export interface AndroidMaterialized {
    /** Local cache path per id that was copied. */
    paths: Map<string, string>;
    /** Ids whose staged copy does not match the size or time the walk recorded: the file changed while copying. */
    changed: Set<string>;
    /** Ids that exist only in the cloud and have no local bytes on this device. */
    notOnDevice: Set<string>;
}

/**
 * Copies the named files into the app cache. An id the bridge could not read is
 * absent from paths rather than failing the whole batch; the other two sets say
 * why a file is missing or was copied from a file that moved underneath us.
 */
export async function materializeAndroidFiles(ids: string[]): Promise<AndroidMaterialized> {
    const raw = await callBridge('materializeFiles', [JSON.stringify(ids)], 'This build cannot read folder contents.');
    const parsed = JSON.parse(raw || '{}') as { paths?: unknown; changed?: unknown; notOnDevice?: unknown };
    const record = parsed.paths && typeof parsed.paths === 'object' ? parsed.paths as Record<string, unknown> : {};
    const paths = new Map<string, string>();
    for (const [id, path] of Object.entries(record)) {
        const local = String(path ?? '');
        if (local) paths.set(id, local);
    }
    return {
        paths,
        changed: idSet(parsed.changed),
        notOnDevice: idSet(parsed.notOnDevice),
    };
}

function idSet(value: unknown): Set<string> {
    return new Set(Array.isArray(value) ? value.map(String).filter(Boolean) : []);
}

/** Drops the cache copies made for these ids. Safe for ids never materialized. */
export async function releaseAndroidFiles(ids: string[]): Promise<void> {
    await callBridge('releaseFiles', [JSON.stringify(ids)], 'This build cannot release folder contents.');
}

function pathSegments(path: string): string[] {
    return path.split('/').filter((segment) => segment && segment !== '.' && segment !== '..');
}

/** The directory a manifest entry belongs in, "" for the top of the tree. */
export function folderPathFor(root: string, rel: string): string {
    const segments = pathSegments(`${root}/${rel}`);
    segments.pop();
    return segments.join('/');
}

/**
 * Every directory the manifest implies, shallowest first, because a folder
 * cannot be created before the parent whose id it needs.
 */
export function folderPathsFor(manifest: AndroidFolderManifest): string[] {
    const paths = new Set<string>();
    const root = pathSegments(manifest.root).join('/');
    if (root) paths.add(root);
    for (const file of manifest.files) {
        const segments = pathSegments(folderPathFor(manifest.root, file.rel));
        for (let depth = 1; depth <= segments.length; depth += 1) {
            paths.add(segments.slice(0, depth).join('/'));
        }
    }
    return [...paths].sort((a, b) => a.split('/').length - b.split('/').length);
}
