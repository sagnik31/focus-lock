import { describe, expect, it } from 'vitest';
import { createSerialQueue } from './serialQueue';

describe('createSerialQueue', () => {
    it('does not lose concurrent read-modify-writes', async () => {
        const run = createSerialQueue();
        let stored: string[] = [];
        const add = (v: string) => run(async () => {
            const current = stored;
            await new Promise(r => setTimeout(r, 1));
            stored = [...current, v];
        });
        await Promise.all([add('a'), add('b'), add('c')]);
        expect(stored).toEqual(['a', 'b', 'c']);
    });

    it('keeps running after a failure', async () => {
        const run = createSerialQueue();
        const failed = run(() => Promise.reject(new Error('refused')));
        const next = run(async () => 1);
        await expect(failed).rejects.toThrow('refused');
        await expect(next).resolves.toBe(1);
    });
});
