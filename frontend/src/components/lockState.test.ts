import { describe, expect, it } from 'vitest';
import { NO_LOCK, parseLockState } from './lockState';

describe('parseLockState', () => {
    it('reads a manual lock', () => {
        expect(parseLockState('{"kind":"manual","end_time":"2026-11-20T10:00:00Z"}'))
            .toEqual({ kind: 'manual', endTime: '2026-11-20T10:00:00Z' });
    });

    it('reads a schedule window', () => {
        expect(parseLockState('{"kind":"schedule","end_time":"2026-11-21T06:00:00+05:30"}'))
            .toEqual({ kind: 'schedule', endTime: '2026-11-21T06:00:00+05:30' });
    });

    it.each([
        '{"kind":"none","end_time":""}',
        '{"kind":"manual","end_time":""}',
        '{"kind":"manual","end_time":"soon"}',
        '{"kind":"other","end_time":"2026-11-20T10:00:00Z"}',
        'null',
        '',
        '{bad',
    ])('treats %s as no lock', (json) => {
        expect(parseLockState(json)).toEqual(NO_LOCK);
    });
});
