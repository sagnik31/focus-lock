import { useState, useEffect, useMemo } from 'react';
import { AddApp, RemoveApp, StartFocus, GetConfig, SetBlockedApps, GetInstalledApps } from "../wailsjs/go/main/App";
import { storage, sysinfo } from "../wailsjs/go/models";
import { AppSelector } from "./components/AppSelector";
import { InlineKeypad } from "./components/InlineKeypad";

function App() {
    const [config, setConfig] = useState<storage.Config | null>(null);
    const [newApp, setNewApp] = useState("");
    // Time state managed by InlineKeypad internal buffer
    // Seconds removed per new design requirement
    const [error, setError] = useState("");
    const [isSelectorOpen, setIsSelectorOpen] = useState(false);
    const [installedApps, setInstalledApps] = useState<sysinfo.AppInfo[]>([]);

    const refresh = async () => {
        try {
            const data = await GetConfig();
            setConfig(data);
        } catch (e) {
            console.error(e);
        }
    };

    useEffect(() => {
        refresh();
        const interval = setInterval(refresh, 2000); // Poll for updates

        // Fetch installed apps initially to populate names/icons
        GetInstalledApps().then(setInstalledApps).catch(console.error);

        return () => clearInterval(interval);
    }, []);

    const appMap = useMemo(() => {
        const map = new Map<string, sysinfo.AppInfo>();
        installedApps.forEach(app => {
            map.set(app.exe.toLowerCase(), app);
        });
        return map;
    }, [installedApps]);

    const handleAdd = async () => {
        if (!newApp) return;
        try {
            await AddApp(newApp);
            setNewApp("");
            refresh();
        } catch (err: any) {
            setError(err.toString());
        }
    };

    const handleRemove = async (app: string) => {
        await RemoveApp(app);
        refresh();
    };

    const handleSaveApps = async (apps: string[]) => {
        console.log("Saving apps:", apps);
        try {
            await SetBlockedApps(apps);
            console.log("SetBlockedApps completed");
            refresh();
        } catch (err: any) {
            console.error("SetBlockedApps failed:", err);
            setError(err.toString());
        }
    };

    const handleStart = async (h: number, m: number) => {
        try {
            const totalSeconds = (h * 3600) + (m * 60);
            if (totalSeconds <= 0) {
                setError("Duration must be greater than 0");
                return;
            }
            await StartFocus(totalSeconds);
            refresh();
        } catch (err: any) {
            setError("Failed to start: " + err);
        }
    };


    if (!config) return <div className="p-8 text-white">Loading...</div>;

    const isLocked = new Date(config.lock_end_time) > new Date();

    // Calculate time remaining
    const timeLeft = isLocked ? Math.max(0, Math.floor((new Date(config.lock_end_time).getTime() - new Date().getTime()) / 1000)) : 0;
    const hoursLeft = Math.floor(timeLeft / 3600);
    const minutesLeft = Math.floor((timeLeft % 3600) / 60);
    const secondsLeft = timeLeft % 60;

    return (
        <div id="app" className="min-h-screen bg-slate-900 text-slate-100 p-8 font-sans">
            <div className="w-full space-y-8">
                <header className="flex justify-between items-center border-b border-slate-700 pb-4">
                    <h1 className="text-3xl font-bold bg-clip-text text-transparent bg-gradient-to-r from-blue-400 to-purple-500">
                        Focus Lock (v1.1)
                    </h1>
                    <div className={`px-3 py-1 rounded-full text-sm font-bold ${isLocked ? 'bg-red-500/20 text-red-400' : 'bg-green-500/20 text-green-400'}`}>
                        {isLocked ? "LOCKED" : "READY"}
                    </div>
                </header>

                {error && (
                    <div className="bg-red-500/10 border border-red-500/50 text-red-200 p-4 rounded-lg">
                        {error}
                    </div>
                )}

                {/* Timer Section */}
                <section className="bg-slate-800 p-6 rounded-xl shadow-lg border border-slate-700 text-center">
                    {isLocked ? (
                        <div className="space-y-4">
                            <div className="text-6xl font-mono font-bold tracking-widest text-blue-400">
                                {hoursLeft > 0 ? (
                                    <span>{hoursLeft.toString().padStart(2, '0')}:</span>
                                ) : null}
                                {minutesLeft.toString().padStart(2, '0')}:{secondsLeft.toString().padStart(2, '0')}
                            </div>
                            <p className="text-slate-400 text-sm">Focus Mode Active</p>
                        </div>
                    ) : (
                        <div className="space-y-4">
                            <p className="text-slate-400 text-sm mb-4">Set Duration</p>
                            <InlineKeypad onStart={handleStart} />
                        </div>
                    )}
                </section>

                {/* Apps Section */}
                <section className="space-y-4">
                    <h2 className="text-xl font-semibold text-slate-300">Blocked Applications</h2>
                    <div className="flex gap-2">
                        <input
                            type="text"
                            placeholder="e.g. WhatsApp.exe"
                            value={newApp}
                            onChange={(e) => setNewApp(e.target.value)}
                            onKeyDown={(e) => e.key === 'Enter' && handleAdd()}
                            className="flex-1 bg-slate-800 border border-slate-600 rounded-lg px-4 py-2 focus:ring-2 focus:ring-blue-500 outline-none"
                            disabled={isLocked}
                        />
                        <button
                            onClick={handleAdd}
                            disabled={isLocked}
                            className="px-6 bg-slate-700 hover:bg-slate-600 rounded-lg font-semibold disabled:opacity-50"
                        >
                            Add
                        </button>
                        <button
                            onClick={() => setIsSelectorOpen(true)}
                            disabled={isLocked}
                            className="px-4 bg-slate-700 hover:bg-slate-600 rounded-lg font-semibold disabled:opacity-50 text-slate-300"
                            title="Select from installed apps"
                        >
                            ☰
                        </button>
                    </div>

                    <div className="grid grid-cols-3 gap-4">
                        {config.blocked_apps.length === 0 && (
                            <div className="text-center py-8 text-slate-500 italic border border-dashed border-slate-700 rounded-lg">
                                No apps blocked. Add one above.
                            </div>
                        )}
                        {config.blocked_apps.map((exeName) => {
                            const appInfo = appMap.get(exeName.toLowerCase());
                            return (
                                <div key={exeName} className="flex items-center justify-between bg-slate-800 p-3 rounded-lg border border-slate-700">
                                    <div className="flex items-center gap-3">
                                        {/* Icon */}
                                        {appInfo?.icon ? (
                                            <img src={appInfo.icon} alt={appInfo.name} className="w-8 h-8 object-contain" />
                                        ) : (
                                            <div className="w-8 h-8 bg-slate-600 rounded flex items-center justify-center text-xs text-white">?</div>
                                        )}

                                        <div className="flex flex-col">
                                            {/* Friendly Name */}
                                            <span className="font-semibold text-white">
                                                {appInfo?.name || exeName}
                                            </span>
                                        </div>
                                    </div>
                                    <button
                                        onClick={() => handleRemove(exeName)}
                                        disabled={isLocked}
                                        className="text-slate-500 hover:text-red-400 disabled:opacity-30 p-1"
                                    >
                                        Remove
                                    </button>
                                </div>
                            );
                        })}
                    </div>
                </section>
            </div>

            <AppSelector
                isOpen={isSelectorOpen}
                onClose={() => setIsSelectorOpen(false)}
                onSave={handleSaveApps}
                currentlyBlocked={config.blocked_apps || []}
            />
        </div>
    );
}

export default App;
