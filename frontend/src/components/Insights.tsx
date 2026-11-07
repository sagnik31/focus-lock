import { useEffect, useState } from 'react';
import { GetInsights } from "../../wailsjs/go/bridge/App";
import { sysinfo } from "../../wailsjs/go/models";
import { X, Flame, Trophy, Clock, Ban, CalendarDays } from 'lucide-react';

interface SessionRecord {
    start: string;
    end: string;
    duration_sec: number;
    kind: string;
    app_count: number;
    site_count: number;
}

interface Insights {
    current_streak: number;
    longest_streak: number;
    total_focus_seconds: number;
    session_count: number;
    kills_by_hour: number[];
    top_killed: { name: string; count: number }[];
    recent_sessions: SessionRecord[];
}

interface InsightsModalProps {
    onClose: () => void;
    appMap: Map<string, sysinfo.AppInfo>;
}

function formatDuration(totalSec: number): string {
    if (totalSec <= 0) return "0m";
    const h = Math.floor(totalSec / 3600);
    const m = Math.round((totalSec % 3600) / 60);
    if (h > 0) return m > 0 ? `${h}h ${m}m` : `${h}h`;
    return `${m}m`;
}

function formatWhen(iso: string): string {
    const d = new Date(iso);
    if (isNaN(d.getTime())) return "";
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) +
        " · " + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

export function InsightsModal({ onClose, appMap }: InsightsModalProps) {
    const [data, setData] = useState<Insights | null>(null);
    const [error, setError] = useState("");

    useEffect(() => {
        GetInsights()
            .then((raw) => setData(JSON.parse(raw) as Insights))
            .catch((e) => setError(String(e)));
    }, []);

    const prettyApp = (exe: string) => appMap.get(exe.toLowerCase())?.name || exe;

    const hours = data?.kills_by_hour || [];
    const maxHour = Math.max(1, ...hours);
    const peakHour = hours.length === 24 ? hours.indexOf(Math.max(...hours)) : -1;
    const hasKills = hours.some((v) => v > 0);

    const maxKill = Math.max(1, ...(data?.top_killed || []).map((t) => t.count));

    return (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
            <div className="bg-slate-900 border border-white/10 rounded-2xl shadow-2xl w-full max-w-2xl max-h-[85vh] overflow-y-auto">
                {/* Header */}
                <div className="flex items-center justify-between px-6 py-4 border-b border-white/10 sticky top-0 bg-slate-900 z-10">
                    <h2 className="text-lg font-bold text-slate-100 flex items-center gap-2">
                        <CalendarDays className="w-5 h-5 text-blue-400" />
                        Insights
                    </h2>
                    <button
                        onClick={onClose}
                        aria-label="Close insights"
                        className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-white/10 transition-colors"
                    >
                        <X className="w-5 h-5" />
                    </button>
                </div>

                <div className="p-6 space-y-6">
                    {error && (
                        <div className="bg-red-500/10 border border-red-500/20 text-red-200 px-4 py-3 rounded-xl text-sm">
                            Couldn't load insights: {error}
                        </div>
                    )}

                    {!data && !error && (
                        <div className="text-slate-500 text-sm text-center py-8">Loading…</div>
                    )}

                    {data && (
                        <>
                            {/* Top stat cards */}
                            <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
                                <StatCard icon={<Flame className="w-4 h-4 text-orange-400" />} label="Current streak" value={`${data.current_streak}d`} />
                                <StatCard icon={<Trophy className="w-4 h-4 text-amber-400" />} label="Longest streak" value={`${data.longest_streak}d`} />
                                <StatCard icon={<Clock className="w-4 h-4 text-blue-400" />} label="Total focus" value={formatDuration(data.total_focus_seconds)} />
                                <StatCard icon={<CalendarDays className="w-4 h-4 text-emerald-400" />} label="Sessions" value={`${data.session_count}`} />
                            </div>

                            {/* Most-attempted apps */}
                            <section>
                                <h3 className="text-slate-400 text-xs uppercase tracking-widest mb-3 flex items-center gap-2">
                                    <Ban className="w-3.5 h-3.5" /> Most-blocked apps
                                </h3>
                                {data.top_killed.length === 0 ? (
                                    <p className="text-slate-600 text-sm">No blocked apps have been launched during a session yet.</p>
                                ) : (
                                    <div className="space-y-2">
                                        {data.top_killed.map((t) => (
                                            <div key={t.name} className="flex items-center gap-3">
                                                <div className="w-28 shrink-0 truncate text-sm text-slate-300" title={prettyApp(t.name)}>
                                                    {prettyApp(t.name)}
                                                </div>
                                                <div className="flex-1 bg-slate-800/60 rounded-full h-3 overflow-hidden">
                                                    <div
                                                        className="h-full bg-gradient-to-r from-blue-600 to-blue-400 rounded-full"
                                                        style={{ width: `${(t.count / maxKill) * 100}%` }}
                                                    />
                                                </div>
                                                <div className="w-10 text-right text-sm text-slate-400 tabular-nums">{t.count}</div>
                                            </div>
                                        ))}
                                    </div>
                                )}
                            </section>

                            {/* Time-of-day histogram */}
                            <section>
                                <h3 className="text-slate-400 text-xs uppercase tracking-widest mb-3 flex items-center gap-2">
                                    <Clock className="w-3.5 h-3.5" /> When you get distracted
                                </h3>
                                {!hasKills ? (
                                    <p className="text-slate-600 text-sm">No distraction attempts recorded yet.</p>
                                ) : (
                                    <>
                                        <div className="flex items-end gap-[3px] h-24">
                                            {hours.map((v, h) => (
                                                <div
                                                    key={h}
                                                    className={`flex-1 rounded-t ${h === peakHour ? 'bg-orange-400' : 'bg-blue-500/50'}`}
                                                    style={{ height: `${Math.max(2, (v / maxHour) * 100)}%` }}
                                                    title={`${h.toString().padStart(2, '0')}:00 — ${v} attempt${v === 1 ? '' : 's'}`}
                                                />
                                            ))}
                                        </div>
                                        <div className="flex justify-between text-[10px] text-slate-600 mt-1">
                                            <span>12a</span><span>6a</span><span>12p</span><span>6p</span><span>11p</span>
                                        </div>
                                        {peakHour >= 0 && (
                                            <p className="text-slate-500 text-xs mt-2">
                                                Peak distraction time: around {formatHour(peakHour)}.
                                            </p>
                                        )}
                                    </>
                                )}
                            </section>

                            {/* Recent sessions */}
                            <section>
                                <h3 className="text-slate-400 text-xs uppercase tracking-widest mb-3 flex items-center gap-2">
                                    <CalendarDays className="w-3.5 h-3.5" /> Recent sessions
                                </h3>
                                {data.recent_sessions.length === 0 ? (
                                    <p className="text-slate-600 text-sm">No completed sessions yet. Finish a focus session to start your history.</p>
                                ) : (
                                    <div className="space-y-1.5">
                                        {data.recent_sessions.map((s, i) => (
                                            <div key={i} className="flex items-center justify-between bg-slate-800/40 rounded-lg px-3 py-2 text-sm">
                                                <span className="text-slate-300">{formatWhen(s.start)}</span>
                                                <span className="text-slate-400 tabular-nums">{formatDuration(s.duration_sec)}</span>
                                            </div>
                                        ))}
                                    </div>
                                )}
                            </section>
                        </>
                    )}
                </div>
            </div>
        </div>
    );
}

function StatCard({ icon, label, value }: { icon: React.ReactNode; label: string; value: string }) {
    return (
        <div className="bg-slate-800/40 border border-white/5 rounded-xl p-3 flex flex-col gap-1">
            <div className="flex items-center gap-1.5 text-[11px] uppercase tracking-wide text-slate-500">
                {icon}{label}
            </div>
            <div className="text-xl font-bold text-slate-100">{value}</div>
        </div>
    );
}

function formatHour(h: number): string {
    if (h === 0) return "12 AM";
    if (h === 12) return "12 PM";
    return h < 12 ? `${h} AM` : `${h - 12} PM`;
}
