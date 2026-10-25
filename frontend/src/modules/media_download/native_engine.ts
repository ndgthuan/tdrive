// Copyright (c) 2026 Nguyễn Dương Gia Thuận

// Phone side of the link downloader. Go emits one request per engine call on
// media_dl:host; this module answers it through the native bridge and passes
// the engine's output lines back. Android exposes the calls as wails.mediaDl*
// methods, iOS as window.tdriveMediaDl. Neither side is reachable from Go
// directly, which is why the web view relays.

import { callBridge, hasBridgeMethod } from '../android_bridge';
import type { RuntimeUnsubscribe } from '../../api/gateway';
import {
    report_media_host_line,
    reply_media_host,
    subscribe_media_host,
    type media_host_request,
} from '../../api/media_download';

/** Shown when no native engine is present, so the sheet can say why nothing starts. */
export const ENGINE_UNAVAILABLE_MESSAGE = 'The video downloader is not available on this device.';
const ENGINE_FAILED_MESSAGE = 'The video downloader failed. Try again.';

/** The calls the iOS bridge exposes. Each may answer synchronously or with a promise. */
interface ios_engine_bridge {
    status?: () => unknown;
    install?: (request_id: string) => unknown;
    update?: () => unknown;
    run?: (request_id: string, args_json: string) => unknown;
    cancel?: (request_id: string) => unknown;
}

declare global {
    interface Window {
        tdriveMediaDl?: ios_engine_bridge;
        /** Set by the relay; the native side calls it with each output line. */
        tdriveMediaDlLine?: (request_id: string, line: string) => void;
    }
}

/** One engine, as the relay calls it. Tests supply a fake. */
export interface native_engine_port {
    status(): Promise<string>;
    install(request_id: string): Promise<string>;
    update(): Promise<string>;
    run(request_id: string, args: string[]): Promise<string>;
    cancel(request_id: string): void;
}

function ios_engine(): ios_engine_bridge | null {
    if (typeof window === 'undefined') return null;
    return window.tdriveMediaDl ?? null;
}

/** Whether this device has a native downloader engine at all. */
export function native_engine_present(): boolean {
    return Boolean(ios_engine()?.status) || hasBridgeMethod('mediaDlStatus');
}

async function text_of(value: unknown): Promise<string> {
    const resolved = await Promise.resolve(value);
    return typeof resolved === 'string' ? resolved : '';
}

/** The engine for this device, or null when it has none. */
export function platform_engine(): native_engine_port | null {
    const ios = ios_engine();
    if (ios?.status) {
        return {
            status: async () => text_of(ios.status?.()),
            install: async (request_id) => text_of(ios.install?.(request_id)),
            update: async () => text_of(ios.update?.()),
            run: async (request_id, args) => text_of(ios.run?.(request_id, JSON.stringify(args))),
            cancel: (request_id) => {
                void Promise.resolve(ios.cancel?.(request_id)).catch(() => undefined);
            },
        };
    }
    if (!hasBridgeMethod('mediaDlStatus')) return null;
    return {
        status: () => callBridge('mediaDlStatus', [], ENGINE_UNAVAILABLE_MESSAGE),
        install: (request_id) => callBridge('mediaDlInstall', [request_id], ENGINE_UNAVAILABLE_MESSAGE),
        update: () => callBridge('mediaDlUpdate', [], ENGINE_UNAVAILABLE_MESSAGE),
        run: (request_id, args) => callBridge('mediaDlRun', [JSON.stringify({ id: request_id, args })], ENGINE_UNAVAILABLE_MESSAGE),
        cancel: (request_id) => {
            void callBridge('mediaDlCancel', [request_id], '').catch(() => undefined);
        },
    };
}

function failure_text(cause: unknown): string {
    return cause instanceof Error && cause.message ? cause.message : ENGINE_FAILED_MESSAGE;
}

/**
 * Answers one request from Go. A cancel is passed to the engine and gets no
 * reply. Any other request gets exactly one reply: the result, or the failure.
 */
export async function answer_host_request(
    request: media_host_request,
    engine: native_engine_port,
    reply: (request_id: string, result: string, failure: string) => Promise<void> = reply_media_host,
): Promise<void> {
    if (request.op === 'cancel') {
        engine.cancel(request.request_id);
        return;
    }
    let result: string;
    try {
        result = await run_operation(request, engine);
    } catch (cause) {
        await reply(request.request_id, '', failure_text(cause));
        return;
    }
    await reply(request.request_id, result, '');
}

async function run_operation(request: media_host_request, engine: native_engine_port): Promise<string> {
    switch (request.op) {
        case 'status':
            return engine.status();
        case 'install':
            return engine.install(request.request_id);
        case 'update':
            return engine.update();
        case 'run':
            return engine.run(request.request_id, request.args ?? []);
        default:
            throw new Error(ENGINE_FAILED_MESSAGE);
    }
}

let relay_stop: RuntimeUnsubscribe | null = null;

// Native output and completion cross separate asynchronous Go calls. Drain
// output before resolving the request, otherwise the broker drops the final
// metadata/error line after it has removed the completed request.
export function create_line_forwarder(report = report_media_host_line) {
    const pending = new Map<string, Promise<void>>();
    return {
        line(id: string, line: string): void {
            const next = (pending.get(id) ?? Promise.resolve()).then(() => report(id, line));
            pending.set(id, next);
            void next.catch(() => undefined);
        },
        async drain(id: string): Promise<void> {
            try { await pending.get(id); } finally { pending.delete(id); }
        },
    };
}

/**
 * Starts answering engine requests and forwarding output lines. It does nothing
 * on a device without a native engine. Calling it again returns the same stop.
 */
export function start_host_relay(): RuntimeUnsubscribe {
    if (relay_stop) return relay_stop;
    const engine = platform_engine();
    if (!engine || typeof window === 'undefined') return () => undefined;
    const output = create_line_forwarder();
    window.tdriveMediaDlLine = (request_id, line) => {
        output.line(request_id, line);
    };
    const reply = async (id: string, result: string, failure: string) => {
        try { await output.drain(id); }
        catch { failure ||= ENGINE_FAILED_MESSAGE; }
        await reply_media_host(id, result, failure);
    };
    const unsubscribe = subscribe_media_host((request) => {
        void answer_host_request(request, engine, reply).catch(() => undefined);
    });
    relay_stop = () => {
        unsubscribe();
        if (typeof window !== 'undefined') delete window.tdriveMediaDlLine;
        relay_stop = null;
    };
    return relay_stop;
}
