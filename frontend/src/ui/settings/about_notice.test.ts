// Copyright (c) 2026 Nguyễn Dương Gia Thuận

import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import * as about from './about_notice';

const repo_root = fileURLToPath(new URL('../../../../', import.meta.url));

function go_const(source: string, name: string): string {
    const match = new RegExp(name + '\\s*=\\s*"([^"]*)"').exec(source);
    if (!match) throw new Error(`missing Go constant ${name}`);
    return match[1];
}

describe('about notice', () => {
    it('bundles the same text as NOTICE.md at the repository root', () => {
        const on_disk = readFileSync(`${repo_root}NOTICE.md`, 'utf8');
        expect(about.notice_text.replace(/\r\n/g, '\n')).toBe(on_disk.replace(/\r\n/g, '\n'));
    });

    it('states the upstream attribution line', () => {
        expect(about.notice_text).toContain('Based on TDrive by Jeetraj');
        expect(about.notice_text).toContain('https://github.com/jeetrex17/TDrive');
    });

    it('keeps the author and repository in sync with the Go buildinfo package', () => {
        const go_source = readFileSync(`${repo_root}internal/buildinfo/author.go`, 'utf8');
        expect(about.author).toBe(go_const(go_source, 'Author'));
        expect(about.repository_url).toBe(go_const(go_source, 'Repository'));
        expect(about.upstream_url).toBe(go_const(go_source, 'UpstreamURL'));
    });
});
