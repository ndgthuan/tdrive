// Copyright (c) 2026 Nguyễn Dương Gia Thuận

import { describe, expect, it, vi } from 'vitest';
import { answer_host_request, create_line_forwarder, native_engine_present, type native_engine_port } from './native_engine';
import type { media_host_request } from '../../api/media_download';

function fake_engine(overrides: Partial<native_engine_port> = {}): native_engine_port {
    return {
        status: vi.fn(async () => '{"installed":true,"version":"2026.10.12","ffmpeg":true}'),
        install: vi.fn(async () => ''),
        update: vi.fn(async () => 'Updated to 2026.10.12'),
        run: vi.fn(async () => ''),
        cancel: vi.fn((_request_id: string) => undefined),
        ...overrides,
    };
}

describe('answer_host_request', () => {
    it('replies to a status request with the engine answer', async () => {
        const engine = fake_engine();
        const reply = vi.fn(async () => undefined);
        const request: media_host_request = { request_id: 'r1', op: 'status' };

        await answer_host_request(request, engine, reply);

        expect(engine.status).toHaveBeenCalledOnce();
        expect(reply).toHaveBeenCalledWith('r1', '{"installed":true,"version":"2026.10.12","ffmpeg":true}', '');
    });

    it('passes the argv to run and replies once the engine finishes', async () => {
        const engine = fake_engine();
        const reply = vi.fn(async () => undefined);
        const args = ['--no-update', '--', 'https://example.com/v'];

        await answer_host_request({ request_id: 'r2', op: 'run', args }, engine, reply);

        expect(engine.run).toHaveBeenCalledWith('r2', args);
        expect(reply).toHaveBeenCalledWith('r2', '', '');
    });

    it('reports an engine failure as the reply failure text', async () => {
        const engine = fake_engine({ update: vi.fn(async () => { throw new Error('no network'); }) });
        const reply = vi.fn(async () => undefined);

        await answer_host_request({ request_id: 'r3', op: 'update' }, engine, reply);

        expect(reply).toHaveBeenCalledWith('r3', '', 'no network');
    });

    it('falls back to a generic message when the failure has no text', async () => {
        const engine = fake_engine({ install: vi.fn(async () => { throw 'boom'; }) });
        const reply = vi.fn(async () => undefined);

        await answer_host_request({ request_id: 'r4', op: 'install' }, engine, reply);

        expect(reply).toHaveBeenCalledWith('r4', '', 'The video downloader failed. Try again.');
    });

    it('passes a cancel to the engine and sends no reply', async () => {
        const engine = fake_engine();
        const reply = vi.fn(async () => undefined);

        await answer_host_request({ request_id: 'r5', op: 'cancel' }, engine, reply);

        expect(engine.cancel).toHaveBeenCalledWith('r5');
        expect(reply).not.toHaveBeenCalled();
    });

    it('answers an unknown operation with a failure rather than hanging', async () => {
        const engine = fake_engine();
        const reply = vi.fn(async () => undefined);

        await answer_host_request({ request_id: 'r6', op: 'explode' as media_host_request['op'] }, engine, reply);

        expect(reply).toHaveBeenCalledWith('r6', '', expect.any(String));
    });
});

describe('native_engine_present', () => {
    it('is false on a desktop test host with no native bridge', () => {
        expect(native_engine_present()).toBe(false);
    });
});

it('waits for metadata delivery before allowing request completion', async () => {
    let finish!: () => void;
    const report = vi.fn(() => new Promise<void>((resolve) => { finish = resolve; }));
    const output = create_line_forwarder(report);
    output.line('probe', '{"title":"Video","formats":[]}');
    const complete = vi.fn();
    const drained = output.drain('probe').then(complete);
    await Promise.resolve();
    expect(report).toHaveBeenCalledWith('probe', '{"title":"Video","formats":[]}');
    expect(complete).not.toHaveBeenCalled();
    finish();
    await drained;
    expect(complete).toHaveBeenCalledOnce();
});
