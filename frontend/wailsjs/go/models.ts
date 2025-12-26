export namespace storage {
	
	export class Stats {
	    kill_counts: Record<string, number>;
	
	    static createFrom(source: any = {}) {
	        return new Stats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kill_counts = source["kill_counts"];
	    }
	}
	export class Config {
	    blocked_apps: string[];
	    schedule?: Record<string, string>;
	    stats: Stats;
	    // Go type: time
	    lock_end_time: any;
	    ghost_task_name: string;
	    ghost_exe_path: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.blocked_apps = source["blocked_apps"];
	        this.schedule = source["schedule"];
	        this.stats = this.convertValues(source["stats"], Stats);
	        this.lock_end_time = this.convertValues(source["lock_end_time"], null);
	        this.ghost_task_name = source["ghost_task_name"];
	        this.ghost_exe_path = source["ghost_exe_path"];
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
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.icon = source["icon"];
	        this.exe = source["exe"];
	    }
	}

}

