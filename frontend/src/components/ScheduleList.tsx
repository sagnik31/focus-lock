import { useState, useEffect } from 'react';
import { ScheduleEditor } from './ScheduleEditor';
import { GetSchedules, SaveSchedules } from '../../wailsjs/go/bridge/App';
import { storage } from '../../wailsjs/go/models';
import { Plus, CalendarClock, Pencil, Trash2 } from 'lucide-react';

interface ScheduleListProps {
    isLocked?: boolean;
}

export const ScheduleList: React.FC<ScheduleListProps> = ({ isLocked }) => {
    // State
    const [schedules, setSchedules] = useState<storage.Schedule[]>([]);
    const [editingSchedule, setEditingSchedule] = useState<storage.Schedule | null>(null);
    const [isCreating, setIsCreating] = useState(false);
    const [isLoading, setIsLoading] = useState(true);

    // Load Schedules on Mount
    useEffect(() => {
        loadSchedules();
    }, []);

    const loadSchedules = async () => {
        try {
            const data = await GetSchedules();
            setSchedules(data || []);
        } catch (err) {
            console.error("Failed to load schedules", err);
        } finally {
            setIsLoading(false);
        }
    };

    const handleSave = async (schedule: storage.Schedule) => {
        let newSchedules: storage.Schedule[];

        // Ensure proper typing for creating vs updating
        // The editor returns a partial schedule compatible object, 
        // we might need to ensure it matches the exact storage.Schedule type structure

        if (isCreating) {
            newSchedules = [...schedules, schedule];
            setIsCreating(false);
        } else {
            newSchedules = schedules.map(s => s.id === schedule.id ? schedule : s);
            setEditingSchedule(null);
        }

        setSchedules(newSchedules); // Optimistic update
        await SaveSchedules(newSchedules);
    };

    const handleDelete = async (id: string) => {
        const newSchedules = schedules.filter(s => s.id !== id);
        setSchedules(newSchedules); // Optimistic
        await SaveSchedules(newSchedules);
    };

    const toggleEnabled = async (id: string) => {
        const newSchedules = schedules.map(s =>
            s.id === id ? { ...s, enabled: !s.enabled } : s
        );
        setSchedules(newSchedules); // Optimistic
        await SaveSchedules(newSchedules);
    };

    if (isCreating || editingSchedule) {
        return (
            <ScheduleEditor
                schedule={editingSchedule}
                onSave={handleSave}
                onCancel={() => {
                    setIsCreating(false);
                    setEditingSchedule(null);
                }}
            />
        );
    }

    return (
        <div className="flex flex-col h-full overflow-hidden">
            <div className="flex justify-between items-center mb-4 shrink-0">
                <h3 className="text-xs font-bold text-slate-500 uppercase tracking-widest">Active Schedules</h3>
                <button
                    onClick={() => setIsCreating(true)}
                    className="text-sm font-semibold bg-blue-600 hover:bg-blue-500 text-white px-3 py-1.5 rounded-lg transition-all shadow-lg shadow-blue-900/20 flex items-center gap-1.5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400"
                >
                    <Plus className="w-4 h-4" /> New
                </button>
            </div>

            <div className="flex-1 overflow-y-auto space-y-3 custom-scrollbar pr-2 min-h-0">
                {schedules.length === 0 ? (
                    <div className="h-full flex flex-col items-center justify-center text-slate-500 space-y-3 opacity-60">
                        <CalendarClock className="w-10 h-10" strokeWidth={1} />
                        <span className="text-sm">No schedules set</span>
                    </div>
                ) : (
                    schedules.map(schedule => (
                        <div key={schedule.id} className="bg-slate-800/40 border border-white/5 rounded-xl p-4 hover:border-white/10 transition-all group relative">
                            <div className="flex justify-between items-start mb-2">
                                <div>
                                    <h4 className={`font-semibold text-sm ${schedule.enabled ? 'text-slate-200' : 'text-slate-500'}`}>
                                        {schedule.name}
                                    </h4>
                                    <div className="text-xs text-slate-400 mt-1 flex gap-2">
                                        <span className="font-mono bg-slate-950/30 px-1.5 py-0.5 rounded text-blue-300">
                                            {schedule.start_time} - {schedule.end_time}
                                        </span>
                                    </div>
                                </div>

                                {!isLocked && (
                                    <label className="relative inline-flex items-center cursor-pointer">
                                        <input
                                            type="checkbox"
                                            className="sr-only peer"
                                            checked={schedule.enabled}
                                            onChange={() => toggleEnabled(schedule.id)}
                                        />
                                        <div className="w-9 h-5 bg-slate-700 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-600"></div>
                                    </label>
                                )}
                            </div>

                            <div className="flex justify-between items-center mt-3">
                                <div className="flex gap-1">
                                    {['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'].map(day => (
                                        <span
                                            key={day}
                                            className={`text-[10px] uppercase font-bold px-1.5 py-0.5 rounded ${schedule.days.includes(day)
                                                ? (schedule.enabled ? 'bg-blue-500/20 text-blue-300 border border-blue-500/20' : 'bg-slate-700 text-slate-500 border border-slate-600')
                                                : 'bg-transparent text-slate-700'
                                                }`}
                                        >
                                            {day.substring(0, 1)}
                                        </span>
                                    ))}
                                </div>

                                <div className="flex gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                                    {!isLocked && (
                                        <>
                                            <button
                                                onClick={() => setEditingSchedule(schedule)}
                                                className="p-1.5 text-slate-400 hover:text-white hover:bg-slate-700 rounded-lg transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/50"
                                                title="Edit"
                                                aria-label={`Edit ${schedule.name}`}
                                            >
                                                <Pencil className="w-3.5 h-3.5" />
                                            </button>
                                            <button
                                                onClick={() => handleDelete(schedule.id)}
                                                className="p-1.5 text-slate-400 hover:text-red-400 hover:bg-red-900/20 rounded-lg transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-400/50"
                                                title="Delete"
                                                aria-label={`Delete ${schedule.name}`}
                                            >
                                                <Trash2 className="w-3.5 h-3.5" />
                                            </button>
                                        </>
                                    )}
                                </div>
                            </div>
                        </div>
                    ))
                )}
            </div>
        </div>
    );
};
