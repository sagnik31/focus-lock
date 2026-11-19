import { describe, expect, it } from 'vitest';
import { formatDuration } from './duration';

describe('formatDuration', () => {
    it.each([
        [0, '0m'],
        [-120, '0m'],
        [29, '0m'],
        [30, '1m'],
        [45 * 60, '45m'],
        [60 * 60, '1h'],
        [2 * 3600 + 15 * 60, '2h 15m'],
        [24 * 3600, '1d'],
        [3 * 86400 + 4 * 3600 + 10 * 60, '3d 4h 10m'],
        [86400 + 5 * 60, '1d 5m'],
    ])('formats %i seconds as %s', (secs, want) => {
        expect(formatDuration(secs)).toBe(want);
    });
});
