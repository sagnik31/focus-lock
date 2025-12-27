import { storage, sysinfo } from "../../wailsjs/go/models";
import { AppSelector } from "./AppSelector";
import { InlineKeypad } from "./InlineKeypad";

interface AppLayoutProps {
    config: storage.Config | null;
    newApp: string;
    setNewApp: (val: string) => void;
    error: string;

    // State
    isSelectorOpen: boolean;
    setIsSelectorOpen: (val: boolean) => void;
    showConfirm: boolean;
    setShowConfirm: (val: boolean) => void;
    pendingSession: { h: number, m: number } | null;

    // Data
    topApps: sysinfo.AppInfo[];
    appMap: Map<string, sysinfo.AppInfo>;

    // Handlers
    handleAdd: () => void;
    handleAddByName: (name: string) => void;
    handleRemove: (app: string) => void;
    handleSaveApps: (apps: string[]) => void;
    handleRequestStart: (h: number, m: number) => void;
    confirmStart: () => void;
}

export function AppLayout({
    config,
    newApp,
    setNewApp,
    error,
    isSelectorOpen,
    setIsSelectorOpen,
    showConfirm,
    setShowConfirm,
    pendingSession,
    topApps,
    appMap,
    handleAdd,
    handleAddByName,
    handleRemove,
    handleSaveApps,
    handleRequestStart,
    confirmStart
}: AppLayoutProps) {

    if (!config) return <div className="p-8 text-white">Loading...</div>;

    const isLocked = new Date(config.lock_end_time) > new Date();

    // Calculate time remaining
    const timeLeft = isLocked ? Math.max(0, Math.floor((new Date(config.lock_end_time).getTime() - new Date().getTime()) / 1000)) : 0;
    const hoursLeft = Math.floor(timeLeft / 3600);
    const minutesLeft = Math.floor((timeLeft % 3600) / 60);

    return (
        <div id="app" className="min-h-screen bg-slate-900 text-slate-100 p-8 font-sans relative">
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
                <section className="bg-slate-800 p-6 rounded-xl shadow-lg border border-slate-700">
                    {isLocked ? (
                        <div className="space-y-4 text-center">
                            <div className="flex items-baseline justify-center gap-2">
                                <div className="text-5xl font-bold text-blue-400">
                                    {hoursLeft > 0 ? (
                                        <span>{hoursLeft} hrs </span>
                                    ) : null}
                                    <span>{minutesLeft} mins</span>
                                </div>
                                <span className="text-slate-400 text-sm">remaining</span>
                            </div>
                            <p className="text-slate-400 text-sm">Focus Mode Active</p>
                        </div>
                    ) : (
                        <div className="flex gap-8 items-start justify-between">
                            {/* Keypad Section (Left) */}
                            <div className="space-y-4">
                                <p className="text-slate-400 text-sm mb-4">Set Focus Duration</p>
                                <InlineKeypad onStart={handleRequestStart} />
                            </div>

                            {/* Frequent Apps List (Right) */}
                            <div className="flex-1 flex flex-col items-end space-y-2 pt-8">
                                <h3 className="text-slate-400 text-xs uppercase tracking-widest mb-1">Frequently Blocked</h3>
                                {topApps.length > 0 ? (
                                    <div className="flex flex-col items-end gap-2">
                                        {topApps.map(app => {
                                            const isBlocked = config.blocked_apps.includes(app.exe);
                                            return (
                                                <button
                                                    key={app.exe}
                                                    onClick={() => !isBlocked && handleAddByName(app.exe)}
                                                    disabled={isBlocked}
                                                    className={`flex items-center gap-2 px-3 py-2 rounded-lg text-sm transition-all ${isBlocked
                                                        ? "bg-slate-700/50 text-slate-500 cursor-default"
                                                        : "bg-slate-700 hover:bg-slate-600 text-slate-200"
                                                        }`}
                                                >
                                                    <span className="font-medium">{app.name}</span>
                                                    {app.icon ? (
                                                        <img src={app.icon} alt="" className="w-5 h-5 object-contain" />
                                                    ) : (
                                                        <div className="w-5 h-5 bg-slate-600 rounded-sm"></div>
                                                    )}
                                                </button>
                                            );
                                        })}
                                    </div>
                                ) : (
                                    <div className="text-slate-600 text-xs italic">
                                        No recent apps found.
                                    </div>
                                )}
                            </div>
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
                            onClick={() => setIsSelectorOpen(true)}
                            disabled={isLocked}
                            className="px-6 bg-slate-700 hover:bg-slate-600 rounded-lg font-semibold disabled:opacity-50"
                        >
                            Add App
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

            {/* Confirmation Modal */}
            {showConfirm && pendingSession && (
                <div className="fixed inset-0 bg-black/80 backdrop-blur-sm flex items-center justify-center z-50 p-4">
                    <div className="bg-slate-800 border border-slate-600 rounded-xl shadow-2xl max-w-md w-full p-6 space-y-6">
                        <div className="space-y-2 text-center">
                            <h3 className="text-2xl font-bold text-white">Start Focus Session?</h3>
                            <p className="text-slate-400">You are about to lock your device.</p>
                        </div>

                        <div className="bg-slate-900/50 rounded-lg p-4 space-y-3">
                            <div className="flex justify-between items-center text-sm">
                                <span className="text-slate-400">Duration</span>
                                <span className="text-xl font-bold text-blue-400">
                                    {pendingSession.h > 0 ? `${pendingSession.h}h ` : ''}{pendingSession.m}m
                                </span>
                            </div>
                            <div className="flex justify-between items-center text-sm">
                                <span className="text-slate-400">Apps Blocked</span>
                                <span className="text-white font-semibold">{config.blocked_apps.length} Applications</span>
                            </div>
                        </div>

                        {config.blocked_apps.length > 0 && (
                            <div className="text-sm text-slate-500">
                                <p className="mb-2">Blocking:</p>
                                <div className="flex flex-wrap gap-2">
                                    {config.blocked_apps.slice(0, 5).map(app => (
                                        <span key={app} className="bg-slate-700 px-2 py-1 rounded text-slate-300 text-xs">
                                            {appMap.get(app.toLowerCase())?.name || app}
                                        </span>
                                    ))}
                                    {config.blocked_apps.length > 5 && (
                                        <span className="text-slate-600 px-2 py-1 text-xs">
                                            +{config.blocked_apps.length - 5} more
                                        </span>
                                    )}
                                </div>
                            </div>
                        )}

                        <div className="flex gap-4 pt-2">
                            <button
                                onClick={() => setShowConfirm(false)}
                                className="flex-1 px-4 py-3 rounded-lg bg-slate-700 hover:bg-slate-600 text-white font-semibold transition-colors"
                            >
                                Cancel
                            </button>
                            <button
                                onClick={confirmStart}
                                className="flex-1 px-4 py-3 rounded-lg bg-red-600 hover:bg-red-500 text-white font-bold shadow-lg shadow-red-900/20 transition-all transform hover:scale-[1.02]"
                            >
                                START FOCUS
                            </button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}
