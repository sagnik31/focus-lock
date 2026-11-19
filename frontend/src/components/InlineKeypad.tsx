import { useState, useEffect } from 'react';
import { Delete, ArrowLeft } from 'lucide-react';

interface InlineKeypadProps {
    onStart: (hours: number, minutes: number) => void;
    // Returns to the presets screen.
    onBack?: () => void;
}

export function InlineKeypad({ onStart, onBack }: InlineKeypadProps) {
    const [buffer, setBuffer] = useState("");

    const handleNum = (num: string) => {
        // Up to 4 digits (HHMM); "00" must not push the buffer past that.
        setBuffer(prev => (prev + num).slice(0, 4));
    };

    const handleBackspace = () => {
        setBuffer(prev => prev.slice(0, -1));
    };

    const handleClear = () => {
        setBuffer("");
    }

    // Calculate display
    const padded = buffer.padStart(4, '0');
    const displayHours = padded.slice(0, 2);
    const displayMinutes = padded.slice(2, 4);

    const handleOK = () => {
        const h = parseInt(displayHours, 10);
        const m = parseInt(displayMinutes, 10);
        onStart(h, m);
    };

    // Keyboard support
    useEffect(() => {
        const handleKeyDown = (e: KeyboardEvent) => {
            // Ignore keys typed into text fields elsewhere on the page (e.g.
            // the website box), or digits/Enter there would drive the keypad.
            const t = e.target as HTMLElement | null;
            if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
            if (e.key >= '0' && e.key <= '9') {
                handleNum(e.key);
            } else if (e.key === 'Backspace') {
                handleBackspace();
            } else if (e.key === 'Enter') {
                handleOK();
            } else if (e.key === 'Escape') {
                // Escape clears what was typed; on an empty keypad it goes back.
                if (buffer === "" && onBack) onBack();
                else handleClear();
            }
        };

        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
        // handleOK and the digit handlers read only buffer (or use functional
        // updates); onBack/onStart are App handlers that only call setters.
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [buffer]); // Re-bind when buffer changes to ensure state freshness or use functional updates everywhere (already doing so)

    return (
        <div
            className="w-56 bg-slate-900 border border-white/10 rounded-xl shadow-2xl overflow-hidden flex flex-col shrink-0"
        >
            {/* Header / Display */}
            <div
                className="bg-blue-600 px-4 py-3 flex items-center justify-between relative h-16 shadow-inner cursor-text"
                onClick={() => { /* Visual cue or focus logic if needed */ }}
            >
                <div className="flex items-baseline gap-1 text-white">
                    <span className="text-3xl font-bold tracking-tighter drop-shadow-md">{displayHours}</span>
                    <span className="text-xs font-medium opacity-80 mr-2">h</span>
                    <span className="text-3xl font-bold tracking-tighter drop-shadow-md">{displayMinutes}</span>
                    <span className="text-xs font-medium opacity-80">m</span>
                </div>

                {/* Backspace Button */}
                <button
                    onClick={handleBackspace}
                    className="absolute top-3 right-3 text-white/80 hover:text-white p-1 rounded transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/50"
                    title="Backspace"
                    aria-label="Backspace"
                >
                    <Delete className="w-[18px] h-[18px]" />
                </button>
            </div>

            {/* Keypad */}
            <div
                className="p-3 py-3 text-white bg-slate-900"
                style={{
                    display: 'grid',
                    gridTemplateColumns: 'repeat(3, 1fr)',
                    gap: '0.5rem',
                    justifyItems: 'center',
                    alignItems: 'center'
                }}
            >
                {[1, 2, 3, 4, 5, 6, 7, 8, 9].map(num => (
                    <button
                        key={num}
                        onClick={() => handleNum(num.toString())}
                        className="w-10 h-10 text-lg font-medium rounded-full transition-all flex items-center justify-center hover:bg-white/10 active:bg-white/20 text-slate-200"
                    >
                        {num}
                    </button>
                ))}

                {/* 0 Button - Center column (start at 2) */}
                <button
                    onClick={() => handleNum("0")}
                    className="w-10 h-10 text-lg font-medium rounded-full transition-all flex items-center justify-center hover:bg-white/10 active:bg-white/20 text-slate-200"
                    style={{ gridColumnStart: 2 }}
                >
                    0
                </button>

                <button
                    onClick={() => handleNum("00")}
                    className="w-10 h-10 text-xs font-medium rounded-full transition-all flex items-center justify-center hover:bg-white/10 active:bg-white/20 text-slate-400"
                >
                    00
                </button>
            </div>

            {/* Footer Actions */}
            <div className="flex justify-between items-center px-4 pb-3 pt-0 gap-4 bg-slate-900">
                <div className="flex items-center gap-3">
                    {onBack && (
                        <button
                            onClick={onBack}
                            className="flex items-center gap-1 text-slate-500 font-bold tracking-wider text-[10px] hover:text-slate-300 py-2 transition-colors"
                            aria-label="Back to presets"
                        >
                            <ArrowLeft className="w-3 h-3" /> BACK
                        </button>
                    )}
                    <button
                        onClick={handleClear}
                        className="text-slate-500 font-bold tracking-wider text-[10px] hover:text-slate-300 py-2 transition-colors"
                    >
                        CLEAR
                    </button>
                </div>
                <button
                    onClick={handleOK}
                    className="text-blue-400 font-bold tracking-wider text-[10px] hover:text-blue-300 py-2 transition-colors uppercase"
                >
                    CONFIRM
                </button>
            </div>
        </div>
    );
}
