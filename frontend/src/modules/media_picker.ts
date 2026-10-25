import { hasBridgeMethod } from './android_bridge';
import { selectFiles } from '../api';

interface MediaBridge {
    pickUploadMedia: () => Promise<{ paths: string[]; token: string }>;
    releaseUploadMedia: (token: string) => Promise<unknown>;
}
function iosBridge(): MediaBridge | undefined {
    return (window as Window & { tdriveIOSPhotos?: MediaBridge }).tdriveIOSPhotos;
}
export function mediaPickerAvailable(): boolean {
    return typeof iosBridge()?.pickUploadMedia === 'function' || hasBridgeMethod('prepareMediaPicker');
}
export async function pickUploadMedia(): Promise<{ paths: string[]; release: () => Promise<void> }> {
    const bridge = iosBridge();
    if (bridge?.pickUploadMedia) {
        const result = await bridge.pickUploadMedia();
        return { paths: result.paths, release: async () => { await bridge.releaseUploadMedia(result.token); } };
    }
    const android = (window as Window & { wails?: { prepareMediaPicker?: () => void } }).wails;
    if (!android?.prepareMediaPicker) throw new Error('Photo library picker is unavailable in this build.');
    android.prepareMediaPicker();
    return { paths: await selectFiles(), release: async () => {} };
}
