import { useState, useEffect } from 'react';
import { storage } from '../../wailsjs/go/models';

interface ScheduleEditorProps {
    schedule?: storage.Schedule | null;
    onSave: (schedule: storage.Schedule) => void;
    onCancel: () => void;
}

const DAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

export const ScheduleEditor: React.FC<ScheduleEditorProps> = ({ schedule, onSave, onCancel }) => {
    const [name, setName] = useState("");
    const [selectedDays, setSelectedDays] = useState<string[]>([]);
    const [startTime, setStartTime] = useState("09:00");
    const [endTime, setEndTime] = useState("17:00");
    const [error, setError] = useState("");

    useEffect(() => {
        if (schedule) {
            setName(schedule.name);
            setSelectedDays(schedule.days);
            setStartTime(schedule.start_time);
            setEndTime(schedule.end_time);
        } else {
            // Defaults for new schedule
            setName("");
            setSelectedDays(["Mon", "Tue", "Wed", "Thu", "Fri"]);
            setStartTime("09:00");
            setEndTime("17:00");
        }
    }, [schedule]);

    const toggleDay = (day: string) => {
        if (selectedDays.includes(day)) {
            setSelectedDays(selectedDays.filter(d => d !== day));
        } else {
            // Keep order
            const newDays = [...selectedDays, day];
            // Sort based on DAYS constant index
            newDays.sort((a, b) => DAYS.indexOf(a) - DAYS.indexOf(b));
            setSelectedDays(newDays);
        }
    };

    const handleSave = () => {
        if (!name.trim()) {
            setError("Name is required");
            return;
        }
        if (selectedDays.length === 0) {
            setError("Select at least one day");
            return;
        }
        if (startTime >= endTime) {
            setError("End time must be after start time");
            return;
        }

        const newSchedule = new storage.Schedule({
            id: schedule?.id || Math.random().toString(36).substr(2, 9),
            name,
            days: selectedDays,
            start_time: startTime,
            end_time: endTime,
            enabled: schedule ? schedule.enabled : true,
        });
        onSave(newSchedule);
    };

    return (
        <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-300">
            <div className="flex justify-between items-center">
                <h3 className="text-lg font-bold text-white">
                    {schedule ? 'Edit Schedule' : 'New Schedule'}
                </h3>
                {error && <span className="text-red-400 text-sm">{error}</span>}
            </div>

            {/* Name Input */}
            <div className="space-y-2">
                <label className="text-xs font-bold text-slate-500 uppercase tracking-wider">Schedule Name</label>
                <input
                    type="text"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="e.g. Work Hours"
                    className="w-full bg-slate-950/50 border border-slate-700/50 focus:border-blue-500/50 rounded-lg px-4 py-2 text-slate-200 outline-none transition-all"
                />
            </div>

            {/* Day Picker */}
            <div className="space-y-2">
                <label className="text-xs font-bold text-slate-500 uppercase tracking-wider">Active Days</label>
                <div className="flex gap-2">
                    {DAYS.map(day => (
                        <button
                            key={day}
                            onClick={() => toggleDay(day)}
                            className={`flex-1 py-2 rounded-lg text-xs font-bold transition-all ${selectedDays.includes(day)
                                ? 'bg-blue-600 text-white shadow-lg shadow-blue-900/30'
                                : 'bg-slate-800 text-slate-400 hover:bg-slate-700'
                                }`}
                        >
                            {day}
                        </button>
                    ))}
                </div>
            </div>

            {/* Time Picker */}
            <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                    <label className="text-xs font-bold text-slate-500 uppercase tracking-wider">Start Time</label>
                    <input
                        type="time"
                        value={startTime}
                        onChange={(e) => setStartTime(e.target.value)}
                        className="w-full bg-slate-950/50 border border-slate-700/50 focus:border-blue-500/50 rounded-lg px-4 py-2 text-slate-200 outline-none transition-all"
                    />
                </div>
                <div className="space-y-2">
                    <label className="text-xs font-bold text-slate-500 uppercase tracking-wider">End Time</label>
                    <input
                        type="time"
                        value={endTime}
                        onChange={(e) => setEndTime(e.target.value)}
                        className="w-full bg-slate-950/50 border border-slate-700/50 focus:border-blue-500/50 rounded-lg px-4 py-2 text-slate-200 outline-none transition-all"
                    />
                </div>
            </div>

            {/* Actions */}
            <div className="flex gap-3 pt-4">
                <button
                    onClick={onCancel}
                    className="flex-1 px-4 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 font-semibold transition-colors text-sm"
                >
                    Cancel
                </button>
                <button
                    onClick={handleSave}
                    className="flex-1 px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-500 text-white font-bold shadow-lg shadow-blue-900/40 transition-all text-sm"
                >
                    Save Schedule
                </button>
            </div>
        </div>
    );
};
