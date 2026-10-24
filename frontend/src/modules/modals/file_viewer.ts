import { t } from '../../i18n/i18n';
import { closeMedia, isMobilePlatform, setScreenProtect, onRuntimeEvent, openStream, type MediaOpenResult } from '../../api';
import { formatBytes } from '../../utils';
import { enqueueDownload } from '../transfers';
import { notify } from '../notifications';
import { accessEncryptedResource } from '../encryption';
import { acquireScreenProtection } from '../../ui/viewers/view_only_guards';
import { toAppError } from '../errors';
import { canOpenFileViewer, fileKindLabel, fileOpenKind } from '../media_types';
import {
    closeFileViewerView,
    openFileViewerView,
    setFileViewerError,
    setFileViewerLoading,
    type FileViewerKind,
} from '../../ui/viewers/file_viewer_store';

export interface FileViewerTarget {
    id: number;
    name: string;
    size: number;
    encrypted?: boolean;
    protected?: boolean;
    /** Backend-admitted viewer kind for a read-only source, including nameless attachments. */
    kind?: FileViewerKind;
}



let viewerHost: HTMLElement | null = null;
let lifecycleObserver: MutationObserver | null = null;
let activeToken = '';
let activeTarget: FileViewerTarget | null = null;
let activeReadOnly = false;
// A read-only source post may still be saved; the viewer keeps the handler the
// source supplied rather than an id that only a My Drive file has.
let activeSourceDownload: (() => void) | null = null;
let releaseScreenProtection: (() => void) | null = null;

function syncScreenProtection(protectedContent: boolean): void {
    if (protectedContent && isMobilePlatform()) {
        releaseScreenProtection ??= acquireScreenProtection(setScreenProtect);
    } else {
        releaseScreenProtection?.();
        releaseScreenProtection = null;
    }
}
let openSeq = 0;
let encryptedSessionsEpoch = 0;
let unsubscribeEncryptedSessionsClosed: (() => void) | null = null;

function bindEncryptedMediaLifecycle(): void {
    if (unsubscribeEncryptedSessionsClosed) return;
    unsubscribeEncryptedSessionsClosed = onRuntimeEvent('encrypted_media_sessions_closed', () => {
        encryptedSessionsEpoch += 1;
        if (activeTarget?.encrypted) closeFileViewer();
    });
}

export function activateFileViewerModal(): () => void {
    const host = document.getElementById('viewer-modal');
    if (!host) return () => {};
    if (viewerHost === host) return teardownFileViewerModal;

    teardownFileViewerModal();
    viewerHost = host;
    bindEncryptedMediaLifecycle();
    lifecycleObserver = new MutationObserver(() => {
        if (!host.isConnected) teardownFileViewerModal();
    });
    lifecycleObserver.observe(document.body, { childList: true, subtree: true });
    return teardownFileViewerModal;
}

export function teardownFileViewerModal(): void {
    lifecycleObserver?.disconnect();
    lifecycleObserver = null;
    viewerHost = null;
    unsubscribeEncryptedSessionsClosed?.();
    unsubscribeEncryptedSessionsClosed = null;
    closeFileViewer();
}



export async function openFileViewer(target: FileViewerTarget, sourceOpener?: () => Promise<MediaOpenResult>, sourceDownload?: () => void): Promise<void> {
    const kind = sourceOpener && target.kind ? target.kind : fileOpenKind(target.name);
    if (!(sourceOpener && target.kind) && !canOpenFileViewer(target.name)) {
        notify({ level: 'warning', title: t('messages.viewer_kind_unavailable', { kind: fileKindLabel(target.name) }) });
        return;
    }
    const seq = ++openSeq;
    // The backend can reveal encryption only after a pending open completes.
    const encryptedEpoch = encryptedSessionsEpoch;
    const nextTarget = {
        id: Number(target.id || 0),
        name: String(target.name || t('messages.viewer_file_default')),
        size: Number(target.size || 0),
        encrypted: Boolean(target.encrypted),
        protected: Boolean(target.protected),
    };
    activeTarget = nextTarget;
    activeReadOnly = Boolean(sourceOpener);
    activeSourceDownload = sourceOpener ? sourceDownload ?? null : null;
    closeFileViewerView();
    await releaseActiveSession();
    if (seq !== openSeq) return;
    syncScreenProtection(nextTarget.protected);

    openFileViewerView({
        kind: kind as FileViewerKind,
        token: '',
        url: '',
        title: activeTarget.name,
        meta: `${fileKindLabel(activeTarget.name)} · ${formatBytes(activeTarget.size)}`,
        mimeType: '',
        loading: true,
        error: '',
        readOnly: Boolean(sourceOpener),
        downloadable: Boolean(activeSourceDownload) && !nextTarget.protected,
        protected: nextTarget.protected,
    });

    try {
        const opened = await accessEncryptedResource(
            nextTarget.encrypted,
            () => sourceOpener ? sourceOpener() : openStream(nextTarget.id),
        );
        if (!opened) {
            if (seq === openSeq) closeFileViewer();
            return;
        }
        if (seq !== openSeq) {
            await closeMedia(opened.token);
            return;
        }
        if (sourceOpener && opened.kind && opened.kind !== kind) {
            activeToken = opened.token;
            await releaseActiveSession();
            if (seq === openSeq) {
                syncScreenProtection(false);
                setFileViewerError(t('messages.viewer_post_changed'));
            }
            return;
        }
        const encrypted = nextTarget.encrypted || Boolean(opened.info.encrypted);
        if (encrypted && encryptedEpoch !== encryptedSessionsEpoch) {
            closeFileViewer();
            await closeMedia(opened.token);
            return;
        }
        activeTarget = { ...nextTarget, encrypted, protected: nextTarget.protected || Boolean(opened.info.protected) };
        activeToken = opened.token;
        syncScreenProtection(Boolean(activeTarget.protected));
        openFileViewerView({
            kind: kind as FileViewerKind,
            token: opened.token,
            url: opened.url,
            title: opened.name || activeTarget.name,
            meta: `${fileKindLabel(opened.name || activeTarget.name)} · ${formatBytes(opened.info.plaintextSize || opened.info.storedSize || activeTarget.size)}`,
            mimeType: opened.mimeType,
            loading: false,
            error: '',
            readOnly: Boolean(sourceOpener),
            downloadable: Boolean(activeSourceDownload) && !activeTarget.protected,
            protected: activeTarget.protected,
        });
    } catch (error) {
        if (seq !== openSeq) return;
        syncScreenProtection(false);
        setFileViewerError(sourceOpener ? toAppError(error, { source: 'backend' }).message : String(error || t('messages.viewer_open_failed')));
    } finally {
        if (seq === openSeq) setFileViewerLoading(false);
    }
}

async function releaseActiveSession(): Promise<void> {
    const token = activeToken;
    activeToken = '';
    if (!token) return;
    try {
        await closeMedia(token);
    } catch {
        // Closing a stale loopback token is best-effort; the backend also
        // releases sessions on app shutdown.
    }
}

export function closeFileViewer(): void {
    openSeq += 1;
    void releaseActiveSession();
    activeTarget = null;
    activeReadOnly = false;
    activeSourceDownload = null;
    closeFileViewerView();
    syncScreenProtection(false);
}

export function downloadActiveFile(): void {
    if (!activeTarget || activeTarget.protected) return;
    if (activeReadOnly) {
        activeSourceDownload?.();
        return;
    }
    enqueueDownload(activeTarget.id, activeTarget.name, activeTarget.size);
}
