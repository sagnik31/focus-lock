// A session the user has picked but not yet confirmed. For "focus until"
// sessions, `until` (epoch ms) is the source of truth and h/m are only for
// display; the real duration is computed against the trusted clock at start.
export type PendingSession = { h: number; m: number; until?: number };

// Human duration: "45m", "2h 15m", "3d 4h 10m".
export const formatDuration = (totalSeconds: number): string => {
    const mins = Math.max(0, Math.round(totalSeconds / 60));
    const d = Math.floor(mins / 1440);
    const h = Math.floor((mins % 1440) / 60);
    const m = mins % 60;
    const parts: string[] = [];
    if (d) parts.push(`${d}d`);
    if (h) parts.push(`${h}h`);
    if (m || parts.length === 0) parts.push(`${m}m`);
    return parts.join(' ');
};
