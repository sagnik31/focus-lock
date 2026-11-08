export namespace storage {
	
	export class SessionRecord {
	    // Go type: time
	    start: any;
	    // Go type: time
	    end: any;
	    duration_sec: number;
	    kind: string;
	    app_count: number;
	    site_count: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.start = this.convertValues(source["start"], null);
	        this.end = this.convertValues(source["end"], null);
	        this.duration_sec = source["duration_sec"];
	        this.kind = source["kind"];
	        this.app_count = source["app_count"];
	        this.site_count = source["site_count"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Stats {
	    kill_counts: Record<string, number>;
	    blocked_frequency: Record<string, number>;
	    blocked_duration: Record<string, number>;
	    sessions: SessionRecord[];
	    focus_days: string[];
	    kills_by_hour: number[];
	    total_focus_seconds: number;
	
	    static createFrom(source: any = {}) {
	        return new Stats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kill_counts = source["kill_counts"];
	        this.blocked_frequency = source["blocked_frequency"];
	        this.blocked_duration = source["blocked_duration"];
	        this.sessions = this.convertValues(source["sessions"], SessionRecord);
	        this.focus_days = source["focus_days"];
	        this.kills_by_hour = source["kills_by_hour"];
	        this.total_focus_seconds = source["total_focus_seconds"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Schedule {
	    id: string;
	    name: string;
	    days: string[];
	    start_time: string;
	    end_time: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Schedule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.days = source["days"];
	        this.start_time = source["start_time"];
	        this.end_time = source["end_time"];
	        this.enabled = source["enabled"];
	    }
	}
	export class Config {
	    blocked_apps: string[];
	    blocked_sites: string[];
	    block_common_vpn: boolean;
	    schedules: Schedule[];
	    stats: Stats;
	    // Go type: time
	    lock_end_time: any;
	    remaining_duration: number;
	    ghost_task_name: string;
	    ghost_exe_path: string;
	    // Go type: time
	    paused_until: any;
	    emergency_unlocks_used: number;
	    last_enforcement_error: string;
	    // Go type: time
	    session_start: any;
	    schedule_unlock_window: string;
	    // Go type: time
	    schedule_session_start: any;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.blocked_apps = source["blocked_apps"];
	        this.blocked_sites = source["blocked_sites"];
	        this.block_common_vpn = source["block_common_vpn"];
	        this.schedules = this.convertValues(source["schedules"], Schedule);
	        this.stats = this.convertValues(source["stats"], Stats);
	        this.lock_end_time = this.convertValues(source["lock_end_time"], null);
	        this.remaining_duration = source["remaining_duration"];
	        this.ghost_task_name = source["ghost_task_name"];
	        this.ghost_exe_path = source["ghost_exe_path"];
	        this.paused_until = this.convertValues(source["paused_until"], null);
	        this.emergency_unlocks_used = source["emergency_unlocks_used"];
	        this.last_enforcement_error = source["last_enforcement_error"];
	        this.session_start = this.convertValues(source["session_start"], null);
	        this.schedule_unlock_window = source["schedule_unlock_window"];
	        this.schedule_session_start = this.convertValues(source["schedule_session_start"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	

}

export namespace sysinfo {
	
	export class AppInfo {
	    name: string;
	    icon: string;
	    exe: string;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.icon = source["icon"];
	        this.exe = source["exe"];
	        this.source = source["source"];
	    }
	}

}

