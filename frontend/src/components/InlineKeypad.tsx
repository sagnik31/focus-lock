import { useState, useEffect } from 'react';

interface InlineKeypadProps {
    onStart: (hours: number, minutes: number) => void;
}

export function InlineKeypad({ onStart }: InlineKeypadProps) {
    const [buffer, setBuffer] = useState("");

    const handleNum = (num: string) => {
        if (buffer.length >= 4) return;
        setBuffer(prev => prev + num);
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
            if (e.key >= '0' && e.key <= '9') {
                handleNum(e.key);
            } else if (e.key === 'Backspace') {
                handleBackspace();
            } else if (e.key === 'Enter') {
                handleOK();
            } else if (e.key === 'Escape') {
                handleClear();
            }
        };

        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [buffer]); // Re-bind when buffer changes to ensure state freshness or use functional updates everywhere (already doing so)

    return (
        <div
            className="w-56 bg-slate-900 border border-white/10 rounded-xl shadow-2xl overflow-hidden flex flex-col shrink-0"
        >
            {/* Header / Display */}
            <div
                className="bg-gradient-to-r from-blue-600 to-purple-600 px-4 py-3 flex items-center justify-between relative h-16 shadow-inner cursor-text"
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
                    className="absolute top-3 right-3 text-white/80 hover:text-white p-1 transition-colors"
                    title="Backspace"
                >
                    <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
                        <path d="M22 3H7c-.69 0-1.23.35-1.59.88L0 12l5.41 8.11c.36.53.9.89 1.59.89h15c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zm-3 12.59L17.59 17 14 13.41 10.41 17 9 15.59 12.59 12 9 8.41 10.41 7 14 10.59 17.59 7 19 8.41 15.41 12 19 15.59z" />
                    </svg>
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
                <button
                    onClick={handleClear}
                    className="text-slate-500 font-bold tracking-wider text-[10px] hover:text-slate-300 py-2 transition-colors"
                >
                    CLEAR
                </button>
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
