import { useState, useEffect, useMemo } from 'react';
import { AddApp, RemoveApp, StartFocus, StopFocus, GetConfig, GetNow, SetBlockedApps, GetInstalledApps, GetTopBlockedApps, AddBlockedSite, RemoveBlockedSite, AddBlockedSites, RemoveBlockedSites, SetBlockCommonVPN, ImportSettings } from "../wailsjs/go/bridge/App";
import { storage, sysinfo } from "../wailsjs/go/models";
import { FocusActive } from "./components/FocusActive";
import { AppLayout } from "./components/AppLayout";

function App() {
    const [config, setConfig] = useState<storage.Config | null>(null);
    const [newApp, setNewApp] = useState("");
    const [pendingSession, setPendingSession] = useState<{ h: number, m: number } | null>(null);
    const [showConfirm, setShowConfirm] = useState(false);
    const [error, setError] = useState("");
    const [isSelectorOpen, setIsSelectorOpen] = useState(false);
    const [installedApps, setInstalledApps] = useState<sysinfo.AppInfo[]>([]);
    const [topApps, setTopApps] = useState<sysinfo.AppInfo[]>([]);
    const [focusViewMode, setFocusViewMode] = useState<'active' | 'settings'>('active');
    // Offset (ms) between the enforcer's trusted (NTP-anchored) clock and this
    // browser's local clock: trustedNow = Date.now() + nowOffset. Keeps the UI's
    // timer and lock state aligned with what the Ghost actually enforces, even if
    // the system clock is changed.
    const [nowOffset, setNowOffset] = useState(0);

    const refresh = async () => {
        try {
            const data = await GetConfig();
            setConfig(data);
            try {
                const serverMs = await GetNow();
                setNowOffset(serverMs - Date.now());
            } catch { /* GetNow unavailable: fall back to local clock (offset 0) */ }
            const top = await GetTopBlockedApps();
            setTopApps(top);
        } catch (e) {
            console.error(e);
        }
    };

    const trustedNow = () => new Date(Date.now() + nowOffset);

    useEffect(() => {
        refresh();
        const interval = setInterval(refresh, 2000); // Poll for updates

        // Fetch installed apps initially to populate names/icons
        GetInstalledApps().then(setInstalledApps).catch(console.error);

        // Fetch top apps initially
        GetTopBlockedApps().then(setTopApps).catch(console.error);

        // Request notification permission on load
        if (Notification.permission !== "granted") {
            Notification.requestPermission();
        }

        return () => clearInterval(interval);
    }, []);

    const appMap = useMemo(() => {
        const map = new Map<string, sysinfo.AppInfo>();
        installedApps.forEach(app => {
            map.set(app.exe.toLowerCase(), app);
        });
        return map;
    }, [installedApps]);

    // Helper to check active schedule
    const getActiveScheduleEndTime = (schedules: storage.Schedule[] | undefined, now: Date): Date | null => {
        if (!schedules) return null;

        const days = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
        const today = days[now.getDay()];
        const yesterday = days[(now.getDay() + 6) % 7];
        const currentTime = now.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' }); // "HH:MM" 24h

        // dayOffset is added to the (trusted) current date to land on the window's END day.
        const endDateFor = (endTime: string, dayOffset: number): Date => {
            const [h, m] = endTime.split(':').map(Number);
            const endDate = new Date(now.getTime());
            endDate.setDate(endDate.getDate() + dayOffset);
            endDate.setHours(h, m, 0, 0);
            return endDate;
        };

        for (const s of schedules) {
            if (!s.enabled) continue;
            if (s.start_time === s.end_time) continue; // zero-length window

            const overnight = s.end_time < s.start_time; // crosses midnight (mirrors backend)

            if (s.days.includes(today)) {
                if (overnight) {
                    // Pre-midnight half, on the start day: ends tomorrow.
                    if (currentTime >= s.start_time) return endDateFor(s.end_time, 1);
                } else if (currentTime >= s.start_time && currentTime < s.end_time) {
                    return endDateFor(s.end_time, 0);
                }
            }

            // Post-midnight half of a window that started yesterday: ends today.
            if (overnight && s.days.includes(yesterday) && currentTime < s.end_time) {
                return endDateFor(s.end_time, 0);
            }
        }
        return null;
    };

    // Derived State
    const activeScheduleEndTime = useMemo(() => getActiveScheduleEndTime(config?.schedules, trustedNow()), [config, nowOffset, new Date().getMinutes()]); // Re-eval on minute change? Effect loop handles refresh.

    const isLocked = useMemo(() => {
        const manualLock = config?.lock_end_time && new Date(config.lock_end_time) > trustedNow();
        return manualLock || !!activeScheduleEndTime;
    }, [config, activeScheduleEndTime, nowOffset]);

    const handleAdd = async () => {
        if (!newApp) return;
        try {
            await AddApp(newApp);
            setNewApp("");
            refresh();
        } catch (err: any) {
            setError(err.toString());
        }
    };

    // Quick add from top apps
    const handleAddByName = async (name: string) => {
        try {
            await AddApp(name);
            refresh();
        } catch (err: any) {
            setError(err.toString());
        }
    };

    const handleRemove = async (app: string) => {
        await RemoveApp(app);
        refresh();
    };

    const handleSaveApps = async (apps: string[]) => {
        try {
            await SetBlockedApps(apps);
            refresh();
        } catch (err: any) {
            console.error("SetBlockedApps failed:", err);
            setError(err.toString());
        }
    };

    // Website Handlers
    const handleAddSite = async (url: string) => {
        try {
            await AddBlockedSite(url);
            refresh();
        } catch (err: any) {
            setError(err.toString());
        }
    };

    const handleRemoveSite = async (url: string) => {
        try {
            await RemoveBlockedSite(url);
            refresh();
        } catch (err: any) {
            setError(err.toString());
        }
    };

    const handleQuit = async () => {
        try {
            await StopFocus();
            refresh();
        } catch (err: any) {
            setError(err.toString());
        }
    };

    const handleToggleVPN = async (enabled: boolean) => {
        try {
            await SetBlockCommonVPN(enabled);
            refresh();
        } catch (err: any) {
            setError(err.toString());
        }
    };

    // Import settings from JSON
    const handleImportSettings = async (jsonContent: string) => {
        try {
            await ImportSettings(jsonContent);
            refresh();
        } catch (err: any) {
            setError("Import failed: " + err.toString());
            throw err; // Re-throw to let caller handle UI
        }
    };

    // Called by Keypad "OK"
    const handleRequestStart = (h: number, m: number) => {
        const totalSeconds = (h * 3600) + (m * 60);
        if (totalSeconds <= 0) {
            setError("Duration must be greater than 0");
            return;
        }
        setPendingSession({ h, m });
        setShowConfirm(true);
    };

    const confirmStart = async () => {
        if (!pendingSession) return;
        try {
            const { h, m } = pendingSession;
            const totalSeconds = (h * 3600) + (m * 60);
            await StartFocus(totalSeconds);

            // Success!
            setShowConfirm(false);
            setPendingSession(null);

            // Trigger Notification
            new Notification("Focus Session Started", {
                body: `Locked for ${h > 0 ? h + "h " : ""}${m}m. Stay productive!`,
                requireInteraction: false,
            });

            refresh();
        } catch (err: any) {
            setError("Failed to start: " + err);
        }
    };

    const handleAddSites = (urls: string[]) => {
        AddBlockedSites(urls).then(refresh).catch((err: any) => setError(err.toString()));
    };

    const handleRemoveSites = (urls: string[]) => {
        RemoveBlockedSites(urls).then(refresh).catch((err: any) => setError(err.toString()));
    };

    if (isLocked && config) {
        // Determine which end time to show
        let effectiveEndTime = config.lock_end_time;
        if (activeScheduleEndTime) {
            // If manual lock is ALSO active and ends later, use that? 
            // Logic: If manual lock is active, use it. If not, use schedule.
            // But what if both? Usually manual overrides.
            // Let's check logic: isLocked is OR.
            const manualActive = config.lock_end_time && new Date(config.lock_end_time) > trustedNow();
            if (!manualActive) {
                effectiveEndTime = activeScheduleEndTime.toISOString();
            } else {
                // Both active? Show whichever is longer? Or just manual?
                // Let's default to max of both to be safe, or just stick to manual if present.
                // If manual is active, user specifically requested it.
            }
        }

        // If user wants to see settings during active session
        if (focusViewMode === 'settings') {
            return (
                <AppLayout
                    config={config}
                    newApp={newApp}
                    setNewApp={setNewApp}
                    error={error}
                    isSelectorOpen={isSelectorOpen}
                    setIsSelectorOpen={setIsSelectorOpen}
                    showConfirm={showConfirm}
                    setShowConfirm={setShowConfirm}
                    pendingSession={pendingSession}
                    setPendingSession={setPendingSession}
                    topApps={topApps}
                    appMap={appMap}
                    handleAdd={handleAdd}
                    handleAddByName={handleAddByName}
                    handleRemove={handleRemove}
                    handleSaveApps={handleSaveApps}
                    handleRequestStart={handleRequestStart}
                    confirmStart={confirmStart}
                    handleAddSite={handleAddSite}
                    handleRemoveSite={handleRemoveSite}
                    handleAddSites={handleAddSites}
                    handleRemoveSites={handleRemoveSites}
                    handleToggleVPN={handleToggleVPN}
                    handleImportSettings={handleImportSettings}
                    isLocked={true}
                    onBackToFocus={() => setFocusViewMode('active')}
                />
            );
        }

        return (
            <FocusActive
                endTime={activeScheduleEndTime?.toString() || config.lock_end_time}
                blockedApps={config.blocked_apps || []}
                blockedSites={config.blocked_sites || []}
                appMap={appMap}
                pausedUntil={config.paused_until}
                isSchedule={!!activeScheduleEndTime}
                emergencyUnlocksUsed={config.emergency_unlocks_used || 0}
                onShowSettings={() => setFocusViewMode('settings')}
                onQuit={handleQuit}
                enforcementError={config.last_enforcement_error}
                nowOffsetMs={nowOffset}
            />
        );
    }

    return (
        <AppLayout
            config={config}
            newApp={newApp}
            setNewApp={setNewApp}
            error={error}
            isSelectorOpen={isSelectorOpen}
            setIsSelectorOpen={setIsSelectorOpen}
            showConfirm={showConfirm}
            setShowConfirm={setShowConfirm}
            pendingSession={pendingSession}
            setPendingSession={setPendingSession}
            topApps={topApps}
            appMap={appMap}
            handleAdd={handleAdd}
            handleAddByName={handleAddByName}
            handleRemove={handleRemove}
            handleSaveApps={handleSaveApps}
            handleRequestStart={handleRequestStart}
            confirmStart={confirmStart}
            handleAddSite={handleAddSite}
            handleRemoveSite={handleRemoveSite}
            handleAddSites={handleAddSites}
            handleRemoveSites={handleRemoveSites}
            handleToggleVPN={handleToggleVPN}
            handleImportSettings={handleImportSettings}
        />
    );
}

export default App;
