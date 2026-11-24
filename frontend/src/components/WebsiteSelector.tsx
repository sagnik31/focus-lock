import React, { useState } from 'react';
import { Plus, Trash2, Check, Pencil, X, History, FolderPlus } from 'lucide-react';
import { storage } from '../../wailsjs/go/models';

interface WebsiteSelectorProps {
    sites: string[];
    onAdd: (url: string) => void;
    onRemove: (url: string) => void;
    onAddSites: (urls: string[]) => void;
    onRemoveSites: (urls: string[]) => void;
    blockVPN: boolean;
    onToggleVPN: (val: boolean) => void;
    isLocked?: boolean;

    // Recently used + custom groups
    recentSites: string[];
    onForgetRecent: (url: string) => void;
    siteGroups: storage.SiteGroup[];
    onSaveGroups: (groups: storage.SiteGroup[]) => Promise<void>;
}

const SUGGESTIONS: Record<string, string[]> = {
    "Social Media": ["facebook.com", "instagram.com", "twitter.com", "tiktok.com", "reddit.com", "linkedin.com"],
    "Entertainment": ["youtube.com", "netflix.com", "twitch.tv", "hulu.com", "disneyplus.com"],
    "Gaming": ["steamcommunity.com", "roblox.com", "epicgames.com", "battle.net"],
    "Adult": ["pornhub.com", "xvideos.com", "xnxx.com", "xhamster.com", "onlyfans.com"]
};

// How many "Recently used" chips to show.
const RECENT_LIMIT = 10;

// Splits free text ("a.com, b.com\nc.com") into a clean, de-duplicated list.
const parseSites = (text: string): string[] => {
    const out: string[] = [];
    for (const raw of text.split(/[\s,;]+/)) {
        const s = raw.trim().toLowerCase();
        if (s && !out.includes(s)) out.push(s);
    }
    return out;
};

const newId = () =>
    (typeof crypto !== 'undefined' && 'randomUUID' in crypto)
        ? crypto.randomUUID()
        : `g-${Date.now()}-${Math.random().toString(36).slice(2)}`;

const Favicon: React.FC<{ site: string; className?: string }> = ({ site, className }) => (
    <img
        src={`https://www.google.com/s2/favicons?domain=${site}&sz=16`}
        alt=""
        className={className}
        onError={(e) => (e.currentTarget.style.display = 'none')}
    />
);

// Same switch used by built-in categories and custom groups.
const Toggle: React.FC<{ on: boolean; onClick: () => void; label: string; disabled?: boolean }> = ({ on, onClick, label, disabled }) => (
    <button
        onClick={onClick}
        disabled={disabled}
        className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus-visible:ring-2 focus-visible:ring-white focus-visible:ring-opacity-75 disabled:opacity-40 disabled:cursor-not-allowed ${on ? 'bg-blue-600' : 'bg-slate-700'}`}
    >
        <span className="sr-only">Toggle {label}</span>
        <span
            aria-hidden="true"
            className={`${on ? 'translate-x-4' : 'translate-x-0'} pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-lg ring-0 transition duration-200 ease-in-out`}
        />
    </button>
);

// One-click site chips, shared by groups and suggestions.
const SiteChips: React.FC<{ items: string[]; sites: string[]; onAdd: (url: string) => void }> = ({ items, sites, onAdd }) => (
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
                    <Favicon site={site} className={`w-3 h-3 ${isBlocked ? 'opacity-50' : 'opacity-30'}`} />
                    {site}
                    {isBlocked && <Check className="w-3 h-3 text-blue-400" />}
                </button>
            );
        })}
    </div>
);

// Inline create/edit form for a custom group.
const GroupEditor: React.FC<{
    initial?: storage.SiteGroup;
    onSave: (name: string, sites: string[]) => Promise<void>;
    onCancel: () => void;
}> = ({ initial, onSave, onCancel }) => {
    const [name, setName] = useState(initial?.name ?? '');
    const [text, setText] = useState((initial?.sites ?? []).join('\n'));
    const [err, setErr] = useState('');
    const [saving, setSaving] = useState(false);

    const submit = async () => {
        if (!name.trim()) { setErr('Give the group a name.'); return; }
        const list = parseSites(text);
        if (list.length === 0) { setErr('Add at least one website.'); return; }
        setSaving(true);
        try {
            await onSave(name.trim(), list);
        } catch (e) {
            setErr(String(e));
            setSaving(false);
        }
    };

    return (
        <div className="bg-slate-800/40 border border-blue-500/30 rounded-xl p-3 space-y-2">
            <input
                autoFocus
                type="text"
                placeholder="Group name, e.g. Work distractions"
                value={name}
                onChange={(e) => { setName(e.target.value); setErr(''); }}
                onKeyDown={(e) => e.key === 'Escape' && onCancel()}
                className="w-full bg-slate-950/50 border border-slate-700/50 focus:border-blue-500/50 rounded-lg px-3 py-2 text-sm outline-none placeholder:text-slate-500"
            />
            <textarea
                placeholder={"Websites, one per line or comma-separated\nnews.ycombinator.com, bbc.com"}
                value={text}
                rows={3}
                onChange={(e) => { setText(e.target.value); setErr(''); }}
                onKeyDown={(e) => e.key === 'Escape' && onCancel()}
                className="w-full bg-slate-950/50 border border-slate-700/50 focus:border-blue-500/50 rounded-lg px-3 py-2 text-sm outline-none placeholder:text-slate-500 resize-none custom-scrollbar"
            />
            {err && <p className="text-xs text-red-300">{err}</p>}
            <div className="flex justify-end gap-2">
                <button onClick={onCancel} className="px-3 py-1.5 rounded-lg text-xs font-semibold text-slate-400 hover:text-slate-200 hover:bg-white/5">
                    Cancel
                </button>
                <button
                    onClick={submit}
                    disabled={saving}
                    className="px-3 py-1.5 rounded-lg text-xs font-semibold bg-blue-600 hover:bg-blue-500 text-white disabled:opacity-50"
                >
                    {initial ? 'Save group' : 'Create group'}
                </button>
            </div>
        </div>
    );
};

export const WebsiteSelector: React.FC<WebsiteSelectorProps> = ({
    sites,
    onAdd,
    onRemove,
    onAddSites,
    onRemoveSites,
    blockVPN,
    onToggleVPN,
    isLocked,
    recentSites,
    onForgetRecent,
    siteGroups,
    onSaveGroups,
}) => {
    const [input, setInput] = useState("");
    const [groupError, setGroupError] = useState("");
    // null = no editor open; 'new' = creating; otherwise the id being edited.
    const [editing, setEditing] = useState<string | null>(null);

    const handleSubmit = () => {
        if (!input.trim()) return;
        onAdd(input.trim());
        setInput("");
    };

    // Turning a category on adds its missing sites; turning it off removes
    // them (refused while locked: a session can only ever be strengthened).
    const toggleSet = (items: string[]) => {
        const allBlocked = items.length > 0 && items.every(site => sites.includes(site));
        if (allBlocked) {
            if (!isLocked) {
                const toRemove = items.filter(site => sites.includes(site));
                if (toRemove.length > 0) onRemoveSites(toRemove);
            }
        } else {
            const toAdd = items.filter(site => !sites.includes(site));
            if (toAdd.length > 0) onAddSites(toAdd);
        }
    };

    const saveGroup = async (id: string | null, name: string, groupSites: string[]) => {
        const next = id
            ? siteGroups.map(g => g.id === id ? { ...g, name, sites: groupSites } : g)
            : [...siteGroups, { id: newId(), name, sites: groupSites }];
        await onSaveGroups(next as storage.SiteGroup[]);
        setEditing(null);
    };

    const deleteGroup = (id: string) => {
        setGroupError("");
        onSaveGroups(siteGroups.filter(g => g.id !== id)).catch((e) => setGroupError("Could not delete the group: " + String(e)));
    };

    const recent = recentSites.filter(s => !sites.includes(s)).slice(0, RECENT_LIMIT);

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
            <div className="flex-1 overflow-y-auto pr-2 space-y-6 custom-scrollbar min-h-0 pb-4">

                {/* My Groups — user-defined sections take precedence over built-in suggestions */}
                <div className="space-y-3">
                    <div className="flex items-center justify-between">
                        <h4 className="text-xs font-bold text-slate-400 uppercase tracking-widest">My Groups</h4>
                        {editing !== 'new' && (
                            <button
                                onClick={() => setEditing('new')}
                                className="flex items-center gap-1 text-[11px] font-semibold text-blue-400 hover:text-blue-300"
                            >
                                <FolderPlus className="w-3.5 h-3.5" /> New group
                            </button>
                        )}
                    </div>

                    {editing === 'new' && (
                        <GroupEditor onSave={(n, s) => saveGroup(null, n, s)} onCancel={() => setEditing(null)} />
                    )}

                    {groupError && <p role="alert" className="text-xs text-red-300">{groupError}</p>}

                    {siteGroups.length === 0 && editing !== 'new' && (
                        <p className="text-xs text-slate-500">
                            Create your own sections of websites to block together in one click.
                        </p>
                    )}

                    {siteGroups.map(group => editing === group.id ? (
                        <GroupEditor
                            key={group.id}
                            initial={group}
                            onSave={(n, s) => saveGroup(group.id, n, s)}
                            onCancel={() => setEditing(null)}
                        />
                    ) : (
                        <div key={group.id} className="group/g">
                            <div className="flex items-center justify-between mb-2">
                                <div className="flex items-center gap-2 min-w-0">
                                    <span className="text-xs font-bold text-slate-300 uppercase tracking-widest truncate">{group.name}</span>
                                    <button
                                        onClick={() => setEditing(group.id)}
                                        className="opacity-0 group-hover/g:opacity-100 focus-visible:opacity-100 text-slate-500 hover:text-slate-200 p-1 rounded"
                                        aria-label={`Edit ${group.name}`}
                                        title="Edit group"
                                    >
                                        <Pencil className="w-3 h-3" />
                                    </button>
                                    <button
                                        onClick={() => deleteGroup(group.id)}
                                        className="opacity-0 group-hover/g:opacity-100 focus-visible:opacity-100 text-slate-500 hover:text-red-400 p-1 rounded"
                                        aria-label={`Delete ${group.name}`}
                                        title="Delete group (does not unblock its sites)"
                                    >
                                        <Trash2 className="w-3 h-3" />
                                    </button>
                                </div>
                                <Toggle
                                    on={group.sites.length > 0 && group.sites.every(s => sites.includes(s))}
                                    onClick={() => toggleSet(group.sites)}
                                    label={group.name}
                                    disabled={group.sites.length === 0}
                                />
                            </div>
                            <SiteChips items={group.sites} sites={sites} onAdd={onAdd} />
                        </div>
                    ))}
                </div>

                {/* Recently used — previously blocked sites that are not blocked now */}
                {recent.length > 0 && (
                    <div>
                        <h4 className="text-xs font-bold text-slate-400 uppercase tracking-widest mb-3 flex items-center gap-1.5">
                            <History className="w-3.5 h-3.5" /> Recently Used
                        </h4>
                        <div className="flex flex-wrap gap-2">
                            {recent.map(site => (
                                <div
                                    key={site}
                                    className="flex items-center rounded-lg text-xs font-medium border bg-slate-800/40 border-slate-700/50 text-slate-400 hover:border-slate-600 transition-all"
                                >
                                    <button
                                        onClick={() => onAdd(site)}
                                        className="flex items-center gap-2 pl-3 pr-1.5 py-2 hover:text-slate-200"
                                        title={`Block ${site}`}
                                    >
                                        <Favicon site={site} className="w-3 h-3 opacity-40" />
                                        {site}
                                        <Plus className="w-3 h-3 text-blue-400/70" />
                                    </button>
                                    <button
                                        onClick={() => onForgetRecent(site)}
                                        className="pr-2 pl-1 py-2 text-slate-600 hover:text-slate-300"
                                        aria-label={`Remove ${site} from recently used`}
                                        title="Remove from recently used"
                                    >
                                        <X className="w-3 h-3" />
                                    </button>
                                </div>
                            ))}
                        </div>
                    </div>
                )}

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

                {/* Built-in suggestions */}
                {Object.entries(SUGGESTIONS).map(([category, items]) => (
                    <div key={category}>
                        <div className="flex items-center justify-between mb-3">
                            <h4 className="text-xs font-bold text-slate-500 uppercase tracking-widest">{category}</h4>
                            <Toggle
                                on={items.every(site => sites.includes(site))}
                                onClick={() => toggleSet(items)}
                                label={category}
                            />
                        </div>
                        <SiteChips items={items} sites={sites} onAdd={onAdd} />
                    </div>
                ))}
            </div>
        </div>
    );
};
