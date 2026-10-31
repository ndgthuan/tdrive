import { describe, expect, it } from 'vitest';
import { flood_wait_seconds } from './flood_wait';

describe('flood_wait_seconds', () => {
    it('reads the wait from the backend flood-wait error text', () => {
        expect(flood_wait_seconds('personaldrive: list owned channels: tgclient: flood wait: 23s')).toBe(23);
    });

    it('reads the wait from the raw Telegram RPC error code', () => {
        expect(flood_wait_seconds('rpc error code 420: FLOOD_WAIT_30')).toBe(30);
    });

    it('parses Go minute durations into total seconds', () => {
        expect(flood_wait_seconds('tgclient: flood wait: 1m31s')).toBe(91);
        expect(flood_wait_seconds('tgclient: flood wait: 5m0s')).toBe(300);
    });

    it('parses Go hour durations into total seconds', () => {
        expect(flood_wait_seconds('tgclient: flood wait: 1h1m40s')).toBe(3700);
    });

    it('rounds fractional seconds up', () => {
        expect(flood_wait_seconds('tgclient: flood wait: 1.5s')).toBe(2);
        expect(flood_wait_seconds('tgclient: flood wait: 1m0.2s')).toBe(61);
    });

    it('returns null for errors that are not flood waits', () => {
        expect(flood_wait_seconds('read config: permission denied')).toBeNull();
        expect(flood_wait_seconds('Telegram is unavailable.')).toBeNull();
    });

    it('returns null when a flood wait carries no duration', () => {
        expect(flood_wait_seconds('tgclient: flood wait')).toBeNull();
    });
});
