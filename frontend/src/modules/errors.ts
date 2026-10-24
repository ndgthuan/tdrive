import type { OperationErrorCode } from '../types';

import { t } from '../i18n/i18n';

const MAX_USER_MESSAGE_LENGTH = 240;
const MAX_DIAGNOSTIC_MESSAGE_LENGTH = 480;
const MAX_DIAGNOSTIC_STACK_LENGTH = 1_600;
const MAX_CAUSE_DEPTH = 5;

const LOCAL_PATH = /(?:file:\/\/)?\/(?:Users|home|tmp|private|var\/folders)\/[^\s',;)]+/gi;
const WINDOWS_PATH = /\b[A-Z]:\\(?:[^\\\r\n]+\\)*[^\s,;)'"]+/gi;
const HOME_PATH = /(^|\s)~\/(?:[^\s',;)]+)/g;
const SECRET_ASSIGNMENT = /\b(api[_ -]?hash|access[_ -]?token|refresh[_ -]?token|token|password|secret|authorization)\b\s*[:=]\s*(?:"[^"]*"|'[^']*'|[^\s,;]+)/gi;
const URL_SECRET = /([?&](?:api[_-]?hash|access[_-]?token|refresh[_-]?token|token|password|secret)=)[^&#\s]+/gi;
const URL_CREDENTIALS = /(https?:\/\/)[^\s/@:]+:[^\s/@]+@/gi;
const JWT = /\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b/g;
const PRIVATE_KEY = /-----BEGIN [^-\r\n]*PRIVATE KEY-----[\s\S]*?-----END [^-\r\n]*PRIVATE KEY-----/gi;
const SAFE_CODE = /^[a-z][a-z0-9_]{0,63}$/;

export type AppErrorKind =
    | 'authentication'
    | 'permission'
    | 'not_found'
    | 'conflict'
    | 'validation'
    | 'network'
    | 'timeout'
    | 'storage'
    | 'canceled'
    | 'unavailable'
    | 'render'
    | 'unexpected';

export type AppErrorSource = 'application' | 'backend' | 'runtime' | 'startup' | 'render';

export interface SafeErrorDetails {
    readonly name: string;
    readonly message?: string;
    readonly code?: string;
    readonly stack?: string;
}

type AppErrorVariant<Kind extends AppErrorKind> = Readonly<{
    kind: Kind;
    title: string;
    message: string;
    retryable: boolean;
    source: AppErrorSource;
    code?: OperationErrorCode;
    details: SafeErrorDetails;
    /** The untouched failure is retained for control flow, never for presentation. */
    cause: unknown;
}>;

/** A UI-safe failure whose kind can be exhaustively narrowed by consumers. */
export type AppError = {
    [Kind in AppErrorKind]: AppErrorVariant<Kind>;
}[AppErrorKind];

export interface AppErrorOptions {
    source?: AppErrorSource;
}

interface ErrorPresentation {
    kind: AppErrorKind;
    title: string;
    message: string;
    retryable: boolean;
}

/**
 * Built on each call, not once at import: the copy is translated, so a
 * module-level t() would freeze whichever language was live at import time.
 */
function operation_presentations(): Record<OperationErrorCode, ErrorPresentation> {
    return {
            operation_failed: {
                kind: 'unexpected',
                title: t('leftovers.error_operation_failed_title'),
                message: '',
                retryable: true,
            },
            backend_unavailable: {
                kind: 'unavailable',
                title: t('leftovers.error_backend_unavailable_title'),
                message: t('leftovers.error_backend_unavailable_message'),
                retryable: true,
            },
            encryption_password_required: {
                kind: 'authentication',
                title: t('leftovers.error_encryption_password_required_title'),
                message: t('leftovers.error_encryption_password_required_message'),
                retryable: false,
            },
            invalid_encryption_password: {
                kind: 'authentication',
                title: t('leftovers.error_invalid_encryption_password_title'),
                message: t('leftovers.error_invalid_encryption_password_message'),
                retryable: false,
            },
            encryption_password_already_set: {
                kind: 'conflict',
                title: t('leftovers.error_encryption_password_already_set_title'),
                message: t('leftovers.error_encryption_password_already_set_message'),
                retryable: false,
            },
            encryption_policy_unavailable: {
                kind: 'unavailable',
                title: t('leftovers.error_encryption_policy_unavailable_title'),
                message: t('leftovers.error_encryption_policy_unavailable_message'),
                retryable: false,
            },
            biometric_canceled: {
                kind: 'canceled',
                title: t('leftovers.error_biometric_canceled_title'),
                message: t('leftovers.error_biometric_canceled_message'),
                retryable: false,
            },
            biometric_unavailable: {
                kind: 'unavailable',
                title: t('leftovers.error_biometric_unavailable_title'),
                message: t('leftovers.error_biometric_unavailable_message'),
                retryable: false,
            },
            biometric_invalidated: {
                kind: 'authentication',
                title: t('leftovers.error_biometric_invalidated_title'),
                message: t('leftovers.error_biometric_invalidated_message'),
                retryable: false,
            },
            biometric_not_enrolled: {
                kind: 'authentication',
                title: t('leftovers.error_biometric_not_enrolled_title'),
                message: t('leftovers.error_biometric_not_enrolled_message'),
                retryable: false,
            },
            biometric_busy: {
                kind: 'conflict',
                title: t('leftovers.error_biometric_busy_title'),
                message: t('leftovers.error_biometric_busy_message'),
                retryable: false,
            },
            biometric_failed: {
                kind: 'authentication',
                title: t('leftovers.error_biometric_failed_title'),
                message: t('leftovers.error_biometric_failed_message'),
                retryable: false,
            },
            biometric_cleanup_required: {
                kind: 'storage',
                title: t('leftovers.error_biometric_cleanup_required_title'),
                message: t('leftovers.error_biometric_cleanup_required_message'),
                retryable: false,
            },
            biometric_locked_out: {
                kind: 'authentication',
                title: t('leftovers.error_biometric_locked_out_title'),
                message: t('leftovers.error_biometric_locked_out_message'),
                retryable: false,
            },
            canceled: {
                kind: 'canceled',
                title: t('leftovers.error_canceled_title'),
                message: t('leftovers.error_canceled_message'),
                retryable: false,
            },
            deadline_exceeded: {
                kind: 'timeout',
                title: t('leftovers.error_deadline_exceeded_title'),
                message: t('leftovers.error_deadline_exceeded_message'),
                retryable: true,
            },
            not_found: {
                kind: 'not_found',
                title: t('leftovers.error_not_found_title'),
                message: t('leftovers.error_not_found_message'),
                retryable: true,
            },
            permission_denied: {
                kind: 'permission',
                title: t('leftovers.error_permission_denied_title'),
                message: t('leftovers.error_permission_denied_message'),
                retryable: false,
            },
            already_exists: {
                kind: 'conflict',
                title: t('leftovers.error_already_exists_title'),
                message: t('leftovers.error_already_exists_message'),
                retryable: false,
            },
            insufficient_storage: {
                kind: 'storage',
                title: t('leftovers.error_insufficient_storage_title'),
                message: t('leftovers.error_insufficient_storage_message'),
                retryable: false,
            },
            network_unavailable: {
                kind: 'network',
                title: t('leftovers.error_network_unavailable_title'),
                message: t('leftovers.error_network_unavailable_message'),
                retryable: true,
            },
            file_too_large: {
                kind: 'validation',
                title: t('leftovers.error_file_too_large_title'),
                message: t('leftovers.error_file_too_large_message'),
                retryable: false,
            },

    };
}

const APP_ERROR_KINDS: Record<AppErrorKind, true> = {
    authentication: true,
    permission: true,
    not_found: true,
    conflict: true,
    validation: true,
    network: true,
    timeout: true,
    storage: true,
    canceled: true,
    unavailable: true,
    render: true,
    unexpected: true,
};

const APP_ERROR_SOURCES: Record<AppErrorSource, true> = {
    application: true,
    backend: true,
    runtime: true,
    startup: true,
    render: true,
};

function property(value: unknown, key: string): unknown {
    if ((typeof value !== 'object' || value === null) && typeof value !== 'function') return undefined;
    try {
        return Reflect.get(value, key);
    } catch {
        return undefined;
    }
}

function textProperty(value: unknown, key: string): string | undefined {
    const candidate = property(value, key);
    return typeof candidate === 'string' ? candidate : undefined;
}

function redact(text: string): string {
    return text
        .replace(PRIVATE_KEY, '[private key redacted]')
        .replace(URL_CREDENTIALS, '$1[credentials redacted]@')
        .replace(URL_SECRET, '$1[redacted]')
        .replace(SECRET_ASSIGNMENT, (assignment, key: string) => {
            const separator = assignment.includes(':') ? ':' : '=';
            return key + separator + '[redacted]';
        })
        .replace(JWT, '[token redacted]')
        .replace(LOCAL_PATH, '[local path]')
        .replace(WINDOWS_PATH, '[local path]')
        .replace(HOME_PATH, '$1[local path]');
}

function singleLine(text: string, maxLength: number): string {
    return redact(text)
        .replace(/^(?:Error:?\s*)+/i, '')
        .replace(/\s+/g, ' ')
        .trim()
        .slice(0, maxLength);
}

function diagnosticText(text: string, maxLength: number): string {
    return redact(text)
        .replace(/\r\n?/g, '\n')
        .split('\n')
        .map((line) => line.replace(/[\t ]+/g, ' ').trimEnd())
        .join('\n')
        .trim()
        .slice(0, maxLength);
}

function causeChain(error: unknown): unknown[] {
    const chain: unknown[] = [];
    const seen = new Set<object>();
    let current: unknown = error;

    for (let depth = 0; depth < MAX_CAUSE_DEPTH && current !== undefined && current !== null; depth += 1) {
        if ((typeof current === 'object' && current !== null) || typeof current === 'function') {
            if (seen.has(current)) break;
            seen.add(current);
        }
        chain.push(current);
        current = property(current, 'cause');
    }

    return chain;
}

function rawMessage(chain: readonly unknown[]): string {
    for (const item of chain) {
        if (typeof item === 'string') return item;
        const message = textProperty(item, 'message');
        if (message) return message;
    }
    return '';
}

function knownOperationCode(chain: readonly unknown[]): OperationErrorCode | undefined {
    for (const item of chain) {
        const code = textProperty(item, 'code');
        if (code && Object.prototype.hasOwnProperty.call(operation_presentations(), code)) {
            return code as OperationErrorCode;
        }
    }
    return undefined;
}

function diagnosticCode(chain: readonly unknown[]): string | undefined {
    for (const item of chain) {
        const code = textProperty(item, 'code');
        if (code && SAFE_CODE.test(code)) return code;
    }
    return undefined;
}

function safeDetails(chain: readonly unknown[], raw: string): SafeErrorDetails {
    const first = chain[0];
    const rawName = textProperty(first, 'name');
    const name = singleLine(rawName || (typeof first === 'string' ? 'Error' : 'UnknownError'), 80)
        || 'UnknownError';
    const message = diagnosticText(raw, MAX_DIAGNOSTIC_MESSAGE_LENGTH);
    const rawStack = chain.map((item) => textProperty(item, 'stack')).find(Boolean);
    const stack = rawStack ? diagnosticText(rawStack, MAX_DIAGNOSTIC_STACK_LENGTH) : '';
    const code = diagnosticCode(chain);

    return Object.freeze({
        name,
        ...(message ? { message } : {}),
        ...(code ? { code } : {}),
        ...(stack ? { stack } : {}),
    });
}

/** Telegram's own refusal codes, mapped to the locale key of the sentence shown for each. */
const TELEGRAM_CODES = new Map<string, string>([
    ['FILE_REFERENCE_EXPIRED', 'leftovers.error_telegram_file_reference_expired'],
    ['MESSAGE_ID_INVALID', 'leftovers.error_telegram_message_id_invalid'],
    ['CHAT_WRITE_FORBIDDEN', 'leftovers.error_telegram_chat_write_forbidden'],
    ['CHAT_ADMIN_REQUIRED', 'leftovers.error_telegram_chat_admin_required'],
    ['USER_BANNED_IN_CHANNEL', 'leftovers.error_telegram_user_banned'],
    ['CHANNEL_PRIVATE', 'leftovers.error_telegram_channel_private'],
    ['FILE_PARTS_INVALID', 'leftovers.error_telegram_file_parts_invalid'],
    ['MEDIA_EMPTY', 'leftovers.error_telegram_media_empty'],
]);

function telegramPresentation(code: string): ErrorPresentation {
    const known = TELEGRAM_CODES.get(code);
    const forbidden = code.includes('FORBIDDEN') || code.includes('ADMIN_REQUIRED') || code.includes('BANNED');
    return {
        kind: forbidden ? 'permission' : 'unavailable',
        title: forbidden ? t('leftovers.error_telegram_forbidden_title') : t('leftovers.error_telegram_refused_title'),
        message: known
            ? t(known)
            : t('leftovers.error_telegram_answered', { code: code.toLowerCase().replace(/_/g, ' ') }),
        // Only a message that says the file is gone is not worth another try.
        retryable: !forbidden && code !== 'MESSAGE_ID_INVALID',
    };
}

function classifyMessage(raw: string, source: AppErrorSource): ErrorPresentation {
    const message = singleLine(raw, MAX_USER_MESSAGE_LENGTH);
    const lower = message.toLowerCase();

    if (source === 'render') {
        return {
            kind: 'render',
            title: t('leftovers.error_render_title'),
            message: t('leftovers.error_render_message'),
            retryable: true,
        };
    }
    if (lower.includes('move would create cycle') || lower.includes('own subfolder')) {
        return {
            kind: 'validation',
            title: t('leftovers.error_move_cycle_title'),
            message: t('leftovers.error_move_cycle_message'),
            retryable: false,
        };
    }
    if (lower.includes('only the uploader can')) {
        return { kind: 'permission', title: t('leftovers.error_uploader_only_title'), message, retryable: false };
    }
    if (lower.includes('file is already in this folder') || lower.includes('folder is already here')) {
        return operation_presentations().already_exists;
    }
    if (lower.includes('invalid target') || lower.includes('target folder not found')) {
        return {
            kind: 'validation',
            title: t('leftovers.error_destination_title'),
            message: t('leftovers.error_destination_message'),
            retryable: false,
        };
    }
    if (lower.includes('not found')) return operation_presentations().not_found;
    if (lower.includes('encryption password required')) {
        return operation_presentations().encryption_password_required;
    }
    if (lower.includes('phone_number_invalid') || lower.includes('invalid phone')) {
        return {
            kind: 'validation',
            title: t('leftovers.error_phone_title'),
            message: t('leftovers.error_phone_message'),
            retryable: false,
        };
    }
    if (lower.includes('phone_code_invalid') || lower.includes('invalid code')) {
        return {
            kind: 'authentication',
            title: t('leftovers.error_code_title'),
            message: t('leftovers.error_code_message'),
            retryable: false,
        };
    }
    if (lower.includes('password_hash_invalid') || lower.includes('invalid password')) {
        return {
            kind: 'authentication',
            title: t('leftovers.error_two_step_title'),
            message: t('leftovers.error_two_step_message'),
            retryable: false,
        };
    }
    if (lower.includes('flood_wait') || lower.includes('flood wait') || lower.includes('too many requests')) {
        return {
            kind: 'network',
            title: t('leftovers.error_flood_title'),
            message: t('leftovers.error_flood_message'),
            retryable: true,
        };
    }
    if (lower.includes('auth_key_unregistered') || lower.includes('session expired')) {
        return {
            kind: 'authentication',
            title: t('leftovers.error_session_title'),
            message: t('leftovers.error_session_message'),
            retryable: false,
        };
    }
    if (lower.includes('permission denied') || lower.includes('not authorized')) {
        return operation_presentations().permission_denied;
    }
    if (lower.includes('no space left') || lower.includes('disk full')) {
        return operation_presentations().insufficient_storage;
    }
    if (lower.includes('deadline exceeded') || lower.includes('timed out') || lower.includes('timeout')) {
        return operation_presentations().deadline_exceeded;
    }
    // Telegram answers a refused RPC with a bare code ("rpc error code 400:
    // FILE_REFERENCE_EXPIRED"), and the backend passes it up as it is. The
    // codes that a user can do something about get their own sentence; the
    // rest are named rather than printed as a stack of capitals.
    const rpc = /\b([A-Z][A-Z0-9]*(?:_[A-Z0-9]+)+)\b/.exec(message);
    if (rpc && (lower.includes('rpc error') || TELEGRAM_CODES.has(rpc[1]))) {
        return telegramPresentation(rpc[1]);
    }
    if (lower.includes('database is locked') || lower.includes('sqlite')) {
        return {
            kind: 'unavailable',
            title: t('leftovers.error_catalog_title'),
            message: t('leftovers.error_catalog_message'),
            retryable: true,
        };
    }
    if (lower.includes('connection dead') || lower.includes('engine forcibly closed') || lower.includes('engine was closed')) {
        return operation_presentations().network_unavailable;
    }
    if (lower.includes('context canceled') || lower.includes('forcibly closed')) {
        return {
            kind: 'unexpected',
            title: t('leftovers.error_interrupted_title'),
            message: t('leftovers.error_interrupted_message'),
            retryable: true,
        };
    }
    if (lower.includes('tg client') || lower.includes('telegram') || lower.includes('network')) {
        return operation_presentations().network_unavailable;
    }
    if (source === 'startup') {
        return {
            kind: 'unavailable',
            title: t('leftovers.error_startup_title'),
            message: t('leftovers.error_startup_message'),
            retryable: true,
        };
    }
    if (source === 'runtime') {
        return {
            kind: 'unavailable',
            title: t('leftovers.error_runtime_title'),
            message: t('leftovers.error_runtime_message'),
            retryable: true,
        };
    }
    return {
        kind: 'unexpected',
        title: t('leftovers.error_generic_title'),
        message: message || t('leftovers.error_generic_message'),
        retryable: true,
    };
}

export function isAppError(value: unknown): value is AppError {
    const kind = property(value, 'kind');
    const source = property(value, 'source');
    return typeof kind === 'string'
        && Object.prototype.hasOwnProperty.call(APP_ERROR_KINDS, kind)
        && typeof source === 'string'
        && Object.prototype.hasOwnProperty.call(APP_ERROR_SOURCES, source)
        && typeof property(value, 'title') === 'string'
        && typeof property(value, 'message') === 'string'
        && typeof property(value, 'retryable') === 'boolean';
}

/** Normalize any thrown value without coercing arbitrary objects into user copy. */
export function toAppError(error: unknown, options: AppErrorOptions = {}): AppError {
    const source = options.source ?? 'application';
    if (isAppError(error) && error.source === source) return error;

    const chain = causeChain(error);
    const raw = rawMessage(chain);
    const code = knownOperationCode(chain);
    let presentation = classifyMessage(raw, source);

    if (source !== 'render' && code) {
        const operationPresentation = operation_presentations()[code];
        presentation = operationPresentation.message
            ? operationPresentation
            : {
                ...operationPresentation,
                message: presentation.message,
                kind: presentation.kind,
                title: presentation.title,
                retryable: presentation.retryable,
            };
    }

    return Object.freeze({
        ...presentation,
        source,
        ...(code ? { code } : {}),
        details: safeDetails(chain, raw),
        cause: error,
    }) as AppError;
}

/** Converts backend failures into concise, non-sensitive copy safe for inline UI. */
export function humanizeBackendError(error: unknown): string {
    return toAppError(error, { source: 'backend' }).message;
}

// Direct Wails methods reject with a wrapped Go error instead of the operation
// envelope used by mutations and downloads. Keep that bridge-specific fallback
// in one place so media surfaces never branch on raw error text themselves.
export function isEncryptionPasswordRequired(error: unknown): boolean {
    const normalized = toAppError(error, { source: 'backend' });
    return normalized.code === 'encryption_password_required'
        || (normalized.kind === 'authentication' && normalized.title === t('leftovers.error_encryption_password_required_title'));
}

/** Formats only already-redacted fields; the original cause is deliberately excluded. */
export function formatAppErrorDiagnostic(error: AppError): string {
    const lines = [
        'Source: ' + error.source,
        'Kind: ' + error.kind,
        ...(error.details.code ? ['Code: ' + error.details.code] : []),
        'Type: ' + error.details.name,
        ...(error.details.message ? ['Message: ' + error.details.message] : []),
        ...(error.details.stack ? ['Stack:\n' + error.details.stack] : []),
    ];
    return lines.join('\n');
}
