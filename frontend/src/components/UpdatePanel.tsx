import { CheckForUpdate, DownloadUpdate, InstallUpdate } from "../../wailsjs/go/bridge/App";
import { BrowserOpenURL } from "../../wailsjs/runtime/runtime";
import { X, RefreshCw, Download, Rocket, CircleCheck, AlertTriangle, Lock, ExternalLink } from 'lucide-react';
import { UpdateState, RELEASES_URL, formatReleaseDate } from './updates';

interface UpdatePanelProps {
    state: UpdateState | null;
    onState: (raw: string) => void;
    // The UI's own view of the session; the backend re-checks on install.
    isLocked: boolean;
    onClose: () => void;
}

const btnBase = "px-4 py-2 rounded-lg text-sm font-semibold transition-all flex items-center gap-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/50 disabled:opacity-50 disabled:cursor-not-allowed";
const btnPrimary = `${btnBase} bg-blue-600 hover:bg-blue-500 text-white`;
const btnSecondary = `${btnBase} bg-slate-800 border border-slate-700 text-slate-200 hover:bg-slate-700`;

export function UpdatePanel({ state, onState, isLocked, onClose }: UpdatePanelProps) {
    const run = (fn: () => Promise<string>) => () => { fn().then(onState).catch(console.error); };

    const phase = state?.phase ?? 'idle';
    const busy = phase === 'checking' || phase === 'downloading' || phase === 'installing';
    // isLocked is polled from the backend; state.sessionActive is only as fresh
    // as the last update event and would keep Install disabled after a session.
    const sessionActive = isLocked;
    const canDownload = !!state?.available && !state.downloaded && !busy;
    const canInstall = !!state?.available && !!state.downloaded && !busy;

    return (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
            <div className="bg-slate-900 border border-white/10 rounded-2xl shadow-2xl w-full max-w-lg max-h-[85vh] overflow-y-auto">
                <div className="flex items-center justify-between px-6 py-4 border-b border-white/10 sticky top-0 bg-slate-900 z-10">
                    <h2 className="text-lg font-bold text-slate-100 flex items-center gap-2">
                        <RefreshCw className="w-5 h-5 text-blue-400" />
                        Updates
                    </h2>
                    <button
                        onClick={onClose}
                        aria-label="Close updates"
                        className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-white/10 transition-colors"
                    >
                        <X className="w-5 h-5" />
                    </button>
                </div>

                <div className="px-6 py-5 space-y-5 text-sm">
                    <div className="flex items-center justify-between">
                        <div>
                            <p className="text-slate-400">Installed version</p>
                            <p className="text-slate-100 font-mono">{state?.current ?? '…'}</p>
                        </div>
                        {state?.lastChecked && (
                            <p className="text-xs text-slate-500">Last checked {formatReleaseDate(state.lastChecked)}</p>
                        )}
                    </div>

                    {state?.dev && (
                        <p className="text-slate-400">
                            This is a development build, so it doesn't check for updates. Release builds update from GitHub.
                        </p>
                    )}

                    {phase === 'checking' && <p className="text-slate-300">Checking for updates…</p>}

                    {phase === 'upToDate' && (
                        <p className="text-green-400 flex items-center gap-2">
                            <CircleCheck className="w-4 h-4" /> You're on the latest version.
                        </p>
                    )}

                    {state?.available && (
                        <div className="rounded-xl border border-blue-500/30 bg-blue-500/10 p-4 space-y-2">
                            <p className="text-blue-200 font-semibold">
                                Version <span className="font-mono">{state.latest}</span> is available
                                {state.publishedAt && <span className="text-blue-300/70 font-normal"> · {formatReleaseDate(state.publishedAt)}</span>}
                            </p>
                            {state.notes && (
                                <pre className="whitespace-pre-wrap font-sans text-xs text-slate-300 max-h-48 overflow-y-auto">{state.notes}</pre>
                            )}
                            <button
                                onClick={() => BrowserOpenURL(RELEASES_URL)}
                                className="text-xs text-blue-300 hover:text-blue-200 flex items-center gap-1"
                            >
                                <ExternalLink className="w-3 h-3" /> View on GitHub
                            </button>
                        </div>
                    )}

                    {phase === 'downloading' && (
                        <div className="space-y-1">
                            <p className="text-slate-300">Downloading… {state?.percent ?? 0}%</p>
                            <div className="h-2 rounded-full bg-slate-800 overflow-hidden">
                                <div className="h-full bg-blue-500 transition-all" style={{ width: `${state?.percent ?? 0}%` }} />
                            </div>
                        </div>
                    )}

                    {state?.downloaded && phase !== 'installing' && (
                        <p className="text-green-400 flex items-center gap-2">
                            <CircleCheck className="w-4 h-4" /> Downloaded and verified.
                        </p>
                    )}

                    {phase === 'installing' && (
                        <p className="text-slate-300">
                            Installing the update… Focus Lock will close and reopen on the new version. If Windows asks for permission, approve it.
                        </p>
                    )}

                    {state?.error && (
                        <div className="bg-red-500/10 border border-red-500/20 text-red-200 px-4 py-3 rounded-xl flex items-start gap-3">
                            <AlertTriangle className="w-4 h-4 text-red-400 shrink-0 mt-0.5" />
                            <span>{state.error}</span>
                        </div>
                    )}

                    {state?.available && sessionActive && (
                        <p className="text-amber-300 flex items-center gap-2">
                            <Lock className="w-4 h-4" /> Available when your session ends. You can download it now.
                        </p>
                    )}

                    <div className="flex flex-wrap gap-2 pt-1">
                        <button className={btnSecondary} disabled={busy || !!state?.dev} onClick={run(CheckForUpdate)}>
                            <RefreshCw className={`w-4 h-4 ${phase === 'checking' ? 'animate-spin' : ''}`} />
                            Check for updates
                        </button>
                        {canDownload && (
                            <button className={btnPrimary} onClick={run(DownloadUpdate)}>
                                <Download className="w-4 h-4" /> Download
                            </button>
                        )}
                        {(canInstall || phase === 'installing') && (
                            <button
                                className={btnPrimary}
                                disabled={sessionActive || phase === 'installing'}
                                title={sessionActive ? "Available when your session ends" : undefined}
                                onClick={run(InstallUpdate)}
                            >
                                <Rocket className="w-4 h-4" /> Install and restart
                            </button>
                        )}
                    </div>
                </div>
            </div>
        </div>
    );
}
