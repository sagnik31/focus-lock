import React, { useState, useMemo, useEffect } from 'react';

interface TimeSeekerProps {
    onDurationChange: (h: number, m: number) => void;
    onStart: () => void;
    initialMinutes?: number;
}

export const TimeSeeker: React.FC<TimeSeekerProps> = ({ onDurationChange, onStart, initialMinutes = 15 }) => {
    // Generate the time steps based on the user's requirements
    const timeSteps = useMemo(() => {
        const steps: number[] = [];

        // 1. 5m to 30m: 5 min intervals (5, 10, ... 30)
        for (let m = 5; m <= 30; m += 5) steps.push(m);

        // 2. 30m to 1h: 10 min intervals (40, 50, 60)
        for (let m = 40; m <= 60; m += 10) steps.push(m);

        // 3. 1h to 2h: 15 min intervals (75, 90, 105, 120)
        for (let m = 75; m <= 120; m += 15) steps.push(m);

        // 4. 2h to 6h: 30 min intervals (150, 180, ... 360)
        for (let m = 150; m <= 360; m += 30) steps.push(m);

        // 5. 6h to 24h: 1 hr intervals (420, 480, ... 1440)
        for (let m = 420; m <= 1440; m += 60) steps.push(m);

        return steps;
    }, []);

    // Find the closest index for the initial value
    const initialIndex = useMemo(() => {
        const idx = timeSteps.findIndex(m => m === initialMinutes);
        return idx !== -1 ? idx : 0;
    }, [initialMinutes, timeSteps]);

    const [sliderValue, setSliderValue] = useState(initialIndex);

    // Calculate generic percentage for background gradient
    const progressPercent = (sliderValue / (timeSteps.length - 1)) * 100;

    const currentMinutes = timeSteps[sliderValue];
    const displayH = Math.floor(currentMinutes / 60);
    const displayM = currentMinutes % 60;

    useEffect(() => {
        onDurationChange(displayH, displayM);
    }, [sliderValue, onDurationChange, displayH, displayM]);

    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        setSliderValue(parseInt(e.target.value));
    };

    return (
        <div className="w-full h-full flex flex-col items-center justify-center p-4">
            {/* Digital Display */}
            <div className="mb-10 text-center relative">
                <div className="text-6xl font-bold text-white tracking-tighter tabular-nums flex items-baseline justify-center gap-2 drop-shadow-2xl">
                    <span>{displayH.toString().padStart(2, '0')}</span>
                    <span className="text-2xl text-slate-500 font-medium">h</span>
                    <span>{displayM.toString().padStart(2, '0')}</span>
                    <span className="text-2xl text-slate-500 font-medium">m</span>
                </div>
            </div>

            {/* Slider Container */}
            <div className="w-full max-w-xs relative h-12 flex items-center mb-10">
                {/* Track Background */}
                <div className="absolute w-full h-2 bg-slate-800/50 rounded-full overflow-hidden border border-white/5">
                    {/* Progress Fill */}
                    <div
                        className="h-full bg-gradient-to-r from-blue-600 to-purple-600 transition-all duration-75 ease-out"
                        style={{ width: `${progressPercent}%` }}
                    />
                </div>

                {/* Range Input */}
                <input
                    type="range"
                    min="0"
                    max={timeSteps.length - 1}
                    step="1"
                    value={sliderValue}
                    onChange={handleChange}
                    className="absolute w-full h-2 appearance-none bg-transparent cursor-pointer z-10 focus:outline-none group"
                />

                {/* Custom Thumb (Pseudo-element via CSS or manual div if needed, but input styles usually cleaner in CSS) 
                    For simplified React implementation without external CSS dependency for thumb styling tricks, 
                    we usually rely on standard browser styling or `accent-color`. 
                    However, `range` inputs are notoriously hard to style identically cross-browser without CSS classes. 
                    I'll add specific classes in index.css
                */}
            </div>

            {/* Quick Actions / Ticks labels (Optional visual guide) */}
            <div className="flex justify-between w-full max-w-xs text-[10px] text-slate-600 font-mono -mt-6 mb-8 px-1">
                <span>5m</span>
                <span>30m</span>
                <span>1h</span>
                <span>6h</span>
                <span>24h</span>
            </div>

            {/* Start Button */}
            <button
                onClick={onStart}
                className="w-full max-w-[200px] bg-white text-slate-950 font-bold py-4 rounded-xl hover:bg-blue-50 transition-colors shadow-lg shadow-white/5"
            >
                START FOCUS
            </button>
        </div>
    );
};
