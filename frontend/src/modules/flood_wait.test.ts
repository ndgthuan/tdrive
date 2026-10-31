import { describe, expect, it } from 'vitest';
import { flood_wait_seconds } from './flood_wait';

describe('flood_wait_seconds', () => {
    it('reads the wait from the backend flood-wait error text', () => {
        expect(flood_wait_seconds('personaldrive: list owned channels: tgclient: flood wait: 23s')).toBe(23);
    });

    it('reads the wait from the raw Telegram RPC error code', () => {
        expect(flood_wait_seconds('rpc error code 420: FLOOD_WAIT_30')).toBe(30);
    });

    it('returns null for errors that are not flood waits', () => {
        expect(flood_wait_seconds('read config: permission denied')).toBeNull();
        expect(flood_wait_seconds('Telegram is unavailable.')).toBeNull();
    });

    it('returns null when a flood wait carries no duration', () => {
        expect(flood_wait_seconds('tgclient: flood wait')).toBeNull();
    });
});
