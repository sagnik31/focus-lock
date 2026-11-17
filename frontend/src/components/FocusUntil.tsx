import React, { useEffect, useMemo, useState } from 'react';
import { AlertTriangle, CalendarClock } from 'lucide-react';
import { formatDuration } from './duration';

interface FocusUntilProps {
    // Called with the chosen end time (epoch ms, local wall-clock time).
    onStart: (untilMs: number) => void;
}

const pad = (n: number) => n.toString().padStart(2, '0');
const toDateInput = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
const toTimeInput = (d: Date) => `${pad(d.getHours())}:${pad(d.getMinutes())}`;

// Default suggestion: one hour from now, rounded up to the next quarter hour.
const defaultEnd = () => {
    const d = new Date(Date.now() + 60 * 60 * 1000);
    d.setSeconds(0, 0);
    d.setMinutes(Math.ceil(d.getMinutes() / 15) * 15);
    return d;
};

export const FocusUntil: React.FC<FocusUntilProps> = ({ onStart }) => {
    const [date, setDate] = useState(() => toDateInput(defaultEnd()));
    const [time, setTime] = useState(() => toTimeInput(defaultEnd()));

    // Re-render periodically so "from now" and the past-time check stay current.
    const [now, setNow] = useState(() => Date.now());
    useEffect(() => {
        const id = setInterval(() => setNow(Date.now()), 15_000);
        return () => clearInterval(id);
    }, []);

    // Parsed as local time: "YYYY-MM-DDTHH:MM" without a zone is local.
    const target = useMemo(() => {
        if (!date || !time) return null;
        const t = new Date(`${date}T${time}`);
        return isNaN(t.getTime()) ? null : t;
    }, [date, time]);

    const seconds = target ? Math.floor((target.getTime() - now) / 1000) : 0;
    const inPast = target !== null && seconds <= 0;
    const valid = target !== null && seconds >= 60;

    return (
        <div className="w-full h-full flex flex-col items-center justify-center px-6 pt-10 pb-4 gap-4">
            <div className="w-full max-w-xs grid grid-cols-2 gap-3">
                <label className="flex flex-col gap-1">
                    <span className="text-[10px] font-bold text-slate-500 uppercase tracking-widest">Date</span>
                    <input
                        type="date"
                        value={date}
                        min={toDateInput(new Date(now))}
                        onChange={(e) => setDate(e.target.value)}
                        style={{ colorScheme: 'dark' }}
                        className="bg-slate-900 border border-slate-700/50 focus:border-blue-500/50 rounded-lg px-3 py-2 text-sm text-slate-100 outline-none"
                    />
                </label>
                <label className="flex flex-col gap-1">
                    <span className="text-[10px] font-bold text-slate-500 uppercase tracking-widest">Time</span>
                    <input
                        type="time"
                        value={time}
                        onChange={(e) => setTime(e.target.value)}
                        style={{ colorScheme: 'dark' }}
                        className="bg-slate-900 border border-slate-700/50 focus:border-blue-500/50 rounded-lg px-3 py-2 text-sm text-slate-100 outline-none"
                    />
                </label>
            </div>

            <div className="w-full max-w-xs min-h-[64px] flex items-center justify-center text-center">
                {inPast ? (
                    <div className="w-full flex items-start gap-2 bg-amber-500/10 border border-amber-500/30 text-amber-200 rounded-lg px-3 py-2 text-xs text-left">
                        <AlertTriangle className="w-4 h-4 text-amber-400 shrink-0 mt-0.5" />
                        <span>This time has already passed. Pick a later time, or choose a different date.</span>
                    </div>
                ) : target && valid ? (
                    <div className="space-y-1">
                        <div className="flex items-center justify-center gap-2 text-white font-semibold">
                            <CalendarClock className="w-4 h-4 text-blue-400" />
                            {target.toLocaleString([], { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })}
                        </div>
                        <div className="text-xs text-slate-400">{formatDuration(seconds)} from now</div>
                    </div>
                ) : target ? (
                    <p className="text-xs text-slate-400">Choose a time at least a minute from now.</p>
                ) : (
                    <p className="text-xs text-slate-400">Choose a date and time.</p>
                )}
            </div>

            <button
                onClick={() => target && valid && onStart(target.getTime())}
                disabled={!valid}
                className="w-full max-w-[200px] bg-white text-slate-950 font-semibold py-4 rounded-xl hover:bg-blue-50 transition-colors shadow-lg shadow-white/5 disabled:opacity-40 disabled:cursor-not-allowed focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/60"
            >
                Start focus
            </button>
        </div>
    );
};
