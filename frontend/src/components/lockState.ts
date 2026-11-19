// The lock in force, as computed by the backend's GetLockState (the same
// schedule logic the enforcer uses, overnight windows included).

export type LockKind = 'manual' | 'schedule' | 'none';

export interface LockState {
    kind: LockKind;
    // ISO 8601 end of enforcement: whichever of the manual lock and the
    // active schedule windows ends later. Empty when kind is 'none'.
    endTime: string;
}

export const NO_LOCK: LockState = { kind: 'none', endTime: '' };

// parseLockState reads GetLockState's JSON. Anything malformed is treated as
// no lock rather than throwing; the next poll corrects it.
export function parseLockState(json: string): LockState {
    try {
        const raw = JSON.parse(json);
        const kind = raw?.kind;
        const endTime = typeof raw?.end_time === 'string' ? raw.end_time : '';
        if ((kind === 'manual' || kind === 'schedule') && endTime && !isNaN(Date.parse(endTime))) {
            return { kind, endTime };
        }
    } catch {
        /* fall through */
    }
    return NO_LOCK;
}
