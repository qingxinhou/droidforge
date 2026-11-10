export namespace adb {
	
	export class AppDetail {
	    package: string;
	    versionName: string;
	    versionCode: number;
	    minSdk: string;
	    targetSdk: string;
	    firstInstall: string;
	    lastUpdate: string;
	    dataDir: string;
	    uid: string;
	    codePath: string;
	    splitPaths: string[];
	    flags: string[];
	    enabled: boolean;
	    system: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AppDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.package = source["package"];
	        this.versionName = source["versionName"];
	        this.versionCode = source["versionCode"];
	        this.minSdk = source["minSdk"];
	        this.targetSdk = source["targetSdk"];
	        this.firstInstall = source["firstInstall"];
	        this.lastUpdate = source["lastUpdate"];
	        this.dataDir = source["dataDir"];
	        this.uid = source["uid"];
	        this.codePath = source["codePath"];
	        this.splitPaths = source["splitPaths"];
	        this.flags = source["flags"];
	        this.enabled = source["enabled"];
	        this.system = source["system"];
	    }
	}
	export class AppInfo {
	    package: string;
	    label: string;
	    version: string;
	    system: boolean;
	    enabled: boolean;
	    apkPath: string;
	    sizeBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.package = source["package"];
	        this.label = source["label"];
	        this.version = source["version"];
	        this.system = source["system"];
	        this.enabled = source["enabled"];
	        this.apkPath = source["apkPath"];
	        this.sizeBytes = source["sizeBytes"];
	    }
	}
	export class Device {
	    serial: string;
	    state: string;
	    model: string;
	    product: string;
	    device: string;
	    transportId: string;
	
	    static createFrom(source: any = {}) {
	        return new Device(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serial = source["serial"];
	        this.state = source["state"];
	        this.model = source["model"];
	        this.product = source["product"];
	        this.device = source["device"];
	        this.transportId = source["transportId"];
	    }
	}

}

export namespace cpu {
	
	export class Cluster {
	    index: number;
	    cores: number[];
	    min: number;
	    max: number;
	    cur: number;
	    governor: string;
	    freqs: number[];
	
	    static createFrom(source: any = {}) {
	        return new Cluster(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.cores = source["cores"];
	        this.min = source["min"];
	        this.max = source["max"];
	        this.cur = source["cur"];
	        this.governor = source["governor"];
	        this.freqs = source["freqs"];
	    }
	}
	export class Core {
	    index: number;
	    cur: number;
	    min: number;
	    max: number;
	    governor: string;
	    freqs: number[];
	
	    static createFrom(source: any = {}) {
	        return new Core(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.cur = source["cur"];
	        this.min = source["min"];
	        this.max = source["max"];
	        this.governor = source["governor"];
	        this.freqs = source["freqs"];
	    }
	}
	export class Snapshot {
	    cores: Core[];
	    clusters: Cluster[];
	
	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cores = this.convertValues(source["cores"], Core);
	        this.clusters = this.convertValues(source["clusters"], Cluster);
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

export namespace device {
	
	export class RootStatus {
	    rooted: boolean;
	    solutions: string[];
	    suPath: string;
	    magiskVersion: string;
	    kernelSUVersion: string;
	    apatchVersion: string;
	    zygisk: boolean;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new RootStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rooted = source["rooted"];
	        this.solutions = source["solutions"];
	        this.suPath = source["suPath"];
	        this.magiskVersion = source["magiskVersion"];
	        this.kernelSUVersion = source["kernelSUVersion"];
	        this.apatchVersion = source["apatchVersion"];
	        this.zygisk = source["zygisk"];
	        this.detail = source["detail"];
	    }
	}
	export class DeviceInfo {
	    serial: string;
	    model: string;
	    brand: string;
	    device: string;
	    productName: string;
	    androidVersion: string;
	    sdk: number;
	    securityPatch: string;
	    buildId: string;
	    buildFingerprint: string;
	    buildType: string;
	    buildTags: string;
	    abi: string;
	    abiList: string;
	    soc: string;
	    kernel: string;
	    arch: string;
	    slot: string;
	    isAb: boolean;
	    hasDynamicPartitions: boolean;
	    verifiedBoot: string;
	    flashLocked: string;
	    debuggable: boolean;
	    selinux: string;
	    uptime: string;
	    memoryTotalKb: number;
	    storageTotalKb: number;
	    storageFreeKb: number;
	    batteryLevel: number;
	    root: RootStatus;
	
	    static createFrom(source: any = {}) {
	        return new DeviceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serial = source["serial"];
	        this.model = source["model"];
	        this.brand = source["brand"];
	        this.device = source["device"];
	        this.productName = source["productName"];
	        this.androidVersion = source["androidVersion"];
	        this.sdk = source["sdk"];
	        this.securityPatch = source["securityPatch"];
	        this.buildId = source["buildId"];
	        this.buildFingerprint = source["buildFingerprint"];
	        this.buildType = source["buildType"];
	        this.buildTags = source["buildTags"];
	        this.abi = source["abi"];
	        this.abiList = source["abiList"];
	        this.soc = source["soc"];
	        this.kernel = source["kernel"];
	        this.arch = source["arch"];
	        this.slot = source["slot"];
	        this.isAb = source["isAb"];
	        this.hasDynamicPartitions = source["hasDynamicPartitions"];
	        this.verifiedBoot = source["verifiedBoot"];
	        this.flashLocked = source["flashLocked"];
	        this.debuggable = source["debuggable"];
	        this.selinux = source["selinux"];
	        this.uptime = source["uptime"];
	        this.memoryTotalKb = source["memoryTotalKb"];
	        this.storageTotalKb = source["storageTotalKb"];
	        this.storageFreeKb = source["storageFreeKb"];
	        this.batteryLevel = source["batteryLevel"];
	        this.root = this.convertValues(source["root"], RootStatus);
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
	export class Partition {
	    name: string;
	    block: string;
	    sizeBytes: number;
	    slot: string;
	    dynamic: boolean;
	    mountPoint: string;
	    fsType: string;
	
	    static createFrom(source: any = {}) {
	        return new Partition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.block = source["block"];
	        this.sizeBytes = source["sizeBytes"];
	        this.slot = source["slot"];
	        this.dynamic = source["dynamic"];
	        this.mountPoint = source["mountPoint"];
	        this.fsType = source["fsType"];
	    }
	}

}

export namespace image {
	
	export class BootImageInfo {
	    path: string;
	    kind: string;
	    headerVersion: number;
	    pageSize: number;
	    kernelSize: number;
	    ramdiskSize: number;
	    secondSize: number;
	    dtbSize: number;
	    recoveryDtboSize: number;
	    signatureSize: number;
	    osVersion: string;
	    name: string;
	    cmdline: string;
	    fileSize: number;
	    sections: string[];
	
	    static createFrom(source: any = {}) {
	        return new BootImageInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.kind = source["kind"];
	        this.headerVersion = source["headerVersion"];
	        this.pageSize = source["pageSize"];
	        this.kernelSize = source["kernelSize"];
	        this.ramdiskSize = source["ramdiskSize"];
	        this.secondSize = source["secondSize"];
	        this.dtbSize = source["dtbSize"];
	        this.recoveryDtboSize = source["recoveryDtboSize"];
	        this.signatureSize = source["signatureSize"];
	        this.osVersion = source["osVersion"];
	        this.name = source["name"];
	        this.cmdline = source["cmdline"];
	        this.fileSize = source["fileSize"];
	        this.sections = source["sections"];
	    }
	}
	export class PayloadPartition {
	    name: string;
	    size: number;
	    operations: number;
	    incremental: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PayloadPartition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.size = source["size"];
	        this.operations = source["operations"];
	        this.incremental = source["incremental"];
	    }
	}
	export class PayloadInfo {
	    path: string;
	    version: number;
	    blockSize: number;
	    partitions: PayloadPartition[];
	    full: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PayloadInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.version = source["version"];
	        this.blockSize = source["blockSize"];
	        this.partitions = this.convertValues(source["partitions"], PayloadPartition);
	        this.full = source["full"];
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

export namespace main {
	
	export class Environment {
	    tools: platform.Tools;
	    adbDevices: adb.Device[];
	    fastbootDevices: string[];
	    config: store.Config;
	    os: string;
	    arch: string;
	    appVersion: string;
	
	    static createFrom(source: any = {}) {
	        return new Environment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tools = this.convertValues(source["tools"], platform.Tools);
	        this.adbDevices = this.convertValues(source["adbDevices"], adb.Device);
	        this.fastbootDevices = source["fastbootDevices"];
	        this.config = this.convertValues(source["config"], store.Config);
	        this.os = source["os"];
	        this.arch = source["arch"];
	        this.appVersion = source["appVersion"];
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
	export class TweaksState {
	    display: tweaks.Display;
	    sysui: tweaks.SysUI;
	
	    static createFrom(source: any = {}) {
	        return new TweaksState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.display = this.convertValues(source["display"], tweaks.Display);
	        this.sysui = this.convertValues(source["sysui"], tweaks.SysUI);
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

export namespace platform {
	
	export class Tools {
	    adbPath: string;
	    fastbootPath: string;
	    adbVersion: string;
	    fastbootVersion: string;
	    source: string;
	    ready: boolean;
	    hint: string;
	
	    static createFrom(source: any = {}) {
	        return new Tools(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.adbPath = source["adbPath"];
	        this.fastbootPath = source["fastbootPath"];
	        this.adbVersion = source["adbVersion"];
	        this.fastbootVersion = source["fastbootVersion"];
	        this.source = source["source"];
	        this.ready = source["ready"];
	        this.hint = source["hint"];
	    }
	}

}

export namespace power {
	
	export class Info {
	    level: number;
	    scale: number;
	    status: number;
	    statusText: string;
	    health: number;
	    healthText: string;
	    present: boolean;
	    acPowered: boolean;
	    usbPowered: boolean;
	    wirelessPowered: boolean;
	    voltageMv: number;
	    voltageV: number;
	    temperatureC: number;
	    currentUa: number;
	    currentMa: number;
	    powerW: number;
	    chargeCounterMah: number;
	    maxChargingMa: number;
	    technology: string;
	    capacityLevel: string;
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	        this.scale = source["scale"];
	        this.status = source["status"];
	        this.statusText = source["statusText"];
	        this.health = source["health"];
	        this.healthText = source["healthText"];
	        this.present = source["present"];
	        this.acPowered = source["acPowered"];
	        this.usbPowered = source["usbPowered"];
	        this.wirelessPowered = source["wirelessPowered"];
	        this.voltageMv = source["voltageMv"];
	        this.voltageV = source["voltageV"];
	        this.temperatureC = source["temperatureC"];
	        this.currentUa = source["currentUa"];
	        this.currentMa = source["currentMa"];
	        this.powerW = source["powerW"];
	        this.chargeCounterMah = source["chargeCounterMah"];
	        this.maxChargingMa = source["maxChargingMa"];
	        this.technology = source["technology"];
	        this.capacityLevel = source["capacityLevel"];
	    }
	}
	export class Mock {
	    level: number;
	    ac?: boolean;
	    usb?: boolean;
	    wireless?: boolean;
	    unplug: boolean;
	    fullReset: boolean;
	    discharging: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Mock(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	        this.ac = source["ac"];
	        this.usb = source["usb"];
	        this.wireless = source["wireless"];
	        this.unplug = source["unplug"];
	        this.fullReset = source["fullReset"];
	        this.discharging = source["discharging"];
	    }
	}

}

export namespace remote {
	
	export class Partition {
	    name: string;
	    size: number;
	    compressed: number;
	    operations: number;
	    incremental: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Partition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.size = source["size"];
	        this.compressed = source["compressed"];
	        this.operations = source["operations"];
	        this.incremental = source["incremental"];
	    }
	}
	export class Info {
	    url: string;
	    fileName: string;
	    totalSize: number;
	    supportsRange: boolean;
	    container: string;
	    payloadOffset: number;
	    payloadSize: number;
	    version: number;
	    blockSize: number;
	    full: boolean;
	    partitions: Partition[];
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.fileName = source["fileName"];
	        this.totalSize = source["totalSize"];
	        this.supportsRange = source["supportsRange"];
	        this.container = source["container"];
	        this.payloadOffset = source["payloadOffset"];
	        this.payloadSize = source["payloadSize"];
	        this.version = source["version"];
	        this.blockSize = source["blockSize"];
	        this.full = source["full"];
	        this.partitions = this.convertValues(source["partitions"], Partition);
	        this.note = source["note"];
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

export namespace rom {
	
	export class Entry {
	    name: string;
	    size: number;
	    kind: string;
	    partition: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.size = source["size"];
	        this.kind = source["kind"];
	        this.partition = source["partition"];
	    }
	}
	export class FlashStep {
	    partition: string;
	    image: string;
	    sizeBytes: number;
	    needsFastbootd: boolean;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new FlashStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.partition = source["partition"];
	        this.image = source["image"];
	        this.sizeBytes = source["sizeBytes"];
	        this.needsFastbootd = source["needsFastbootd"];
	        this.note = source["note"];
	    }
	}
	export class PackageInfo {
	    path: string;
	    type: string;
	    kind: string;
	    totalSize: number;
	    entries: Entry[];
	    partitions: string[];
	    hasPayload: boolean;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new PackageInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.type = source["type"];
	        this.kind = source["kind"];
	        this.totalSize = source["totalSize"];
	        this.entries = this.convertValues(source["entries"], Entry);
	        this.partitions = source["partitions"];
	        this.hasPayload = source["hasPayload"];
	        this.note = source["note"];
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

export namespace root {
	
	export class InstallResult {
	    solution: string;
	    patchedImage: string;
	    steps: string[];
	
	    static createFrom(source: any = {}) {
	        return new InstallResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.solution = source["solution"];
	        this.patchedImage = source["patchedImage"];
	        this.steps = source["steps"];
	    }
	}
	export class Solution {
	    id: string;
	    name: string;
	    vendor: string;
	    description: string;
	    repo: string;
	    assetGlob: string;
	    website: string;
	    family: string;
	
	    static createFrom(source: any = {}) {
	        return new Solution(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.vendor = source["vendor"];
	        this.description = source["description"];
	        this.repo = source["repo"];
	        this.assetGlob = source["assetGlob"];
	        this.website = source["website"];
	        this.family = source["family"];
	    }
	}

}

export namespace script {
	
	export class Result {
	    steps: number;
	    failures: number;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.steps = source["steps"];
	        this.failures = source["failures"];
	    }
	}

}

export namespace store {
	
	export class Config {
	    customAdb: string;
	    customFastboot: string;
	    mirror: string;
	    cacheDir: string;
	    outputDir: string;
	    lastSerial: string;
	    autoRefresh: boolean;
	    confirmDanger: boolean;
	    theme: string;
	    themeExplicit: boolean;
	    keepAwake: boolean;
	    otaUserAgent: string;
	    otaReferer: string;
	    disclaimerVersion: number;
	    disclaimerAcceptedAt: string;
	    startPage: string;
	    uiScale: number;
	    cpuIntervalMs: number;
	    pushDir: string;
	    hideIds: boolean;
	    installReinstall: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.customAdb = source["customAdb"];
	        this.customFastboot = source["customFastboot"];
	        this.mirror = source["mirror"];
	        this.cacheDir = source["cacheDir"];
	        this.outputDir = source["outputDir"];
	        this.lastSerial = source["lastSerial"];
	        this.autoRefresh = source["autoRefresh"];
	        this.confirmDanger = source["confirmDanger"];
	        this.theme = source["theme"];
	        this.themeExplicit = source["themeExplicit"];
	        this.keepAwake = source["keepAwake"];
	        this.otaUserAgent = source["otaUserAgent"];
	        this.otaReferer = source["otaReferer"];
	        this.disclaimerVersion = source["disclaimerVersion"];
	        this.disclaimerAcceptedAt = source["disclaimerAcceptedAt"];
	        this.startPage = source["startPage"];
	        this.uiScale = source["uiScale"];
	        this.cpuIntervalMs = source["cpuIntervalMs"];
	        this.pushDir = source["pushDir"];
	        this.hideIds = source["hideIds"];
	        this.installReinstall = source["installReinstall"];
	    }
	}

}

export namespace tweaks {
	
	export class Display {
	    physicalSize: string;
	    overrideSize: string;
	    physicalDensity: string;
	    overrideDensity: string;
	    fontScale: number;
	    windowAnim: number;
	    transitionAnim: number;
	    animatorAnim: number;
	    screenTimeoutMs: number;
	
	    static createFrom(source: any = {}) {
	        return new Display(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.physicalSize = source["physicalSize"];
	        this.overrideSize = source["overrideSize"];
	        this.physicalDensity = source["physicalDensity"];
	        this.overrideDensity = source["overrideDensity"];
	        this.fontScale = source["fontScale"];
	        this.windowAnim = source["windowAnim"];
	        this.transitionAnim = source["transitionAnim"];
	        this.animatorAnim = source["animatorAnim"];
	        this.screenTimeoutMs = source["screenTimeoutMs"];
	    }
	}
	export class SysUI {
	    iconBlacklist: string[];
	    clockSeconds: boolean;
	    batteryPercent: boolean;
	    quickCount: number;
	    immersive: boolean;
	    captiveDisabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SysUI(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.iconBlacklist = source["iconBlacklist"];
	        this.clockSeconds = source["clockSeconds"];
	        this.batteryPercent = source["batteryPercent"];
	        this.quickCount = source["quickCount"];
	        this.immersive = source["immersive"];
	        this.captiveDisabled = source["captiveDisabled"];
	    }
	}

}

