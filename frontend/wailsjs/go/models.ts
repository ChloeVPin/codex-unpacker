export namespace main {
	
	export class AppStatus {
	    stateVersion: string;
	    stateHash: string;
	    workingFolder: string;
	
	    static createFrom(source: any = {}) {
	        return new AppStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.stateVersion = source["stateVersion"];
	        this.stateHash = source["stateHash"];
	        this.workingFolder = source["workingFolder"];
	    }
	}
	export class DownloadResult {
	    mode: string;
	    version: string;
	    sha256: string;
	    path: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new DownloadResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.version = source["version"];
	        this.sha256 = source["sha256"];
	        this.path = source["path"];
	        this.message = source["message"];
	    }
	}
	export class PackageInfo {
	    name: string;
	    version: string;
	    packageMoniker: string;
	    packageFamilyName: string;
	    publisher: string;
	    architecture: string;
	    sha256: string;
	    size: number;
	    fileName: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new PackageInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.packageMoniker = source["packageMoniker"];
	        this.packageFamilyName = source["packageFamilyName"];
	        this.publisher = source["publisher"];
	        this.architecture = source["architecture"];
	        this.sha256 = source["sha256"];
	        this.size = source["size"];
	        this.fileName = source["fileName"];
	        this.path = source["path"];
	    }
	}
	export class ProbeResult {
	    sourceKind: string;
	    updateManifestVersion: string;
	    packageVersion: string;
	    packageMoniker: string;
	    downloadUrl: string;
	    expectedSha256: string;
	    mirrorReleaseTag: string;
	    mirrorReleaseUrl: string;
	    directStoreStatus: string;
	    wouldUpdate: boolean;
	    currentStateVersion: string;
	    currentStateSha256: string;
	
	    static createFrom(source: any = {}) {
	        return new ProbeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceKind = source["sourceKind"];
	        this.updateManifestVersion = source["updateManifestVersion"];
	        this.packageVersion = source["packageVersion"];
	        this.packageMoniker = source["packageMoniker"];
	        this.downloadUrl = source["downloadUrl"];
	        this.expectedSha256 = source["expectedSha256"];
	        this.mirrorReleaseTag = source["mirrorReleaseTag"];
	        this.mirrorReleaseUrl = source["mirrorReleaseUrl"];
	        this.directStoreStatus = source["directStoreStatus"];
	        this.wouldUpdate = source["wouldUpdate"];
	        this.currentStateVersion = source["currentStateVersion"];
	        this.currentStateSha256 = source["currentStateSha256"];
	    }
	}

}

