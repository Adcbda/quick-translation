export namespace main {
	
	export class Config {
	    provider: string;
	    localModel: string;
	    llmBaseUrl: string;
	    llmModel: string;
	    llmApiKey: string;
	    clipboardEnabled: boolean;
	    alwaysOnTop: boolean;
	    minimalMode: boolean;
	    darkMode: boolean;
	    sourceLanguage: string;
	    targetLanguage: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.localModel = source["localModel"];
	        this.llmBaseUrl = source["llmBaseUrl"];
	        this.llmModel = source["llmModel"];
	        this.llmApiKey = source["llmApiKey"];
	        this.clipboardEnabled = source["clipboardEnabled"];
	        this.alwaysOnTop = source["alwaysOnTop"];
	        this.minimalMode = source["minimalMode"];
	        this.darkMode = source["darkMode"];
	        this.sourceLanguage = source["sourceLanguage"];
	        this.targetLanguage = source["targetLanguage"];
	    }
	}
	export class ModelDownloadStatus {
	    model: string;
	    status: string;
	    message: string;
	    progress: number;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelDownloadStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.status = source["status"];
	        this.message = source["message"];
	        this.progress = source["progress"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class ModelInfo {
	    id: string;
	    name: string;
	    description: string;
	    size: string;
	    downloaded: boolean;
	    selected: boolean;
	    default: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ModelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.size = source["size"];
	        this.downloaded = source["downloaded"];
	        this.selected = source["selected"];
	        this.default = source["default"];
	    }
	}
	export class RuntimeStatus {
	    pythonFound: boolean;
	    ready: boolean;
	    python: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new RuntimeStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pythonFound = source["pythonFound"];
	        this.ready = source["ready"];
	        this.python = source["python"];
	        this.message = source["message"];
	    }
	}
	export class TranslateRequest {
	    text: string;
	    source: string;
	    target: string;
	    requestId: string;
	
	    static createFrom(source: any = {}) {
	        return new TranslateRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.source = source["source"];
	        this.target = source["target"];
	        this.requestId = source["requestId"];
	    }
	}
	export class TranslateResult {
	    text: string;
	    provider: string;
	    model: string;
	
	    static createFrom(source: any = {}) {
	        return new TranslateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	    }
	}

}

