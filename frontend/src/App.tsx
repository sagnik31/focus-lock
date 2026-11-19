import { useState, useEffect, useMemo } from 'react';
import { AddApp, RemoveApp, StartFocus, StopFocus, GetConfig, GetNow, SetBlockedApps, GetInstalledApps, GetTopBlockedApps, AddBlockedSite, RemoveBlockedSite, AddBlockedSites, RemoveBlockedSites, SetBlockCommonVPN, ImportSettings, ForgetRecentSite, SaveSiteGroups } from "../wailsjs/go/bridge/App";
import { storage, sysinfo } from "../wailsjs/go/models";
import { FocusActive } from "./components/FocusActive";
import { AppLayout } from "./components/AppLayout";
import { PendingSession, formatDuration } from "./components/duration";
import { useUpdateState } from "./components/updates";

function App() {
    const [config, setConfig] = useState<storage.Config | null>(null);
    const [newApp, setNewApp] = useState("");
    const [pendingSession, setPendingSession] = useState<PendingSession | null>(null);
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
    const [updateState, applyUpdateState] = useUpdateState();

    const refresh = async () => {
        try {
            const data = await GetConfig();
            setConfig(data);
            try {
                const serverMs = await GetNow();
                setNowOffset(serverMs - Date.now());
            } catch { /* GetNow unavailable: fall back to local clock (offset 0) */ }
        } catch (e) {
            console.error(e);
        }
    };

    // Top apps only change when stats do (a session starts or ends), so they
    // are not part of the 2s poll: resolving them can mean a full installed-
    // apps scan (registry, folders, PowerShell for icons).
    const refreshTopApps = () => {
        GetTopBlockedApps().then(setTopApps).catch(console.error);
    };

    // An action succeeded: clear any error banner from an earlier failure,
    // then pick up the new config.
    const succeeded = () => {
        setError("");
        refresh();
    };

    const trustedNow = () => new Date(Date.now() + nowOffset);

    useEffect(() => {
        refresh();
        const interval = setInterval(refresh, 2000); // Poll for updates

        // Fetch installed apps initially to populate names/icons
        GetInstalledApps().then(setInstalledApps).catch(console.error);

        // Fetch top apps initially
        refreshTopApps();

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
            succeeded();
        } catch (err: any) {
            setError(err.toString());
        }
    };

    // Quick add from top apps
    const handleAddByName = async (name: string) => {
        try {
            await AddApp(name);
            succeeded();
        } catch (err: any) {
            setError(err.toString());
        }
    };

    const handleRemove = async (app: string) => {
        try {
            await RemoveApp(app);
            succeeded();
        } catch (err: any) {
            setError(err.toString());
        }
    };

    const handleSaveApps = async (apps: string[]) => {
        try {
            await SetBlockedApps(apps);
            succeeded();
        } catch (err: any) {
            console.error("SetBlockedApps failed:", err);
            setError(err.toString());
        }
    };

    // Website Handlers
    const handleAddSite = async (url: string) => {
        try {
            await AddBlockedSite(url);
            succeeded();
        } catch (err: any) {
            setError(err.toString());
        }
    };

    const handleRemoveSite = async (url: string) => {
        try {
            await RemoveBlockedSite(url);
            succeeded();
        } catch (err: any) {
            setError(err.toString());
        }
    };

    const handleQuit = async () => {
        try {
            await StopFocus();
            succeeded();
        } catch (err: any) {
            setError(err.toString());
        }
    };

    const handleToggleVPN = async (enabled: boolean) => {
        try {
            await SetBlockCommonVPN(enabled);
            succeeded();
        } catch (err: any) {
            setError(err.toString());
        }
    };

    // Import settings from JSON
    const handleImportSettings = async (jsonContent: string) => {
        try {
            await ImportSettings(jsonContent);
            succeeded();
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
            const { h, m, until } = pendingSession;
            // "Focus until" sessions are measured against the enforcer's
            // trusted clock at the moment of starting, so the lock ends at the
            // chosen time even if the confirm dialog sat open for a while.
            const totalSeconds = until !== undefined
                ? Math.round((until - trustedNow().getTime()) / 1000)
                : (h * 3600) + (m * 60);
            if (totalSeconds <= 0) {
                setShowConfirm(false);
                setError("That end time has already passed. Pick a later date or time.");
                return;
            }
            await StartFocus(totalSeconds);

            // Success!
            setShowConfirm(false);
            setPendingSession(null);

            // Trigger Notification
            new Notification("Focus Session Started", {
                body: until !== undefined
                    ? `Locked until ${new Date(until).toLocaleString([], { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })}. Stay productive!`
                    : `Locked for ${formatDuration(totalSeconds)}. Stay productive!`,
                requireInteraction: false,
            });

            succeeded();
            refreshTopApps();
        } catch (err: any) {
            setError("Failed to start: " + err);
        }
    };

    const handleAddSites = (urls: string[]) => {
        AddBlockedSites(urls).then(succeeded).catch((err: any) => setError(err.toString()));
    };

    const handleRemoveSites = (urls: string[]) => {
        RemoveBlockedSites(urls).then(succeeded).catch((err: any) => setError(err.toString()));
    };

    const handleForgetRecent = (url: string) => {
        ForgetRecentSite(url).then(succeeded).catch((err: any) => setError(err.toString()));
    };

    // Rejects so the group editor can show the error inline and stay open.
    const handleSaveGroups = async (groups: storage.SiteGroup[]) => {
        await SaveSiteGroups(groups);
        setError("");
        await refresh();
    };

    if (isLocked && config) {
        // Enforcement lasts until BOTH the manual lock and the schedule window
        // are over, so the lock screen counts down to whichever ends later.
        // (Showing the schedule's end while a longer manual lock ran made the
        // countdown reach zero early and offer a Quit that the backend refused.)
        const manualEnd = config.lock_end_time && new Date(config.lock_end_time) > trustedNow()
            ? new Date(config.lock_end_time)
            : null;
        const scheduleEndsLast = !!activeScheduleEndTime && (!manualEnd || activeScheduleEndTime > manualEnd);
        const effectiveEndTime = (scheduleEndsLast ? activeScheduleEndTime : manualEnd)?.toISOString() ?? config.lock_end_time;

        // If user wants to see settings during active session
        if (focusViewMode === 'settings') {
            return (
                <AppLayout
                    config={config}
                    newApp={newApp}
                    setNewApp={setNewApp}
                    error={error}
                    onDismissError={() => setError("")}
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
                    handleForgetRecent={handleForgetRecent}
                    handleSaveGroups={handleSaveGroups}
                    isLocked={true}
                    onBackToFocus={() => setFocusViewMode('active')}
                    updateState={updateState}
                    onUpdateState={applyUpdateState}
                />
            );
        }

        return (
            <FocusActive
                endTime={effectiveEndTime}
                blockedApps={config.blocked_apps || []}
                blockedSites={config.blocked_sites || []}
                appMap={appMap}
                pausedUntil={config.paused_until}
                isSchedule={scheduleEndsLast}
                emergencyUnlocksUsed={config.emergency_unlocks_used || 0}
                onShowSettings={() => setFocusViewMode('settings')}
                onQuit={handleQuit}
                enforcementError={config.last_enforcement_error}
                error={error}
                onDismissError={() => setError("")}
                nowOffsetMs={nowOffset}
                updateDownloaded={updateState?.available && updateState.downloaded ? updateState.latest : undefined}
            />
        );
    }

    return (
        <AppLayout
            config={config}
            newApp={newApp}
            setNewApp={setNewApp}
            error={error}
            onDismissError={() => setError("")}
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
            handleForgetRecent={handleForgetRecent}
            handleSaveGroups={handleSaveGroups}
            updateState={updateState}
            onUpdateState={applyUpdateState}
        />
    );
}

export default App;
