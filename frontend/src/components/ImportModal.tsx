import { useState } from "react";

interface ImportModalProps {
    isOpen: boolean;
    onClose: () => void;
    onImport: (jsonContent: string) => Promise<void>;
    onSuccess: () => void;
}

export function ImportModal({ isOpen, onClose, onImport, onSuccess }: ImportModalProps) {
    const [isImporting, setIsImporting] = useState(false);
    const [pasteText, setPasteText] = useState('');
    const [importError, setImportError] = useState('');

    const handleFileImport = async (event: React.ChangeEvent<HTMLInputElement>) => {
        const file = event.target.files?.[0];
        if (!file) return;

        setIsImporting(true);
        setImportError('');
        try {
            const content = await file.text();
            await onImport(content);
            onSuccess();
            onClose();
        } catch (err: any) {
            setImportError(err.toString());
        } finally {
            setIsImporting(false);
            event.target.value = '';
        }
    };

    const handlePasteImport = async () => {
        if (!pasteText.trim()) {
            setImportError('Please paste JSON content');
            return;
        }

        setIsImporting(true);
        setImportError('');
        try {
            await onImport(pasteText);
            onSuccess();
            setPasteText('');
            onClose();
        } catch (err: any) {
            setImportError(err.toString());
        } finally {
            setIsImporting(false);
        }
    };

    if (!isOpen) return null;

    return (
        <div className="fixed inset-0 bg-slate-950/80 backdrop-blur-sm flex items-center justify-center z-50 p-4 animate-in fade-in duration-200">
            <div className="bg-slate-900 border border-white/10 rounded-2xl shadow-2xl max-w-md w-full p-6 space-y-5">
                <div className="text-center space-y-2">
                    <div className="w-12 h-12 bg-blue-500/10 rounded-full flex items-center justify-center mx-auto mb-2 border border-blue-500/20">
                        <svg className="w-6 h-6 text-blue-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
                        </svg>
                    </div>
                    <h3 className="text-lg font-bold text-white">Import Settings</h3>
                    <p className="text-slate-400 text-sm">
                        Import blocked apps, websites, and schedules from a JSON file or paste directly.
                    </p>
                </div>

                {importError && (
                    <div className="bg-red-500/10 border border-red-500/20 text-red-300 px-4 py-3 rounded-lg text-sm">
                        {importError}
                    </div>
                )}

                {/* Option 1: File Upload */}
                <div className="space-y-2">
                    <label className="text-xs font-bold text-slate-500 uppercase tracking-wider">Option 1: Browse File</label>
                    <input
                        type="file"
                        accept=".json"
                        onChange={handleFileImport}
                        disabled={isImporting}
                        className="w-full text-sm text-slate-400 file:mr-4 file:py-2 file:px-4 file:rounded-lg file:border-0 file:text-xs file:font-bold file:bg-blue-600 file:text-white hover:file:bg-blue-500 file:cursor-pointer file:transition-colors bg-slate-800/50 rounded-lg border border-slate-700/50 cursor-pointer"
                    />
                </div>

                <div className="flex items-center gap-3">
                    <div className="flex-1 h-px bg-slate-700/50"></div>
                    <span className="text-xs text-slate-500 font-bold uppercase">or</span>
                    <div className="flex-1 h-px bg-slate-700/50"></div>
                </div>

                {/* Option 2: Paste JSON */}
                <div className="space-y-2">
                    <label className="text-xs font-bold text-slate-500 uppercase tracking-wider">Option 2: Paste JSON</label>
                    <textarea
                        value={pasteText}
                        onChange={(e) => setPasteText(e.target.value)}
                        placeholder='{"blocked": {"apps": [...], "sites": [...]}, "schedules": [...]}'
                        disabled={isImporting}
                        className="w-full h-32 bg-slate-800/50 border border-slate-700/50 focus:border-blue-500/50 rounded-lg px-4 py-3 text-sm text-slate-200 placeholder:text-slate-600 focus:ring-2 focus:ring-blue-500/20 outline-none resize-none font-mono"
                    />
                    <button
                        onClick={handlePasteImport}
                        disabled={isImporting || !pasteText.trim()}
                        className="w-full py-2.5 rounded-lg bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-500 hover:to-purple-500 text-white font-bold text-sm transition-all disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2"
                    >
                        {isImporting ? (
                            <>
                                <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
                                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                                </svg>
                                Importing...
                            </>
                        ) : (
                            'Import from Paste'
                        )}
                    </button>
                </div>

                <button
                    onClick={() => { onClose(); setPasteText(''); setImportError(''); }}
                    className="w-full py-2.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 font-semibold text-sm transition-colors"
                >
                    Cancel
                </button>
            </div>
        </div>
    );
}
