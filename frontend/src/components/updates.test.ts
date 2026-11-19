import { describe, expect, it, vi } from 'vitest';

// updates.ts imports the Wails bindings, which need the desktop runtime.
vi.mock('../../wailsjs/go/bridge/App', () => ({
    AutoCheckForUpdate: vi.fn(),
    GetUpdateState: vi.fn(),
}));
vi.mock('../../wailsjs/runtime/runtime', () => ({ EventsOn: vi.fn() }));

import { formatReleaseDate, parseUpdateState } from './updates';

describe('parseUpdateState', () => {
    it('parses the backend JSON', () => {
        const s = parseUpdateState('{"phase":"ready","latest":"2026.11.19.1","downloaded":true,"sessionActive":false}');
        expect(s).not.toBeNull();
        expect(s?.phase).toBe('ready');
        expect(s?.latest).toBe('2026.11.19.1');
        expect(s?.downloaded).toBe(true);
    });

    it('returns null for malformed JSON', () => {
        expect(parseUpdateState('')).toBeNull();
        expect(parseUpdateState('{not json')).toBeNull();
    });
});

describe('formatReleaseDate', () => {
    it('is empty for a missing or invalid date', () => {
        expect(formatReleaseDate('')).toBe('');
        expect(formatReleaseDate('not a date')).toBe('');
    });

    it('formats a valid date with its year', () => {
        expect(formatReleaseDate('2026-11-19T12:00:00Z')).toContain('2026');
    });
});
