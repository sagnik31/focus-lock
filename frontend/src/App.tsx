import { useState, useEffect, useMemo } from 'react';
import { AddApp, RemoveApp, StartFocus, StopFocus, GetConfig, GetLockState, GetNow, SetBlockedApps, GetInstalledApps, GetTopBlockedApps, AddBlockedSite, RemoveBlockedSite, AddBlockedSites, RemoveBlockedSites, SetBlockCommonVPN, ImportSettings, ForgetRecentSite, SaveSiteGroups } from "../wailsjs/go/bridge/App";
import { storage, sysinfo } from "../wailsjs/go/models";
import { FocusActive } from "./components/FocusActive";
import { AppLayout } from "./components/AppLayout";
import { PendingSession, formatDuration } from "./components/duration";
import { useUpdateState } from "./components/updates";
import { LockState, NO_LOCK, parseLockState } from "./components/lockState";

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
    // The lock in force, from the backend's schedule logic (GetLockState).
    const [lockState, setLockState] = useState<LockState>(NO_LOCK);
    const [updateState, applyUpdateState] = useUpdateState();

    const refresh = async () => {
        try {
            const data = await GetConfig();
            setConfig(data);
            setLockState(parseLockState(await GetLockState()));
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
        // Polling the backend is the external system this effect syncs with;
        // refresh only sets state after its awaits resolve.
        // eslint-disable-next-line react-hooks/set-state-in-effect
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

    const isLocked = lockState.kind !== 'none';

    const handleAdd = async () => {
        if (!newApp) return;
        try {
            await AddApp(newApp);
            setNewApp("");
            succeeded();
        } catch (err) {
            setError(String(err));
        }
    };

    // Quick add from top apps
    const handleAddByName = async (name: string) => {
        try {
            await AddApp(name);
            succeeded();
        } catch (err) {
            setError(String(err));
        }
    };

    const handleRemove = async (app: string) => {
        try {
            await RemoveApp(app);
            succeeded();
        } catch (err) {
            setError(String(err));
        }
    };

    const handleSaveApps = async (apps: string[]) => {
        try {
            await SetBlockedApps(apps);
            succeeded();
        } catch (err) {
            console.error("SetBlockedApps failed:", err);
            setError(String(err));
        }
    };

    // Website Handlers
    const handleAddSite = async (url: string) => {
        try {
            await AddBlockedSite(url);
            succeeded();
        } catch (err) {
            setError(String(err));
        }
    };

    const handleRemoveSite = async (url: string) => {
        try {
            await RemoveBlockedSite(url);
            succeeded();
        } catch (err) {
            setError(String(err));
        }
    };

    const handleQuit = async () => {
        try {
            await StopFocus();
            succeeded();
        } catch (err) {
            setError(String(err));
        }
    };

    const handleToggleVPN = async (enabled: boolean) => {
        try {
            await SetBlockCommonVPN(enabled);
            succeeded();
        } catch (err) {
            setError(String(err));
        }
    };

    // Import settings from JSON
    const handleImportSettings = async (jsonContent: string) => {
        try {
            await ImportSettings(jsonContent);
            succeeded();
        } catch (err) {
            setError("Import failed: " + String(err));
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
        } catch (err) {
            setError("Failed to start: " + err);
        }
    };

    const handleAddSites = (urls: string[]) => {
        AddBlockedSites(urls).then(succeeded).catch((err) => setError(String(err)));
    };

    const handleRemoveSites = (urls: string[]) => {
        RemoveBlockedSites(urls).then(succeeded).catch((err) => setError(String(err)));
    };

    const handleForgetRecent = (url: string) => {
        ForgetRecentSite(url).then(succeeded).catch((err) => setError(String(err)));
    };

    // Rejects so the group editor can show the error inline and stay open.
    const handleSaveGroups = async (groups: storage.SiteGroup[]) => {
        await SaveSiteGroups(groups);
        setError("");
        await refresh();
    };

    if (isLocked && config) {
        // The backend reports whichever of the manual lock and the schedule
        // window ends later, so the countdown matches enforcement.
        const effectiveEndTime = lockState.endTime;

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
                isSchedule={lockState.kind === 'schedule'}
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
