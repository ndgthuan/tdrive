import { describe, expect, it } from 'vitest';
import {
    clamp_percent,
    describe_format,
    format_clock,
    format_eta,
    format_speed,
    human_size,
    is_active_stage,
    is_terminal_stage,
    is_valid_link,
    join_parts,
    link_host,
    setup_stage_key,
    stage_key,
} from './downloader_format';
import {
    apply_progress_rows,
    attach_request,
    is_stage_regression,
    load_download_rows,
    type download_row,
} from './downloader_state';
import { get } from 'svelte/store';
import { download_rows } from './downloader_state';

function row(overrides: Partial<download_row> = {}): download_row {
    return {
        job_id: 'job1',
        url: 'https://example.com/v',
        choice: 'best',
        title: '',
        stage: 'downloading',
        percent: 0,
        speed: 0,
        eta: -1,
        error: '',
        file_name: '',
        folder_id: '',
        drive_id: 0,
        folder_name: '',
        started_at: 1,
        ...overrides,
    };
}

describe('human_size and speed', () => {
    it('uses decimal units with one decimal below ten', () => {
        expect(human_size(0)).toBe('');
        expect(human_size(-5)).toBe('');
        expect(human_size(512)).toBe('512 B');
        expect(human_size(4_200_000)).toBe('4.2 MB');
        expect(human_size(52_300_000)).toBe('52 MB');
        expect(human_size(1_300_000_000)).toBe('1.3 GB');
    });

    it('formats speed per second and leaves unknown speeds empty', () => {
        expect(format_speed(2_300_000)).toBe('2.3 MB/s');
        expect(format_speed(0)).toBe('');
    });
});

describe('clocks and percentages', () => {
    it('reads remaining time as a clock and treats -1 as unknown', () => {
        expect(format_eta(42)).toBe('0:42');
        expect(format_eta(245)).toBe('4:05');
        expect(format_eta(3729)).toBe('1:02:09');
        expect(format_eta(-1)).toBe('');
    });

    it('reads video length as a clock and falls back to zero', () => {
        expect(format_clock(125.9)).toBe('2:05');
        expect(format_clock(Number.NaN)).toBe('0:00');
    });

    it('clamps percentages for the bar', () => {
        expect(clamp_percent(-1)).toBe(0);
        expect(clamp_percent(42.4)).toBe(42);
        expect(clamp_percent(140)).toBe(100);
        expect(clamp_percent(Number.NaN)).toBe(0);
    });
});

describe('link validation', () => {
    it('accepts absolute http and https links', () => {
        expect(is_valid_link('https://www.example.com/watch?v=1')).toBe(true);
        expect(is_valid_link('  http://example.org/a  ')).toBe(true);
    });

    it('rejects other schemes, hostless text and embedded credentials', () => {
        expect(is_valid_link('')).toBe(false);
        expect(is_valid_link('   ')).toBe(false);
        expect(is_valid_link('ftp://example.com/x')).toBe(false);
        expect(is_valid_link('javascript:alert(1)')).toBe(false);
        expect(is_valid_link('https://')).toBe(false);
        expect(is_valid_link('not a url')).toBe(false);
        expect(is_valid_link('https://user:pw@example.com/')).toBe(false);
        expect(is_valid_link(`https://example.com/${'a'.repeat(2048)}`)).toBe(false);
    });

    it('returns the host for a compact label, without a leading www', () => {
        expect(link_host('https://www.example.com/x')).toBe('example.com');
        expect(link_host('nope')).toBe('');
    });
});

describe('format labels', () => {
    it('describes combined, video-only and audio-only formats', () => {
        expect(
            describe_format({ height: 1080, ext: 'mp4', approx_size: 52_300_000, has_audio: false, has_video: true, note: '' }),
        ).toEqual({ kind: 'video_only', resolution: '1080p', ext: 'MP4', size: '52 MB' });
        expect(describe_format({ height: 360, ext: 'mp4', approx_size: 0, has_audio: true, has_video: true, note: '' }).kind).toBe(
            'combined',
        );
        expect(describe_format({ height: 0, ext: 'm4a', approx_size: 4_000_000, has_audio: true, has_video: false, note: 'medium' })).toEqual({
            kind: 'audio_only',
            resolution: 'medium',
            ext: 'M4A',
            size: '4.0 MB',
        });
    });

    it('joins only the parts that have text', () => {
        expect(join_parts(['1080p', '', undefined, 'MP4', '   ', 'video only'])).toBe('1080p · MP4 · video only');
    });
});

describe('stage helpers', () => {
    it('maps stages to translation keys and falls back to the error label', () => {
        expect(stage_key('merging')).toBe('downloader.stage_merging');
        expect(stage_key('weird')).toBe('downloader.stage_error');
        expect(setup_stage_key('verifying')).toBe('downloader.setup_verifying');
        expect(setup_stage_key('???')).toBe('downloader.setup_error');
    });

    it('separates active, terminal and idle stages', () => {
        expect(is_active_stage('uploading')).toBe(true);
        expect(is_active_stage('done')).toBe(false);
        expect(is_terminal_stage('cancelled')).toBe(true);
        expect(is_terminal_stage('merging')).toBe(false);
    });
});

describe('job rows', () => {
    it('updates an existing job and keeps the request fields', () => {
        const rows = [row({ url: 'https://example.com/v', choice: '720p' })];
        const next = apply_progress_rows(rows, {
            job_id: 'job1',
            stage: 'downloading',
            percent: 40,
            speed: 1000,
            eta: 9,
            title: 'Clip',
        });
        expect(next[0]).toMatchObject({ url: 'https://example.com/v', choice: '720p', percent: 40, title: 'Clip', eta: 9 });
        expect(rows[0].percent).toBe(0);
    });

    it('inserts an unknown job first, since its first event can arrive before Start returns', () => {
        const next = apply_progress_rows([row({ job_id: 'older' })], {
            job_id: 'new',
            stage: 'downloading',
            percent: 0,
            speed: 0,
            eta: -1,
            title: '',
        });
        expect(next.map((r) => r.job_id)).toEqual(['new', 'older']);
    });

    it('never moves a job backwards or changes a finished job', () => {
        expect(is_stage_regression('uploading', 'downloading')).toBe(true);
        expect(is_stage_regression('downloading', 'merging')).toBe(false);
        expect(is_stage_regression('done', 'uploading')).toBe(true);

        const uploading = [row({ stage: 'uploading' })];
        expect(apply_progress_rows(uploading, { job_id: 'job1', stage: 'downloading', percent: 5, speed: 0, eta: -1, title: '' })).toBe(uploading);

        const finished = [row({ stage: 'done', file_name: 'a.mp4', folder_id: 'f1' })];
        const after = apply_progress_rows(finished, { job_id: 'job1', stage: 'error', percent: 0, speed: 0, eta: -1, title: '', error: 'x' });
        expect(after[0].stage).toBe('done');
    });

    it('keeps the file name and folder once a finished event supplied them', () => {
        const rows = [row({ stage: 'uploading' })];
        const done = apply_progress_rows(rows, {
            job_id: 'job1',
            stage: 'done',
            percent: 100,
            speed: 0,
            eta: -1,
            title: '',
            file_name: 'clip.mp4',
            folder_id: 'folder-9',
        });
        expect(done[0]).toMatchObject({ stage: 'done', file_name: 'clip.mp4', folder_id: 'folder-9' });
    });

    it('attaches the url and quality the dialog sent to the started job', () => {
        const target = { drive_id: 42, folder_id: 'folder-9', folder_name: 'Clips' };
        const rows = attach_request([], 'job7', 'https://example.com/a', 'audio_mp3', target);
        expect(rows[0]).toMatchObject({
            job_id: 'job7',
            url: 'https://example.com/a',
            choice: 'audio_mp3',
            stage: 'downloading',
            drive_id: 42,
            folder_id: 'folder-9',
            folder_name: 'Clips',
        });
        const updated = attach_request(rows, 'job7', 'https://example.com/b', 'best', target);
        expect(updated).toHaveLength(1);
        expect(updated[0].url).toBe('https://example.com/b');
    });

    it('loads a backend snapshot into the store in the same order', () => {
        load_download_rows([
            { job_id: 'b', url: 'https://x.test/1', choice: 'best', title: 'B', stage: 'done', percent: 100, speed: 0, eta: -1, error: '', file_name: 'b.mp4', folder_id: 'f', started_at: 2 },
            { job_id: 'a', url: 'https://x.test/2', choice: 'best', title: 'A', stage: 'error', percent: 0, speed: 0, eta: -1, error: 'boom', file_name: '', folder_id: '', started_at: 1 },
        ] as never);
        expect(get(download_rows).map((r) => r.job_id)).toEqual(['b', 'a']);
        expect(get(download_rows)[1].error).toBe('boom');
    });
});
