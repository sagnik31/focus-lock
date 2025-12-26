import { useState, useEffect } from 'react';

interface KeypadModalProps {
    isOpen: boolean;
    onClose: () => void;
    onSave: (hours: number, minutes: number) => void;
    initialHours: number;
    initialMinutes: number;
}

export function KeypadModal({ isOpen, onClose, onSave, initialHours, initialMinutes }: KeypadModalProps) {
    const [buffer, setBuffer] = useState("");

    useEffect(() => {
        if (isOpen) {
            // Reset buffer on open, or set to current value?
            // User likely wants to type a new value, so starting empty or with current value is a choice.
            // "The time selection should not be text fields" - typically suggests a fresh entry or easy edit.
            // Let's start empty to make typing "2" result in "00h 02m" immediately rather than appending to existing.
            // But if they just want to fix a mistake?
            // Use case: Defaults to 00 00.
            // If I passed 0, 0 -> buffer empty.
            if (initialHours === 0 && initialMinutes === 0) {
                setBuffer("");
            } else {
                const h = initialHours.toString().padStart(2, '0');
                const m = initialMinutes.toString().padStart(2, '0');
                setBuffer((h + m).replace(/^0+/, '')); // Remove leading zeros for buffer so "01" becomes "1"
            }
        }
    }, [isOpen, initialHours, initialMinutes]);

    if (!isOpen) return null;

    const handleNum = (num: string) => {
        if (buffer.length >= 4) return;
        setBuffer(prev => prev + num);
    };

    const handleBackspace = () => {
        setBuffer(prev => prev.slice(0, -1));
    };

    // Calculate display
    const padded = buffer.padStart(4, '0');
    const displayHours = padded.slice(0, 2);
    const displayMinutes = padded.slice(2, 4);

    const handleSaveClick = () => {
        const h = parseInt(displayHours, 10);
        const m = parseInt(displayMinutes, 10);
        onSave(h, m);
    };

    return (
        <div
            className="fixed inset-0 z-[9999] bg-black/80 backdrop-blur-sm"
            style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                height: '100vh',
                width: '100vw',
                position: 'fixed',
                top: 0,
                left: 0
            }}
        >
            <div
                className="w-full max-w-sm bg-[#333333] rounded-lg shadow-2xl overflow-hidden flex flex-col"
                style={{ maxHeight: '90vh' }}
            >
                <div className="bg-[#D32F2F] p-6 flex items-center justify-between relative h-32">
                    <div className="absolute bottom-1 left-2 text-[10px] text-white/20">v2</div>
                    <div className="flex items-baseline gap-1 text-white">
                        <span className="text-6xl font-normal tracking-tight">{displayHours}</span>
                        <span className="text-xl font-normal opacity-80 mr-4">h</span>
                        <span className="text-6xl font-normal tracking-tight">{displayMinutes}</span>
                        <span className="text-xl font-normal opacity-80">m</span>
                    </div>

                    {/* Backspace Button */}
                    <button
                        onClick={handleBackspace}
                        className="absolute top-4 right-16 text-white/90 hover:text-white p-2"
                    >
                        <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="currentColor">
                            <path d="M22 3H7c-.69 0-1.23.35-1.59.88L0 12l5.41 8.11c.36.53.9.89 1.59.89h15c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zm-3 12.59L17.59 17 14 13.41 10.41 17 9 15.59 12.59 12 9 8.41 10.41 7 14 10.59 17.59 7 19 8.41 15.41 12 19 15.59z" />
                        </svg>
                    </button>

                    {/* Close (X) */}
                    <button
                        onClick={onClose}
                        className="absolute top-4 right-4 text-white/90 hover:text-white p-2"
                    >
                        <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                            <line x1="18" y1="6" x2="6" y2="18"></line>
                            <line x1="6" y1="6" x2="18" y2="18"></line>
                        </svg>
                    </button>
                </div>

                {/* Keypad */}
                <div
                    className="p-4 py-6 text-white"
                    style={{
                        display: 'grid',
                        gridTemplateColumns: 'repeat(3, 1fr)',
                        gap: '1rem',
                        justifyItems: 'center',
                        alignItems: 'center'
                    }}
                >
                    {[1, 2, 3, 4, 5, 6, 7, 8, 9].map(num => (
                        <button
                            key={num}
                            onClick={() => handleNum(num.toString())}
                            className="w-16 h-16 text-3xl font-normal rounded-full transition-colors flex items-center justify-center hover:bg-white/10 active:bg-white/20"
                        >
                            {num}
                        </button>
                    ))}

                    {/* 0 Button - Center column (start at 2) */}
                    <button
                        onClick={() => handleNum("0")}
                        className="w-16 h-16 text-3xl font-normal rounded-full transition-colors flex items-center justify-center hover:bg-white/10 active:bg-white/20"
                        style={{ gridColumnStart: 2 }}
                    >
                        0
                    </button>

                    <button
                        onClick={() => handleNum("00")}
                        className="w-16 h-16 text-2xl font-normal rounded-full transition-colors flex items-center justify-center hover:bg-white/10 active:bg-white/20"
                    >
                        00
                    </button>
                </div>

                {/* Footer Actions */}
                <div className="flex justify-end items-center p-6 gap-8">
                    <button
                        onClick={onClose}
                        className="text-[#E57373] font-bold tracking-widest text-sm hover:opacity-80 py-2"
                    >
                        CANCEL
                    </button>
                    <button
                        onClick={handleSaveClick}
                        className="text-[#E57373] font-bold tracking-widest text-sm hover:opacity-80 py-2"
                    >
                        OK
                    </button>
                </div>
            </div>
        </div>
    );
}
