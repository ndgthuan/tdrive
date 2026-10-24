import { describe, expect, it } from 'vitest';
import { transferView } from './auto_backup_model';

const running = { enabled: true, paused: false, done: 3, total: 10, currentName: 'IMG_1.HEIC', currentBytes: 1024, folderName: 'iPhone' };

describe('automatic backup transfer row', () => {
    it('shows a row only while the run is on, unpaused and has items', () => {
        expect(transferView(null).active).toBe(false);
        expect(transferView({ ...running, enabled: false }).active).toBe(false);
        expect(transferView({ ...running, paused: true }).active).toBe(false);
        expect(transferView({ ...running, total: 0, done: 0 }).active).toBe(false);
        expect(transferView(running)).toMatchObject({ active: true, complete: false, done: 3, total: 10, percent: 30 });
    });

    it('completes once every item is done and never overshoots', () => {
        expect(transferView({ ...running, done: 10 })).toMatchObject({ active: false, complete: true, percent: 100 });
        expect(transferView({ ...running, done: 12 })).toMatchObject({ complete: true, done: 10, total: 10 });
    });

    it('treats a missing enabled flag as on, for older callers', () => {
        const legacy: Parameters<typeof transferView>[0] = { ...running, enabled: undefined };
        expect(transferView(legacy).active).toBe(true);
    });
});
