import { describe, expect, it } from 'vitest';
import { normalizeAutoBackupState } from '../../api/auto_backup';
import {
    folderNameProblemKey, formatBytes, normalizedFolderName, permissionIsLimited,
    permissionNeedsSettings, phaseMessageKey, progressFraction, progressPercent,
} from './auto_backup_model';

describe('folderNameProblemKey', () => {
    it('accepts an empty name, which restores the device default', () => {
        expect(folderNameProblemKey('')).toBeNull();
        expect(folderNameProblemKey('   ')).toBeNull();
    });

    it('accepts ordinary names, including Vietnamese and spaces', () => {
        expect(folderNameProblemKey('Work phone')).toBeNull();
        expect(folderNameProblemKey('Điện thoại của tôi')).toBeNull();
        expect(folderNameProblemKey('Pixel 8 (2026)')).toBeNull();
    });

    it('refuses characters a folder may not hold', () => {
        for (const value of ['a/b', 'a\\b', 'a:b', 'a*b', 'a?b', 'a"b', 'a<b', 'a>b', 'a|b']) {
            expect(folderNameProblemKey(value)).toBe('folder_name_invalid');
        }
    });

    it('refuses control characters and dot-only names', () => {
        expect(folderNameProblemKey('tab\there')).toBe('folder_name_invalid');
        expect(folderNameProblemKey('.')).toBe('folder_name_invalid');
        expect(folderNameProblemKey('..')).toBe('folder_name_invalid');
        expect(folderNameProblemKey('...')).toBe('folder_name_invalid');
    });

    it('measures length in UTF-8 bytes, not characters', () => {
        expect(folderNameProblemKey('a'.repeat(200))).toBeNull();
        expect(folderNameProblemKey('a'.repeat(201))).toBe('folder_name_invalid');
        // 67 characters of 3 bytes each is 201 bytes: too long although it reads short.
        expect(folderNameProblemKey('ệ'.repeat(67))).toBe('folder_name_invalid');
    });
});

describe('normalizedFolderName', () => {
    it('trims surrounding space and keeps the rest', () => {
        expect(normalizedFolderName('  Work phone  ')).toBe('Work phone');
        expect(normalizedFolderName('   ')).toBe('');
    });
});

describe('phaseMessageKey', () => {
    it('maps every backend phase to a translation key', () => {
        const phases = ['off', 'needs_permission', 'paused', 'waiting_for_wifi', 'waiting_for_charging', 'running', 'up_to_date', 'failed'] as const;
        const keys = phases.map(phaseMessageKey);
        expect(new Set(keys).size).toBe(phases.length);
        for (const key of keys) expect(key.startsWith('auto_backup.phase_')).toBe(true);
    });
});

describe('permission helpers', () => {
    it('sends a denied grant to system settings and only a limited grant to the limited notice', () => {
        expect(permissionNeedsSettings('denied')).toBe(true);
        expect(permissionNeedsSettings('granted')).toBe(false);
        expect(permissionIsLimited('limited')).toBe(true);
        expect(permissionIsLimited('granted')).toBe(false);
    });
});

describe('progress', () => {
    it('bounds the fraction to [0, 1] and treats impossible totals as zero', () => {
        expect(progressFraction(128, 2431)).toBeCloseTo(0.0526, 3);
        expect(progressFraction(5, 0)).toBe(0);
        expect(progressFraction(-1, 10)).toBe(0);
        expect(progressFraction(20, 10)).toBe(1);
        expect(progressFraction(Number.NaN, 10)).toBe(0);
    });

    it('rounds to a whole percentage for the bar', () => {
        expect(progressPercent(128, 2431)).toBe(5);
        expect(progressPercent(1, 2)).toBe(50);
    });
});

describe('formatBytes', () => {
    it('uses binary units and one decimal for small values', () => {
        expect(formatBytes(0)).toBe('0 B');
        expect(formatBytes(900)).toBe('900 B');
        expect(formatBytes(3.2 * 1024 * 1024)).toBe('3.2 MB');
        expect(formatBytes(412 * 1024 * 1024)).toBe('412 MB');
        expect(formatBytes(Number.NaN)).toBe('0 B');
    });
});

describe('normalizeAutoBackupState', () => {
    it('reads the backend payload and applies safe defaults to missing fields', () => {
        const state = normalizeAutoBackupState({
            enabled: true,
            folder_name: 'iPhone',
            default_folder_name: 'iPhone',
            folder_name_custom: false,
            folder_id: 'abc',
            wifi_only: true,
            charging_only: false,
            permission: 'granted',
            phase: 'running',
            platform: 'ios',
            errors: ['  Waiting  ', ''],
        });
        expect(state.enabled).toBe(true);
        expect(state.folderName).toBe('iPhone');
        expect(state.phase).toBe('running');
        expect(state.platform).toBe('ios');
        expect(state.errors).toEqual(['Waiting']);
    });

    it('falls back to off and undetermined for values it does not recognise', () => {
        const state = normalizeAutoBackupState({ phase: 'exploding', permission: 'maybe', platform: 'windows' });
        expect(state.phase).toBe('off');
        expect(state.permission).toBe('undetermined');
        expect(state.platform).toBe('');
        expect(state.enabled).toBe(false);
        expect(state.errors).toEqual([]);
    });

    it('keeps Wi-Fi only on when the field is absent, as the backend defaults it', () => {
        expect(normalizeAutoBackupState({}).wifiOnly).toBe(true);
        expect(normalizeAutoBackupState({ wifi_only: false }).wifiOnly).toBe(false);
    });

    it('accepts an unexpected shape without throwing', () => {
        expect(normalizeAutoBackupState(null).phase).toBe('off');
        expect(normalizeAutoBackupState('not an object').folderName).toBe('Phone');
    });
});
