import { useState, useEffect, useMemo } from 'react';
import { AddApp, RemoveApp, StartFocus, GetConfig, SetBlockedApps, GetInstalledApps, GetTopBlockedApps } from "../wailsjs/go/bridge/App";
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

    const refresh = async () => {
        try {
            const data = await GetConfig();
            setConfig(data);
            const top = await GetTopBlockedApps();
            setTopApps(top);
        } catch (e) {
            console.error(e);
        }
    };

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

    // Derived State
    const isLocked = useMemo(() => {
        if (!config?.lock_end_time) return false;
        return new Date(config.lock_end_time) > new Date();
    }, [config]);

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
        console.log("Saving apps:", apps);
        try {
            await SetBlockedApps(apps);
            console.log("SetBlockedApps completed");
            refresh();
        } catch (err: any) {
            console.error("SetBlockedApps failed:", err);
            setError(err.toString());
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

    if (isLocked && config) {
        return (
            <FocusActive
                endTime={config.lock_end_time}
                blockedApps={config.blocked_apps}
                appMap={appMap}
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
        />
    );
}

export default App;
