import { useEffect, useState } from 'react';
import { sysinfo } from "../../wailsjs/go/models";
import { EmergencyUnlock } from "../../wailsjs/go/bridge/App";
import { AlertTriangle, Lock, Plus } from 'lucide-react';

interface FocusActiveProps {
    endTime: string;
    pausedUntil: any;
    blockedApps: string[];
    blockedSites: string[];
    appMap: Map<string, sysinfo.AppInfo>;
    isSchedule?: boolean;
    emergencyUnlocksUsed: number;
    onShowSettings?: () => void;
    onQuit?: () => void;
    enforcementError?: string;
    // Offset (ms) from this browser's clock to the enforcer's trusted clock, so
    // the countdown matches what the Ghost enforces even if the system clock moves.
    nowOffsetMs?: number;
}

export function FocusActive({ endTime, blockedApps, blockedSites, appMap, pausedUntil, isSchedule, emergencyUnlocksUsed, onShowSettings, onQuit, enforcementError, nowOffsetMs = 0 }: FocusActiveProps) {
    const [timeLeft, setTimeLeft] = useState(0);
    const [pauseLeft, setPauseLeft] = useState(0);

    const calculateTime = () => {
        const now = new Date().getTime() + nowOffsetMs;

        // 1. Check Pause Status
        if (pausedUntil) {
            const pauseEnd = new Date(pausedUntil).getTime();
            const pLeft = Math.max(0, Math.floor((pauseEnd - now) / 1000));
            setPauseLeft(pLeft);
        } else {
            setPauseLeft(0);
        }

        // 2. Check Lock Status
        const end = new Date(endTime).getTime();
        const remaining = Math.max(0, Math.floor((end - now) / 1000));
        setTimeLeft(remaining);
    };

    useEffect(() => {
        calculateTime();
        const interval = setInterval(calculateTime, 1000);
        return () => clearInterval(interval);
    }, [endTime, pausedUntil, nowOffsetMs]);

    const handleEmergencyUnlock = async () => {
        try {
            await EmergencyUnlock();
            // The polling in App.tsx will update props shortly
        } catch (e) {
            console.error("Failed to emergency unlock:", e);
        }
    };

    const hours = Math.floor(timeLeft / 3600);
    const minutes = Math.floor((timeLeft % 3600) / 60);
    const displayMinutes = (hours === 0 && minutes === 0 && timeLeft > 0) ? 1 : minutes;

    const isPaused = pauseLeft > 0;
    const pauseMins = Math.floor(pauseLeft / 60);
    const pauseSecs = pauseLeft % 60;

    return (
        <div className={`min-h-screen bg-slate-950 text-slate-100 flex flex-col items-center justify-start p-8 font-sans relative overflow-y-auto transition-colors duration-500 ${isPaused ? 'border-4 border-amber-500/20' : ''}`}>
            {/* Ambient Background Effects (match main layout) */}
            <div className="absolute top-0 left-1/4 w-96 h-96 bg-blue-500/10 rounded-full blur-3xl pointer-events-none -translate-y-1/2 fixed"></div>
            <div className="absolute bottom-0 right-1/4 w-96 h-96 bg-blue-500/5 rounded-full blur-3xl pointer-events-none translate-y-1/2 fixed"></div>

            <div className="relative z-10 w-full max-w-4xl flex flex-col items-center space-y-12 pt-12">

                {enforcementError && (
                    <div className="w-full bg-amber-500/10 border border-amber-500/30 text-amber-200 px-4 py-3 rounded-xl flex items-start gap-3 text-sm">
                        <AlertTriangle className="w-4 h-4 text-amber-400 mt-0.5 shrink-0" />
                        <div>
                            <p className="font-semibold text-amber-300">Enforcement warning</p>
                            <p className="text-amber-200/80">{enforcementError}</p>
                        </div>
                    </div>
                )}

                {/* Header & Timer */}
                <div className="text-center space-y-6">
                    {isPaused ? (
                        <>
                            <div className="inline-block px-4 py-1.5 rounded-full bg-amber-500/20 border border-amber-500/40 text-amber-400 font-semibold tracking-wider text-sm uppercase animate-pulse">
                                Emergency Unlock Active
                            </div>
                            <div className="flex flex-col items-center justify-center">
                                <div className="text-5xl font-bold text-amber-100 drop-shadow-xl mb-2">
                                    {pauseMins}:{pauseSecs.toString().padStart(2, '0')}
                                </div>
                                <p className="text-amber-400/80 text-sm uppercase tracking-wide">Usage Window Remaining</p>
                            </div>
                            <div className="text-slate-500 text-sm mt-4">
                                Focus will resume automatically.
                            </div>
                        </>
                    ) : (
                        <>
                            <div className="inline-block px-4 py-1.5 rounded-full border font-semibold tracking-wider text-sm uppercase bg-blue-500/10 border-blue-500/20 text-blue-400">
                                {isSchedule ? 'Scheduled Session Active' : 'Focus Mode Active'}
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

                            {/* Emergency Unlock Button */}
                            {/* Emergency Unlock Button */}
                            <div className="pt-8 text-center space-y-2">
                                <button
                                    onClick={handleEmergencyUnlock}
                                    disabled={emergencyUnlocksUsed >= 2}
                                    className={`px-6 py-2 rounded-lg transition-all text-sm font-medium flex items-center gap-2 mx-auto ${emergencyUnlocksUsed >= 2
                                        ? 'bg-slate-700/50 text-slate-500 cursor-not-allowed border border-slate-700'
                                        : 'bg-red-500/10 hover:bg-red-500/20 text-red-400 border border-red-500/20 hover:scale-105 active:scale-95'
                                        }`}
                                    title={emergencyUnlocksUsed >= 2 ? "Emergency unlock limit reached" : "Unlocks apps for 1 minute"}
                                >
                                    <Lock className="h-4 w-4" />
                                    Emergency Unlock (1m) • {Math.max(0, 2 - emergencyUnlocksUsed)} left
                                </button>
                                {emergencyUnlocksUsed >= 2 && (
                                    <p className="text-xs text-slate-500">Session limit reached</p>
                                )}
                            </div>

                            {/* Manage Block List Button */}
                            {onShowSettings && (
                                <div className="pt-4">
                                    <button
                                        onClick={onShowSettings}
                                        className="px-5 py-2 rounded-lg bg-slate-700/50 hover:bg-slate-700 text-slate-300 hover:text-white text-sm font-medium transition-all flex items-center gap-2 mx-auto border border-slate-600/50 hover:border-slate-500"
                                    >
                                        <Plus className="h-4 w-4" />
                                        Add More to Block List
                                    </button>
                                </div>
                            )}

                            {/* Quit Button — only once the lock has expired.
                                An active lock cannot be ended early; the
                                enforcer refuses it, so we don't offer it. */}
                            {onQuit && timeLeft <= 0 && (
                                <div className="pt-2">
                                    <button
                                        onClick={onQuit}
                                        className="text-xs text-slate-600 hover:text-red-400 transition-colors uppercase tracking-widest font-bold"
                                    >
                                        Quit Session
                                    </button>
                                </div>
                            )}
                        </>
                    )}
                </div>

                {/* Blocked Apps Grid */}
                <div className={`w-full bg-slate-900/40 rounded-2xl border border-white/5 p-8 backdrop-blur-md shadow-xl transition-opacity duration-300 ${isPaused ? 'opacity-50 grayscale' : 'opacity-100'}`}>
                    <h3 className="text-slate-400 text-sm uppercase tracking-widest mb-6 text-center">
                        {isPaused ? "Apps Temporarily Unlocked" : `Blocked Applications (${blockedApps.length})`}
                    </h3>

                    <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
                        {blockedApps.map((exeName) => {
                            const appInfo = appMap.get(exeName.toLowerCase());
                            return (
                                <div key={exeName} className="flex items-center gap-3 bg-slate-800/40 p-3 rounded-lg border border-white/5 shadow-sm opacity-75">
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

                    {/* Blocked Websites Grid */}
                    {blockedSites && blockedSites.length > 0 && (
                        <div className="mt-8">
                            <h3 className="text-slate-400 text-sm uppercase tracking-widest mb-6 text-center">
                                {isPaused ? "Sites Temporarily Unlocked" : `Blocked Websites (${blockedSites.length})`}
                            </h3>

                            <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
                                {blockedSites.map((site) => (
                                    <div key={site} className="flex items-center gap-3 bg-slate-800/40 p-3 rounded-lg border border-white/5 shadow-sm opacity-75">
                                        {/* Icon */}
                                        <div className="w-8 h-8 bg-slate-700 rounded flex items-center justify-center overflow-hidden">
                                            <img
                                                src={`https://www.google.com/s2/favicons?domain=${site}&sz=64`}
                                                alt={site}
                                                className="w-5 h-5 opacity-70"
                                                onError={(e) => (e.currentTarget.style.display = 'none')}
                                            />
                                        </div>

                                        <div className="flex flex-col overflow-hidden">
                                            <span className="font-medium text-slate-200 truncate text-sm" title={site}>
                                                {site}
                                            </span>
                                        </div>
                                    </div>
                                ))}
                            </div>
                        </div>
                    )}
                </div>

                <div className="text-slate-500 text-sm opacity-60">
                    "Success is the sum of small efforts, repeated day in and day out."
                </div>

            </div>
        </div>
    );
}
