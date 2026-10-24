import { t, type Translator } from '../../i18n/i18n';
import type { BiometricKind } from '../../types';

export function biometricLabel(kind: BiometricKind | undefined, tr: Translator = t): string {
    switch (kind) {
        case 'touch_id': return tr('modals.bio_touch_id');
        case 'face_id': return tr('modals.bio_face_id');
        case 'windows_hello': return tr('modals.bio_windows_hello');
        default: return tr('modals.bio_generic');
    }
}

export function biometricUnavailableMessage(reason: string, tr: Translator = t): string {
    switch (reason) {
        case 'unsupported': return tr('modals.bio_unsupported');
        case 'not_enrolled': return tr('modals.bio_not_enrolled');
        case 'locked_out': return tr('modals.bio_locked_out');
        case 'signing_required': return tr('modals.bio_signing_required');
        case 'invalidated': return tr('modals.bio_invalidated');
        default: return tr('modals.bio_unavailable');
    }
}
