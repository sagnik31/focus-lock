import React, { useState, useMemo, useEffect, useRef } from 'react';

interface TimeSeekerProps {
    onDurationChange: (h: number, m: number) => void;
    onStart: () => void;
    initialMinutes?: number;
}

// Slider steps, finer at the short end where precision matters:
// 5-30m every 5m, to 1h every 10m, to 2h every 15m, to 6h every 30m, to 24h hourly.
const TIME_STEPS: number[] = (() => {
    const steps: number[] = [];
    for (let m = 5; m <= 30; m += 5) steps.push(m);
    for (let m = 40; m <= 60; m += 10) steps.push(m);
    for (let m = 75; m <= 120; m += 15) steps.push(m);
    for (let m = 150; m <= 360; m += 30) steps.push(m);
    for (let m = 420; m <= 1440; m += 60) steps.push(m);
    return steps;
})();

// Tick labels, placed at the slider position of their actual value. (They
// used to be spread evenly with justify-between, but the steps are not
// linear, so "1h" sat where ~3h30m really was.)
const TICKS: { minutes: number; label: string }[] = [
    { minutes: 5, label: '5m' },
    { minutes: 60, label: '1h' },
    { minutes: 120, label: '2h' },
    { minutes: 360, label: '6h' },
    { minutes: 720, label: '12h' },
    { minutes: 1440, label: '24h' },
];

// Must match the thumb width in index.css (input[type=range]::-webkit-slider-thumb).
const THUMB_PX = 24;

// The browser keeps the thumb inside the track, so the thumb centre for a
// fraction p sits at p * (width - thumb) + thumb / 2, not at p * width.
const positionFor = (index: number) => {
    const p = index / (TIME_STEPS.length - 1);
    return `calc(${p * 100}% + ${(0.5 - p) * THUMB_PX}px)`;
};

const nearestIndex = (minutes: number) => {
    let best = 0;
    TIME_STEPS.forEach((m, i) => {
        if (Math.abs(m - minutes) < Math.abs(TIME_STEPS[best] - minutes)) best = i;
    });
    return best;
};

export const TimeSeeker: React.FC<TimeSeekerProps> = ({ onDurationChange, onStart, initialMinutes = 15 }) => {
    const [sliderValue, setSliderValue] = useState(() => nearestIndex(initialMinutes));

    const progressPercent = (sliderValue / (TIME_STEPS.length - 1)) * 100;
    const currentMinutes = TIME_STEPS[sliderValue];
    const displayH = Math.floor(currentMinutes / 60);
    const displayM = currentMinutes % 60;

    // Report changes through a ref: the parent passes a new inline callback on
    // every render, and depending on it re-ran this effect (and re-set the
    // parent's state) on every render, in a loop.
    const onChangeRef = useRef(onDurationChange);
    onChangeRef.current = onDurationChange;
    useEffect(() => {
        onChangeRef.current(displayH, displayM);
    }, [displayH, displayM]);

    const tickIndexes = useMemo(() => TICKS.map(t => ({ ...t, index: TIME_STEPS.indexOf(t.minutes) })), []);

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
                    <div
                        className="h-full bg-blue-600 transition-all duration-75 ease-out"
                        style={{ width: `${progressPercent}%` }}
                    />
                </div>

                <input
                    type="range"
                    min="0"
                    max={TIME_STEPS.length - 1}
                    step="1"
                    value={sliderValue}
                    onChange={(e) => setSliderValue(parseInt(e.target.value))}
                    aria-label="Focus duration"
                    aria-valuetext={`${displayH} hours ${displayM} minutes`}
                    className="absolute w-full h-2 appearance-none bg-transparent cursor-pointer z-10 focus:outline-none group"
                />
            </div>

            {/* Tick labels at their real positions; click to jump there */}
            <div className="relative w-full max-w-xs h-4 text-[10px] text-slate-600 font-mono -mt-6 mb-8">
                {tickIndexes.map(t => (
                    <button
                        key={t.label}
                        onClick={() => setSliderValue(t.index)}
                        className={`absolute -translate-x-1/2 hover:text-slate-300 transition-colors ${sliderValue === t.index ? 'text-slate-300' : ''}`}
                        style={{ left: positionFor(t.index) }}
                    >
                        {t.label}
                    </button>
                ))}
            </div>

            {/* Start Button */}
            <button
                onClick={onStart}
                className="w-full max-w-[200px] bg-white text-slate-950 font-semibold py-4 rounded-xl hover:bg-blue-50 transition-colors shadow-lg shadow-white/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/60"
            >
                Start focus
            </button>
        </div>
    );
};
