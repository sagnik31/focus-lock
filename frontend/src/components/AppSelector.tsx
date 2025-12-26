import { useState, useEffect } from 'react';
import { GetInstalledApps } from "../../wailsjs/go/main/App";
import { sysinfo } from "../../wailsjs/go/models";

interface AppSelectorProps {
    isOpen: boolean;
    onClose: () => void;
    onSave: (selectedApps: string[]) => void;
    currentlyBlocked: string[];
}

export function AppSelector({ isOpen, onClose, onSave, currentlyBlocked }: AppSelectorProps) {
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
        console.log("AppSelector saving:", appsToSave);
        onSave(appsToSave);
        onClose();
    };

    const filteredApps = apps.filter(app =>
        app.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
        app.exe.toLowerCase().includes(searchTerm.toLowerCase())
    );

    if (!isOpen) return null;

    return (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm p-4">
            <div className="bg-slate-800 rounded-xl shadow-2xl border border-slate-700 w-full max-w-2xl max-h-[80vh] flex flex-col">
                <div className="p-4 border-b border-slate-700 flex justify-between items-center">
                    <h2 className="text-xl font-bold text-white">Select Apps to Block</h2>
                    <button onClick={onClose} className="text-slate-400 hover:text-white">✕</button>
                </div>

                <div className="p-4 border-b border-slate-700 flex gap-2">
                    <input
                        type="text"
                        placeholder="Search apps..."
                        className="flex-1 bg-slate-900 border border-slate-600 rounded-lg px-4 py-2 focus:ring-2 focus:ring-blue-500 outline-none text-white"
                        value={searchTerm}
                        onChange={(e) => setSearchTerm(e.target.value)}
                    />
                    <button
                        onClick={() => { setApps([]); setLoading(true); GetInstalledApps().then(res => { setApps(res); setLoading(false); }); }}
                        className="px-3 bg-slate-700 hover:bg-slate-600 rounded-lg text-slate-300"
                        title="Refresh List"
                    >
                        ↻
                    </button>
                </div>

                <div className="flex-1 overflow-y-auto p-2 space-y-1">
                    {loading ? (
                        <div className="text-center py-8 text-slate-400">Loading installed applications...</div>
                    ) : (
                        filteredApps.map((app) => (
                            <div
                                key={app.exe}
                                onClick={() => toggleApp(app.exe)}
                                className={`flex items-center gap-3 p-3 rounded-lg cursor-pointer transition-colors ${selected.has(app.exe) ? 'bg-blue-600/20 border border-blue-500/50' : 'hover:bg-slate-700 border border-transparent'}`}
                            >
                                <input
                                    type="checkbox"
                                    checked={selected.has(app.exe)}
                                    readOnly
                                    className="w-5 h-5 rounded border-slate-500 bg-slate-700 text-blue-500 focus:ring-0 focus:ring-offset-0"
                                />
                                {app.icon ? (
                                    <img src={app.icon} alt={app.name} className="w-8 h-8 object-contain" />
                                ) : (
                                    <div className="w-8 h-8 bg-slate-600 rounded flex items-center justify-center text-xs text-white">?</div>
                                )}
                                <div>
                                    <div className="text-white font-medium">{app.name}</div>
                                    {/* <div className="text-slate-400 text-xs font-mono">{app.exe}</div> */}
                                </div>
                            </div>
                        ))
                    )}
                </div>

                <div className="p-4 border-t border-slate-700 flex justify-end gap-3">
                    <button onClick={onClose} className="px-4 py-2 text-slate-300 hover:text-white">Cancel</button>
                    <button
                        onClick={handleSave}
                        className="px-6 py-2 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-500 hover:to-purple-500 rounded-lg font-bold text-white"
                    >
                        Save Selection ({selected.size})
                    </button>
                </div>
            </div>
        </div>
    );
}
