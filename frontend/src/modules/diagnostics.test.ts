import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const backend = vi.hoisted(() => ({ recentBackendLog: vi.fn() }));
const updates = vi.hoisted(() => ({ getAppVersion: vi.fn() }));
vi.mock('../api/session', () => ({ recentBackendLog: backend.recentBackendLog }));
vi.mock('../api/updates', () => ({ getAppVersion: updates.getAppVersion }));
vi.mock('../i18n/i18n', () => ({ t: (key: string) => key }));
vi.mock('./notifications', () => ({ notify: vi.fn() }));

import { build_backend_log_section, build_diagnostics_report, copy_diagnostics, record_diagnostic } from './diagnostics';

const TITLE = '----- Backend log (last 300 lines) -----';

beforeEach(() => {
    updates.getAppVersion.mockResolvedValue({ version: '3.1.4', os: 'windows', arch: 'amd64', devBuild: false, storeManaged: false });
});

afterEach(() => {
    backend.recentBackendLog.mockReset();
    updates.getAppVersion.mockReset();
    vi.unstubAllGlobals();
});

describe('diagnostics report', () => {
    it('keeps the existing content and appends the backend log section', async () => {
        record_diagnostic('error', 'frontend boom');
        backend.recentBackendLog.mockResolvedValue('2026-10-31T10:00:00.000Z INFO tgclient: dialog page rpc=messages.getDialogs');

        const report = await build_diagnostics_report();

        expect(report.startsWith('TDrive diagnostics\n')).toBe(true);
        expect(report).toContain('App version: 3.1.4 (windows/amd64)');
        expect(report).toContain('frontend boom');
        expect(report.indexOf('frontend boom')).toBeLessThan(report.indexOf(TITLE));
        expect(report).toContain('rpc=messages.getDialogs');
        expect(backend.recentBackendLog).toHaveBeenCalledWith(300);
    });

    it('still produces the report and names the reason when the backend call fails', async () => {
        record_diagnostic('rejection', 'unhandled thing');
        backend.recentBackendLog.mockRejectedValue(new Error('bridge not ready'));

        const report = await build_diagnostics_report();

        expect(report).toContain('TDrive diagnostics');
        expect(report).toContain('unhandled thing');
        expect(report).toContain(TITLE);
        expect(report).toContain('(backend log unavailable: bridge not ready)');
    });

    it('says when no backend lines are recorded', async () => {
        backend.recentBackendLog.mockResolvedValue('   ');

        const section = await build_backend_log_section();

        expect(section).toEqual([TITLE, '(no backend log lines recorded)']);
    });

    it('keeps the newest lines within the size cap', async () => {
        const lines = Array.from({ length: 4000 }, (_, index) => `line ${index} ${'x'.repeat(40)}`);
        backend.recentBackendLog.mockResolvedValue(lines.join('\n'));

        const [title, body] = await build_backend_log_section();

        expect(title).toBe(TITLE);
        expect(body.length).toBeLessThanOrEqual(64 * 1024 + '(older lines omitted)\n'.length);
        expect(body.startsWith('(older lines omitted)\n')).toBe(true);
        expect(body.endsWith(lines[lines.length - 1])).toBe(true);
        expect(body).not.toContain(`${lines[0]}\n`);
    });

    it('copies the report even when the backend log is unavailable', async () => {
        backend.recentBackendLog.mockRejectedValue(new Error('no backend'));
        const writeText = vi.fn().mockResolvedValue(undefined);
        vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } });

        const ok = await copy_diagnostics();

        expect(ok).toBe(true);
        expect(writeText).toHaveBeenCalledOnce();
        expect(String(writeText.mock.calls[0][0])).toContain('(backend log unavailable: no backend)');
    });
});
