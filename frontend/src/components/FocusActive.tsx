import { useEffect, useState } from 'react';
import { sysinfo } from "../../wailsjs/go/models";

interface FocusActiveProps {
    endTime: string;
    blockedApps: string[];
    appMap: Map<string, sysinfo.AppInfo>;
}

export function FocusActive({ endTime, blockedApps, appMap }: FocusActiveProps) {
    const [timeLeft, setTimeLeft] = useState(0);

    useEffect(() => {
        const calculateTime = () => {
            const end = new Date(endTime).getTime();
            const now = new Date().getTime();
            const remaining = Math.max(0, Math.floor((end - now) / 1000));
            setTimeLeft(remaining);
        };

        calculateTime();
        // Update every minute is enough for "X hr Y min", but 5s is good to keep it relatively fresh without "running timer" feel
        const interval = setInterval(calculateTime, 5000);
        return () => clearInterval(interval);
    }, [endTime]);

    const hours = Math.floor(timeLeft / 3600);
    const minutes = Math.floor((timeLeft % 3600) / 60);
    // If < 1 minute, show "< 1 min" or just 1 min.
    const displayMinutes = (hours === 0 && minutes === 0 && timeLeft > 0) ? 1 : minutes;

    return (
        <div className="min-h-screen bg-slate-900 text-slate-100 flex flex-col items-center justify-start p-8 font-sans relative overflow-y-auto">
            {/* Background Pulse Effect */}
            <div className="absolute inset-0 bg-blue-900/5 pointer-events-none fixed"></div>

            <div className="relative z-10 w-full max-w-4xl flex flex-col items-center space-y-12 pt-12">

                {/* Header & Timer */}
                <div className="text-center space-y-6">
                    <div className="inline-block px-4 py-1.5 rounded-full bg-blue-500/10 border border-blue-500/20 text-blue-400 font-semibold tracking-wider text-sm uppercase">
                        Focus Mode Active
                    </div>

                    <div className="flex items-baseline justify-center gap-3 text-6xl md:text-7xl font-bold text-white drop-shadow-xl">
                        {hours > 0 && (
                            <>
                                <span>{hours}</span>
                                <span className="text-2xl text-slate-400 font-normal ml-1 mr-3">hr</span>
                            </>
                        )}
                        <span>{displayMinutes}</span>
                        <span className="text-2xl text-slate-400 font-normal ml-1">min</span>
                    </div>
                    <p className="text-slate-400 text-lg">remaining</p>
                </div>

                {/* Blocked Apps Grid */}
                <div className="w-full bg-slate-800/50 rounded-2xl border border-slate-700/50 p-8 backdrop-blur-sm">
                    <h3 className="text-slate-400 text-sm uppercase tracking-widest mb-6 text-center">
                        Blocked Applications ({blockedApps.length})
                    </h3>

                    <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
                        {blockedApps.map((exeName) => {
                            const appInfo = appMap.get(exeName.toLowerCase());
                            return (
                                <div key={exeName} className="flex items-center gap-3 bg-slate-900/80 p-3 rounded-lg border border-slate-700/50 shadow-sm opacity-75">
                                    {/* Icon */}
                                    {appInfo?.icon ? (
                                        <img src={appInfo.icon} alt={appInfo.name} className="w-8 h-8 object-contain" />
                                    ) : (
                                        <div className="w-8 h-8 bg-slate-700 rounded flex items-center justify-center text-xs text-slate-400 font-bold">
                                            {exeName.charAt(0).toUpperCase()}
                                        </div>
                                    )}

                                    <div className="flex flex-col overflow-hidden">
                                        <span className="font-medium text-slate-200 truncate text-sm" title={appInfo?.name || exeName}>
                                            {appInfo?.name || exeName}
                                        </span>
                                    </div>
                                </div>
                            );
                        })}
                    </div>
                </div>

                <div className="text-slate-500 text-sm opacity-60">
                    "Success is the sum of small efforts, repeated day in and day out."
                </div>

            </div>
        </div>
    );
}
