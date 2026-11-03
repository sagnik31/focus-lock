import React, { useState } from 'react';
import { Plus, Trash2, Check } from 'lucide-react';

interface WebsiteSelectorProps {
    sites: string[];
    onAdd: (url: string) => void;
    onRemove: (url: string) => void;
    onAddSites: (urls: string[]) => void;
    onRemoveSites: (urls: string[]) => void;
    blockVPN: boolean;
    onToggleVPN: (val: boolean) => void;
    isLocked?: boolean;
}

export const WebsiteSelector: React.FC<WebsiteSelectorProps> = ({
    sites,
    onAdd,
    onRemove,
    onAddSites,
    onRemoveSites,
    blockVPN,
    onToggleVPN,
    isLocked
}) => {
    const [input, setInput] = useState("");

    const SUGGESTIONS = {
        "Social Media": ["facebook.com", "instagram.com", "twitter.com", "tiktok.com", "reddit.com", "linkedin.com"],
        "Entertainment": ["youtube.com", "netflix.com", "twitch.tv", "hulu.com", "disneyplus.com"],
        "Gaming": ["steamcommunity.com", "roblox.com", "epicgames.com", "battle.net"],
        "Adult": ["pornhub.com", "xvideos.com", "xnxx.com", "xhamster.com", "onlyfans.com"]
    };

    const handleSubmit = () => {
        if (!input.trim()) return;
        onAdd(input.trim());
        setInput("");
    };

    return (
        <div className="flex flex-col h-full overflow-hidden">
            {/* VPN Toggle */}
            <div className="mb-4 bg-slate-800/50 p-3 rounded-xl border border-white/5 flex items-center justify-between shrink-0">
                <div className="flex flex-col">
                    <span className="text-sm font-medium text-slate-200">Block VPNs</span>
                    <span className="text-xs text-slate-500">Prevent bypass tools</span>
                </div>
                <button
                    onClick={() => onToggleVPN(!blockVPN)}
                    className={`w-12 h-6 rounded-full transition-colors relative ${blockVPN ? 'bg-blue-600' : 'bg-slate-700'}`}
                >
                    <div className={`absolute top-1 w-4 h-4 rounded-full bg-white transition-transform ${blockVPN ? 'left-7' : 'left-1'}`} />
                </button>
            </div>

            {/* Add Input */}
            <div className="relative mb-4 group shrink-0">
                <input
                    type="text"
                    placeholder="Add e.g. facebook.com"
                    value={input}
                    onChange={(e) => setInput(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && handleSubmit()}
                    className="w-full bg-slate-950/50 border border-slate-700/50 focus:border-blue-500/50 rounded-xl px-4 py-3 pl-10 focus:ring-2 focus:ring-blue-500/20 outline-none transition-all placeholder:text-slate-500 text-sm"
                />
                <Plus className="w-5 h-5 text-slate-500 absolute left-3 top-3 group-focus-within:text-blue-400 transition-colors" />
            </div>

            {/* Sites List */}
            <div className="flex-1 overflow-y-auto pr-2 space-y-6 custom-scrollbar min-h-0">

                {/* Current Blocked List */}
                {sites.length > 0 && (
                    <div className="space-y-2">
                        <h4 className="text-xs font-bold text-slate-400 uppercase tracking-widest mb-2 sticky top-0 bg-slate-900/90 backdrop-blur py-2 z-10">Blocked Websites ({sites.length})</h4>
                        {sites.map((site) => (
                            <div key={site} className="group flex items-center justify-between bg-slate-800/40 hover:bg-slate-800/80 p-3 rounded-lg border border-white/5 hover:border-white/20 transition-all">
                                <div className="flex items-center gap-3 overflow-hidden">
                                    <div className="w-8 h-8 bg-slate-700/50 rounded-lg flex items-center justify-center text-slate-300">
                                        <img
                                            src={`https://www.google.com/s2/favicons?domain=${site}&sz=64`}
                                            alt={site}
                                            className="w-5 h-5 opacity-70 group-hover:opacity-100 transition-opacity"
                                            onError={(e) => (e.currentTarget.style.display = 'none')}
                                        />
                                    </div>
                                    <span className="font-medium text-slate-200 text-sm truncate group-hover:text-white transition-colors">
                                        {site}
                                    </span>
                                </div>
                                {!isLocked && (
                                    <button
                                        onClick={() => onRemove(site)}
                                        className="opacity-0 group-hover:opacity-100 focus-visible:opacity-100 text-slate-400 hover:text-red-400 p-2 hover:bg-red-400/10 rounded-lg transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-400/50"
                                        title="Remove"
                                        aria-label={`Remove ${site}`}
                                    >
                                        <Trash2 className="w-4 h-4" />
                                    </button>
                                )}
                            </div>
                        ))}
                    </div>
                )}

                {/* Suggestions */}
                <div className="space-y-6 pb-4">
                    {Object.entries(SUGGESTIONS).map(([category, items]) => {
                        const areAllBlocked = items.every(site => sites.includes(site));

                        const toggleCategory = () => {
                            if (areAllBlocked) {
                                // Remove all - only if not locked
                                if (!isLocked) {
                                    const toRemove = items.filter(site => sites.includes(site));
                                    if (toRemove.length > 0) onRemoveSites(toRemove);
                                }
                            } else {
                                // Add missing
                                const toAdd = items.filter(site => !sites.includes(site));
                                if (toAdd.length > 0) onAddSites(toAdd);
                            }
                        };

                        return (
                            <div key={category}>
                                <div className="flex items-center justify-between mb-3">
                                    <h4 className="text-xs font-bold text-slate-500 uppercase tracking-widest flex items-center gap-2">
                                        {category}
                                    </h4>
                                    <button
                                        onClick={toggleCategory}
                                        className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus-visible:ring-2  focus-visible:ring-white focus-visible:ring-opacity-75 ${areAllBlocked ? 'bg-blue-600' : 'bg-slate-700'
                                            }`}
                                    >
                                        <span className="sr-only">Toggle {category}</span>
                                        <span
                                            aria-hidden="true"
                                            className={`${areAllBlocked ? 'translate-x-4' : 'translate-x-0'
                                                } pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-lg ring-0 transition duration-200 ease-in-out`}
                                        />
                                    </button>
                                </div>
                                <div className="flex flex-wrap gap-2">
                                    {items.map(site => {
                                        const isBlocked = sites.includes(site);
                                        return (
                                            <button
                                                key={site}
                                                onClick={() => !isBlocked && onAdd(site)}
                                                disabled={isBlocked}
                                                className={`flex items-center gap-2 px-3 py-2 rounded-lg text-xs font-medium transition-all border ${isBlocked
                                                    ? 'bg-blue-600/10 border-blue-500/30 text-blue-300 cursor-default'
                                                    : 'bg-slate-800/40 border-slate-700/50 text-slate-400 hover:bg-slate-800 hover:text-slate-200 hover:border-slate-600'
                                                    }`}
                                            >
                                                <img
                                                    src={`https://www.google.com/s2/favicons?domain=${site}&sz=16`}
                                                    alt=""
                                                    className={`w-3 h-3 ${isBlocked ? 'opacity-50' : 'opacity-30 group-hover:opacity-70'}`}
                                                    onError={(e) => (e.currentTarget.style.display = 'none')}
                                                />
                                                {site}
                                                {isBlocked && (
                                                    <Check className="w-3 h-3 text-blue-400" />
                                                )}
                                                {!isBlocked && <span className="opacity-0 hover:opacity-100">+</span>}
                                            </button>
                                        );
                                    })}
                                </div>
                            </div>
                        );
                    })}
                </div>
            </div>
        </div>
    );
};
