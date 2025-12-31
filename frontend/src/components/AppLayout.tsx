
import { storage, sysinfo } from "../../wailsjs/go/models";
import { AppSelector } from "./AppSelector";
import { InlineKeypad } from "./InlineKeypad";
import { TimeSeeker } from './TimeSeeker';
import { WebsiteSelector } from './WebsiteSelector';
import { useState } from "react";
import logo from '../assets/logo.png';

interface AppLayoutProps {
    config: storage.Config | null;
    newApp: string;
    setNewApp: (val: string) => void;
    error: string;

    // State
    isSelectorOpen: boolean;
    setIsSelectorOpen: (val: boolean) => void;

    // Session State (Controlled by Parent)
    showConfirm: boolean;
    setShowConfirm: (val: boolean) => void;
    pendingSession: { h: number, m: number } | null;
    setPendingSession: (val: { h: number, m: number } | null) => void;

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

    // Website Handlers
    handleAddSite: (url: string) => void;
    handleRemoveSite: (url: string) => void;
    handleAddSites: (urls: string[]) => void;
    handleRemoveSites: (urls: string[]) => void;
    handleToggleVPN: (val: boolean) => void;
}

export const AppLayout: React.FC<AppLayoutProps> = ({
    config,
    newApp,
    setNewApp,
    error,
    isSelectorOpen,
    setIsSelectorOpen,
    showConfirm,
    setShowConfirm,
    pendingSession,
    setPendingSession,
    topApps,
    appMap,
    handleAdd,
    handleAddByName,
    handleRemove,
    handleSaveApps,
    handleRequestStart,
    confirmStart,
    handleAddSite,
    handleRemoveSite,
    handleToggleVPN,
    handleAddSites,
    handleRemoveSites
}) => {
    const [inputMode, setInputMode] = useState<'slider' | 'keypad'>('slider');
    const [activeTab, setActiveTab] = useState<'apps' | 'websites'>('apps');
    const [showDetails, setShowDetails] = useState(false);

    if (!config) return (
        <div className="min-h-screen bg-slate-950 flex items-center justify-center text-slate-400">
            <div className="animate-pulse">Loading System...</div>
        </div>
    );

    return (
        <div id="app" className="min-h-screen bg-slate-950 text-slate-100 font-sans relative overflow-hidden selection:bg-blue-500/30 flex flex-col">
            {/* Ambient Background Effects */}
            <div className="absolute top-0 left-1/4 w-96 h-96 bg-blue-500/10 rounded-full blur-3xl pointer-events-none -translate-y-1/2"></div>
            <div className="absolute bottom-0 right-1/4 w-96 h-96 bg-purple-500/10 rounded-full blur-3xl pointer-events-none translate-y-1/2"></div>

            <div className="relative z-10 container mx-auto px-4 py-4 max-w-5xl flex-1 flex flex-col h-full">

                {/* Header */}
                <header className="flex justify-between items-center mb-6 shrink-0">
                    <div className="flex items-center gap-3">
                        <img src={logo} alt="Focus Lock" className="w-10 h-10 rounded-xl shadow-lg shadow-blue-900/20 object-cover" />
                        <h1 className="text-xl font-bold tracking-tight text-slate-200">
                            Focus Lock
                        </h1>
                    </div>
                    <div className="flex items-center gap-4">
                        <div className="px-3 py-1 rounded-full text-xs font-bold tracking-wider uppercase bg-slate-800/50 border border-slate-700/50 text-slate-400 backdrop-blur-sm">
                            System Ready
                        </div>
                    </div>
                </header>

                {error && (
                    <div className="mb-4 bg-red-500/10 border border-red-500/20 text-red-200 px-4 py-3 rounded-xl flex items-center gap-3 backdrop-blur-sm text-sm shrink-0">
                        <svg className="w-4 h-4 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                        </svg>
                        {error}
                    </div>
                )}

                {/* Main Content Grid - 50/50 Split */}
                <div className="grid grid-cols-1 md:grid-cols-2 gap-6 flex-1 min-h-0">

                    {/* Left Panel: Focus Timer (Hero) */}
                    <div className="flex flex-col h-full">
                        <div className="bg-slate-900/40 backdrop-blur-md rounded-2xl border border-white/5 p-6 shadow-2xl relative overflow-hidden group h-full flex flex-col">
                            <div className="absolute inset-0 bg-gradient-to-b from-white/5 to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-700 pointer-events-none"></div>

                            <div className="relative flex-1 flex flex-col justify-center">
                                <h2 className="text-2xl font-semibold text-white mb-2">Start Session</h2>
                                <p className="text-slate-400 mb-8 text-sm leading-relaxed">
                                    Set your duration. Once started, focus is enforced until time expires.
                                </p>

                                <div className="bg-slate-950/50 rounded-2xl border border-white/5 shadow-inner flex flex-col justify-center relative overflow-hidden h-[340px]">
                                    {inputMode === 'slider' ? (
                                        <>
                                            <TimeSeeker
                                                onDurationChange={(h, m) => setPendingSession({ h, m })}
                                                onStart={() => setShowConfirm(true)} // Trigger confirmation directly
                                                initialMinutes={pendingSession ? (pendingSession.h * 60 + pendingSession.m) : 15}
                                            />
                                            <button
                                                onClick={() => setInputMode('keypad')}
                                                className="absolute bottom-4 right-4 text-[10px] font-bold text-slate-500 hover:text-blue-400 uppercase tracking-widest transition-colors"
                                            >
                                                Custom Time &rarr;
                                            </button>
                                        </>
                                    ) : (
                                        <div className="w-full h-full p-6 flex flex-col items-center justify-center">
                                            <InlineKeypad onStart={handleRequestStart} />
                                            <button
                                                onClick={() => setInputMode('slider')}
                                                className="mt-4 text-[10px] font-bold text-slate-500 hover:text-blue-400 uppercase tracking-widest transition-colors"
                                            >
                                                &larr; Back to Presets
                                            </button>
                                        </div>
                                    )}
                                </div>
                            </div>
                        </div>
                    </div>

                    {/* Right Panel: Blocked Apps (More prominent) */}
                    <div className="flex flex-col h-full min-h-0">

                        {/* Quick Actions / Block List */}
                        <div className="bg-slate-900/40 backdrop-blur-md rounded-2xl border border-white/5 p-6 shadow-xl flex flex-col h-full overflow-hidden">
                            {/* Tab Switcher */}
                            <div className="flex justify-between items-center mb-6 shrink-0">
                                <div className="flex bg-slate-800/50 p-1 rounded-lg border border-slate-700/50">
                                    <button
                                        onClick={() => setActiveTab('apps')}
                                        className={`px-4 py-1.5 rounded-md text-sm font-semibold transition-all ${activeTab === 'apps' ? 'bg-blue-600 text-white shadow-lg shadow-blue-900/40' : 'text-slate-400 hover:text-slate-200'}`}
                                    >
                                        Apps
                                    </button>
                                    <button
                                        onClick={() => setActiveTab('websites')}
                                        className={`px-4 py-1.5 rounded-md text-sm font-semibold transition-all ${activeTab === 'websites' ? 'bg-blue-600 text-white shadow-lg shadow-blue-900/40' : 'text-slate-400 hover:text-slate-200'}`}
                                    >
                                        Websites
                                    </button>
                                </div>

                                {activeTab === 'apps' ? (
                                    <button
                                        onClick={() => setIsSelectorOpen(true)}
                                        className="text-xs font-bold bg-blue-600 hover:bg-blue-500 text-white px-4 py-2 rounded-lg transition-all shadow-lg shadow-blue-900/20"
                                    >
                                        + SELECT ALL
                                    </button>
                                ) : null}
                            </div>

                            {activeTab === 'apps' ? (
                                <>
                                    {/* Add App Input - Larger */}
                                    <div className="relative mb-6 group shrink-0">
                                        <input
                                            type="text"
                                            placeholder="Add e.g. steam.exe"
                                            value={newApp}
                                            onChange={(e) => setNewApp(e.target.value)}
                                            onKeyDown={(e) => e.key === 'Enter' && handleAdd()}
                                            className="w-full bg-slate-950/50 border border-slate-700/50 focus:border-blue-500/50 rounded-xl px-4 py-3 pl-10 focus:ring-2 focus:ring-blue-500/20 outline-none transition-all placeholder:text-slate-500 text-sm"
                                        />
                                        <svg className="w-5 h-5 text-slate-500 absolute left-3.5 top-3 group-focus-within:text-blue-400 transition-colors" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
                                        </svg>
                                    </div>

                                    {/* Apps List - Larger Items */}
                                    <div className="flex-1 overflow-y-auto pr-2 space-y-2 custom-scrollbar min-h-0">
                                        {config.blocked_apps.length === 0 ? (
                                            <div className="h-full flex flex-col items-center justify-center text-slate-500 space-y-3 opacity-60">
                                                <svg className="w-10 h-10" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1} d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                                                </svg>
                                                <span className="text-sm">No locked applications</span>
                                            </div>
                                        ) : (
                                            config.blocked_apps.map((exeName) => {
                                                const appInfo = appMap.get(exeName.toLowerCase());
                                                return (
                                                    <div key={exeName} className="group flex items-center justify-between bg-slate-800/40 hover:bg-slate-800/80 p-3 rounded-lg border border-white/5 hover:border-white/20 transition-all">
                                                        <div className="flex items-center gap-3 overflow-hidden">
                                                            {appInfo?.icon ? (
                                                                <img src={appInfo.icon} alt={appInfo.name} className="w-8 h-8 object-contain opacity-90 group-hover:opacity-100 transition-opacity" />
                                                            ) : (
                                                                <div className="w-8 h-8 bg-slate-700/50 rounded-lg flex items-center justify-center text-xs text-slate-300 font-bold">
                                                                    {exeName.charAt(0).toUpperCase()}
                                                                </div>
                                                            )}
                                                            <div className="flex flex-col min-w-0">
                                                                <span className="font-medium text-slate-200 text-sm truncate group-hover:text-white transition-colors">
                                                                    {appInfo?.name || exeName}
                                                                </span>
                                                            </div>
                                                        </div>
                                                        <button
                                                            onClick={() => handleRemove(exeName)}
                                                            className="opacity-0 group-hover:opacity-100 text-slate-400 hover:text-red-400 p-2 hover:bg-red-400/10 rounded-lg transition-all"
                                                            title="Remove"
                                                        >
                                                            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                                                            </svg>
                                                        </button>
                                                    </div>
                                                );
                                            })
                                        )}
                                    </div>

                                    {/* Suggestions - More Visible */}
                                    {topApps.length > 0 && (
                                        <div className="mt-5 pt-5 border-t border-white/10 shrink-0">
                                            <h4 className="text-xs font-bold text-slate-400 uppercase tracking-widest mb-3">Frequently Blocked Apps</h4>
                                            <div className="flex flex-wrap gap-2">
                                                {topApps.slice(0, 6).map(app => (
                                                    !config.blocked_apps.includes(app.exe) && (
                                                        <button
                                                            key={app.exe}
                                                            onClick={() => handleAddByName(app.exe)}
                                                            className="flex items-center gap-2 px-4 py-2.5 bg-slate-800/80 hover:bg-blue-600/20 hover:text-blue-300 hover:border-blue-500/30 border border-slate-700/50 rounded-lg text-sm text-slate-300 transition-all max-w-[180px]"
                                                        >
                                                            <span className="truncate flex-1 text-left">{app.name}</span>
                                                            <span className="text-blue-500/50 text-xs font-bold">+</span>
                                                        </button>
                                                    )
                                                ))}
                                            </div>
                                        </div>
                                    )}
                                </>
                            ) : (
                                <WebsiteSelector
                                    sites={config.blocked_sites || []} // Handle null/undefined just in case
                                    onAdd={handleAddSite}
                                    onRemove={handleRemoveSite}
                                    onAddSites={handleAddSites}
                                    onRemoveSites={handleRemoveSites}
                                    blockVPN={config.block_common_vpn || true}
                                    onToggleVPN={handleToggleVPN}
                                />
                            )}
                        </div>
                    </div>
                </div>
            </div>

            <AppSelector
                isOpen={isSelectorOpen}
                onClose={() => setIsSelectorOpen(false)}
                onSave={handleSaveApps}
                currentlyBlocked={config.blocked_apps || []}
            />

            {/* Confirmation Modal */}
            {showConfirm && pendingSession && (
                <div className="fixed inset-0 bg-slate-950/80 backdrop-blur-sm flex items-center justify-center z-50 p-4 animate-in fade-in duration-200">
                    <div className="bg-slate-900 border border-white/10 rounded-2xl shadow-2xl max-w-sm w-full p-6 space-y-6 transform scale-100">
                        <div className="text-center space-y-2">
                            <div className="w-12 h-12 bg-blue-500/10 rounded-full flex items-center justify-center mx-auto mb-2 border border-blue-500/20">
                                <svg className="w-6 h-6 text-blue-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
                                </svg>
                            </div>
                            <h3 className="text-lg font-bold text-white">Confirm Lock</h3>
                            <p className="text-slate-400 text-sm">
                                You are about to enter a <span className="text-blue-400 font-bold">{pendingSession.h}h {pendingSession.m}m</span> strict focus session.
                            </p>
                        </div>

                        <div className="bg-black/20 rounded-lg border border-white/5 overflow-hidden">
                            <div className="p-4 space-y-3">
                                <div className="flex justify-between items-center text-sm">
                                    <span className="text-slate-500">Duration</span>
                                    <span className="text-white font-mono font-medium">{pendingSession.h > 0 ? `${pendingSession.h} h` : ''}{pendingSession.m}m 00s</span>
                                </div>
                                <div className="flex justify-between items-center text-sm">
                                    <span className="text-slate-500">Apps Blocked</span>
                                    <span className="text-white font-medium">{config.blocked_apps.length} Applications</span>
                                </div>
                                <div className="flex justify-between items-center text-sm">
                                    <span className="text-slate-500">Websites Blocked</span>
                                    <span className="text-white font-medium">{config.blocked_sites?.length || 0} Websites</span>
                                </div>
                            </div>

                            {/* Collapsible Details */}
                            {showDetails && (
                                <div className="bg-slate-950/30 px-4 py-3 border-t border-white/5 max-h-40 overflow-y-auto custom-scrollbar">
                                    {config.blocked_apps.length > 0 && (
                                        <div className="mb-3">
                                            <h4 className="text-xs font-bold text-slate-500 uppercase mb-2">Applications</h4>
                                            <div className="flex flex-wrap gap-1">
                                                {config.blocked_apps.map(app => (
                                                    <span key={app} className="text-xs bg-slate-800 text-slate-300 px-2 py-1 rounded border border-white/5">
                                                        {appMap.get(app.toLowerCase())?.name || app}
                                                    </span>
                                                ))}
                                            </div>
                                        </div>
                                    )}

                                    {(config.blocked_sites?.length || 0) > 0 && (
                                        <div>
                                            <h4 className="text-xs font-bold text-slate-500 uppercase mb-2">Websites</h4>
                                            <div className="flex flex-wrap gap-1">
                                                {config.blocked_sites.map(site => (
                                                    <span key={site} className="text-xs bg-slate-800 text-slate-300 px-2 py-1 rounded border border-white/5">
                                                        {site}
                                                    </span>
                                                ))}
                                            </div>
                                        </div>
                                    )}
                                </div>
                            )}

                            <button
                                onClick={() => setShowDetails(!showDetails)}
                                className="w-full py-2 flex items-center justify-center gap-1 text-xs text-slate-500 hover:text-blue-400 hover:bg-white/5 transition-colors border-t border-white/5"
                            >
                                <span>{showDetails ? 'Hide Details' : 'View Blocked List'}</span>
                                <svg className={`w-3 h-3 transition-transform ${showDetails ? 'rotate-180' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                                </svg>
                            </button>
                        </div>

                        <div className="flex gap-3">
                            <button
                                onClick={() => setShowConfirm(false)}
                                className="flex-1 px-4 py-3 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 font-semibold transition-colors text-sm"
                            >
                                Cancel
                            </button>
                            <button
                                onClick={confirmStart}
                                className="flex-1 px-4 py-3 rounded-lg bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-500 hover:to-purple-500 text-white font-bold shadow-lg shadow-blue-900/40 transition-all text-sm"
                            >
                                Start Focus
                            </button>
                        </div>

                        <p className="text-xs text-center text-slate-500 mt-2">
                            Session cannot be cancelled once started.
                        </p>
                    </div>
                </div>
            )}
        </div>
    );
}
