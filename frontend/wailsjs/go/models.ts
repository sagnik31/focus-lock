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
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.blocked_apps = source["blocked_apps"];
	        this.schedule = source["schedule"];
	        this.stats = this.convertValues(source["stats"], Stats);
	        this.lock_end_time = this.convertValues(source["lock_end_time"], null);
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

