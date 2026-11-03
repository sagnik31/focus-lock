import { useState, useEffect } from 'react';
import { GetInstalledApps } from "../../wailsjs/go/bridge/App";
import { sysinfo } from "../../wailsjs/go/models";
import { X, RefreshCw } from 'lucide-react';

interface AppSelectorProps {
    isOpen: boolean;
    onClose: () => void;
    onSave: (selectedApps: string[]) => void;
    currentlyBlocked: string[];
    isLocked?: boolean;
}

export function AppSelector({ isOpen, onClose, onSave, currentlyBlocked, isLocked }: AppSelectorProps) {
    const [apps, setApps] = useState<sysinfo.AppInfo[]>([]);
    const [loading, setLoading] = useState(false);
    const [searchTerm, setSearchTerm] = useState("");
    const [selected, setSelected] = useState<Set<string>>(new Set(currentlyBlocked));

    useEffect(() => {
        if (isOpen) {
            // Only load apps if empty
            if (apps.length === 0) {
                setLoading(true);
                GetInstalledApps()
                    .then((result) => {
                        setApps(result);
                        setLoading(false);
                    })
                    .catch((err) => {
                        console.error("Failed to load apps:", err);
                        setLoading(false);
                    });
            }
            // Initialize selection state ONLY when opening
            setSelected(new Set(currentlyBlocked));
        }
    }, [isOpen]); // Removed currentlyBlocked from dependency to prevent reset on poll

    const toggleApp = (exeName: string) => {
        // If locked, prevent unchecking existing apps
        if (isLocked && currentlyBlocked.includes(exeName)) {
            return;
        }

        const newSelected = new Set(selected);
        if (newSelected.has(exeName)) {
            newSelected.delete(exeName);
        } else {
            newSelected.add(exeName);
        }
        setSelected(newSelected);
    };

    const handleSave = () => {
        const appsToSave = Array.from(selected);
        onSave(appsToSave);
        onClose();
    };

    const filteredApps = apps.filter(app =>
        app.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
        app.exe.toLowerCase().includes(searchTerm.toLowerCase())
    );

    if (!isOpen) return null;

    return (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/80 backdrop-blur-sm p-4 fl-overlay-in">
            <div className="bg-slate-900 rounded-2xl shadow-2xl border border-white/10 w-full max-w-[90vw] h-[85vh] flex flex-col fl-modal-in">
                <div className="p-4 border-b border-white/5 flex justify-between items-center">
                    <h2 className="text-xl font-bold text-white">Select apps to block</h2>
                    <button onClick={onClose} aria-label="Close" className="text-slate-400 hover:text-white p-1 rounded-lg hover:bg-slate-800 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/50">
                        <X className="w-5 h-5" />
                    </button>
                </div>

                <div className="p-4 border-b border-white/5 flex gap-2">
                    <input
                        type="text"
                        placeholder="Search apps..."
                        className="flex-1 bg-slate-950/50 border border-slate-700/50 rounded-lg px-4 py-2 focus:border-blue-500/50 focus:ring-2 focus:ring-blue-500/20 outline-none text-white transition-all"
                        value={searchTerm}
                        onChange={(e) => setSearchTerm(e.target.value)}
                    />
                    <button
                        onClick={() => { setApps([]); setLoading(true); GetInstalledApps().then(res => { setApps(res); setLoading(false); }); }}
                        className="px-3 bg-slate-800 hover:bg-slate-700 rounded-lg text-slate-300 flex items-center transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/50"
                        title="Refresh List"
                        aria-label="Refresh list"
                    >
                        <RefreshCw className="w-4 h-4" />
                    </button>
                </div>

                <div className="flex-1 overflow-y-auto p-4 custom-scrollbar">
                    {loading ? (
                        <div className="text-center py-8 text-slate-400">Loading installed applications...</div>
                    ) : (
                        <div className="grid grid-cols-3 gap-4">
                            {filteredApps.map((app) => (
                                <div
                                    key={app.exe}
                                    onClick={() => toggleApp(app.exe)}
                                    className={`flex items-center gap-3 p-3 rounded-lg cursor-pointer transition-colors border ${selected.has(app.exe) ? 'bg-blue-600/20 border-blue-500/50' : 'bg-slate-700/50 hover:bg-slate-700 border-transparent'}`}
                                >
                                    <input
                                        type="checkbox"
                                        checked={selected.has(app.exe)}
                                        readOnly
                                        disabled={isLocked && currentlyBlocked.includes(app.exe)}
                                        className={`w-5 h-5 rounded border-slate-500 bg-slate-700 text-blue-500 focus:ring-0 focus:ring-offset-0 shrink-0 ${isLocked && currentlyBlocked.includes(app.exe) ? 'opacity-50 cursor-not-allowed' : ''}`}
                                    />
                                    {app.icon ? (
                                        <img src={app.icon} alt={app.name} className="w-8 h-8 object-contain shrink-0" />
                                    ) : (
                                        <div className="w-8 h-8 bg-slate-600 rounded flex items-center justify-center text-xs text-white shrink-0">?</div>
                                    )}
                                    <div className="min-w-0 flex-1">
                                        <div className="text-white font-medium truncate" title={app.name}>{app.name}</div>
                                    </div>
                                </div>
                            ))}
                        </div>
                    )}
                </div>

                <div className="p-4 border-t border-white/5 flex justify-end gap-3">
                    <button onClick={onClose} className="px-4 py-2 text-slate-300 hover:text-white rounded-lg transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-slate-500">Cancel</button>
                    <button
                        onClick={handleSave}
                        className="px-6 py-2 bg-blue-600 hover:bg-blue-500 rounded-lg font-semibold text-white transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400"
                    >
                        Save selection ({selected.size})
                    </button>
                </div>
            </div>
        </div>
    );
}
