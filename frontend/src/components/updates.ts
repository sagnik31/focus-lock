import { useEffect, useState } from 'react';
import { AutoCheckForUpdate, GetUpdateState } from "../../wailsjs/go/bridge/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";

// Mirrors backend/bridge/update.go updateState (sent as a JSON string).
export type UpdatePhase =
    | 'idle' | 'checking' | 'upToDate' | 'available'
    | 'downloading' | 'ready' | 'installing' | 'error';

export interface UpdateState {
    phase: UpdatePhase;
    current: string;
    dev: boolean;
    available: boolean;
    latest: string;
    notes: string;
    publishedAt: string;
    percent: number;
    downloaded: boolean;
    error: string;
    lastChecked: string;
    // Backend's view: a manual lock or schedule window is active, so Install
    // is refused (the backend re-checks; this only greys the button out).
    sessionActive: boolean;
}

export const RELEASES_URL = "https://github.com/sagnik31/focus-lock/releases/latest";

export function parseUpdateState(raw: string): UpdateState | null {
    try {
        return JSON.parse(raw) as UpdateState;
    } catch {
        return null;
    }
}

// useUpdateState owns the update state for the whole app: it loads it, keeps
// it in sync with the backend's "update:state" events, and runs the quiet
// start-up check (the backend limits that to once per 24h and skips dev builds).
export function useUpdateState(): [UpdateState | null, (raw: string) => void] {
    const [state, setState] = useState<UpdateState | null>(null);
    const apply = (raw: string) => {
        const s = parseUpdateState(raw);
        if (s) setState(s);
    };

    useEffect(() => {
        const off = EventsOn("update:state", (raw: string) => apply(raw));
        GetUpdateState().then(apply).catch(console.error);
        AutoCheckForUpdate().then(apply).catch(console.error);
        return off;
    }, []);

    return [state, apply];
}

export function formatReleaseDate(iso: string): string {
    const d = new Date(iso);
    if (!iso || isNaN(d.getTime())) return "";
    return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
}
