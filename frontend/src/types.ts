// 与 Go 端 JSON 结构一一对应的类型定义

export interface Tools {
  adbPath: string
  fastbootPath: string
  adbVersion: string
  fastbootVersion: string
  source: string
  ready: boolean
  hint: string
}

export interface AdbDevice {
  serial: string
  state: string
  model: string
  product: string
  device: string
  transportId: string
}

export interface Config {
  customAdb: string
  customFastboot: string
  mirror: string
  cacheDir: string
  outputDir: string
  lastSerial: string
  autoRefresh: boolean
  confirmDanger: boolean
  theme: string
  /** 用户是否手动选过主题；false 时跟随系统 */
  themeExplicit: boolean
  keepAwake: boolean
  otaUserAgent: string
  otaReferer: string
  startPage: string
  uiScale: number
  cpuIntervalMs: number
  pushDir: string
  hideIds: boolean
  installReinstall: boolean
  disclaimerVersion: number
  disclaimerAcceptedAt: string
}

export interface Environment {
  tools: Tools
  adbDevices: AdbDevice[] | null
  fastbootDevices: string[] | null
  config: Config
  os: string
  arch: string
  appVersion: string
}

export interface RootStatus {
  rooted: boolean
  solutions: string[] | null
  suPath: string
  magiskVersion: string
  kernelSUVersion: string
  apatchVersion: string
  zygisk: boolean
  detail: string
}

export interface DeviceInfo {
  serial: string
  model: string
  brand: string
  device: string
  productName: string
  androidVersion: string
  sdk: number
  securityPatch: string
  buildId: string
  buildFingerprint: string
  buildType: string
  buildTags: string
  abi: string
  abiList: string
  soc: string
  kernel: string
  arch: string
  slot: string
  isAb: boolean
  hasDynamicPartitions: boolean
  verifiedBoot: string
  flashLocked: string
  debuggable: boolean
  selinux: string
  uptime: string
  memoryTotalKb: number
  storageTotalKb: number
  storageFreeKb: number
  batteryLevel: number
  root: RootStatus
}

export interface Partition {
  name: string
  block: string
  sizeBytes: number
  slot: string
  dynamic: boolean
  mountPoint: string
  fsType: string
}

export interface LogEntry {
  level: string
  text: string
  time: string
}

export interface Progress {
  label: string
  percent: number
  done: boolean
}

export interface PackageEntry {
  name: string
  size: number
  kind: string
  partition: string
}

export interface PackageInfo {
  path: string
  type: string
  kind: string
  totalSize: number
  entries: PackageEntry[] | null
  partitions: string[] | null
  hasPayload: boolean
  note: string
}

export interface BootImageInfo {
  path: string
  kind: string
  headerVersion: number
  pageSize: number
  kernelSize: number
  ramdiskSize: number
  secondSize: number
  dtbSize: number
  recoveryDtboSize: number
  signatureSize: number
  osVersion: string
  name: string
  cmdline: string
  fileSize: number
  sections: string[] | null
}

export interface PayloadPartition {
  name: string
  size: number
  operations: number
  incremental: boolean
}

export interface PayloadInfo {
  path: string
  version: number
  blockSize: number
  partitions: PayloadPartition[] | null
  full: boolean
}

/** 远程 OTA 链接解析结果 */
export interface RemoteOtaPartition {
  name: string
  size: number
  compressed: number
  operations: number
  incremental: boolean
}

export interface RemoteOtaInfo {
  url: string
  fileName: string
  totalSize: number
  supportsRange: boolean
  container: string
  payloadOffset: number
  payloadSize: number
  version: number
  blockSize: number
  full: boolean
  partitions: RemoteOtaPartition[] | null
  note: string
}

export interface FlashStep {
  partition: string
  image: string
  sizeBytes: number
  needsFastbootd: boolean
  note: string
}

export interface Solution {
  id: string
  name: string
  vendor: string
  description: string
  repo: string
  assetGlob: string
  website: string
  family: string
}

export interface AppInfo {
  package: string
  label: string
  version: string
  system: boolean
  enabled: boolean
  apkPath: string
  sizeBytes: number
}

export interface InstallResult {
  solution: string
  patchedImage: string
  steps: string[] | null
}

export interface Toast {
  id: number
  type: 'ok' | 'err' | 'warn' | 'info'
  text: string
}

// ---------------------------------------------------------------------------
// 显示 / 系统界面
// ---------------------------------------------------------------------------

export interface DisplayState {
  physicalSize: string
  overrideSize: string
  physicalDensity: string
  overrideDensity: string
  fontScale: number
  windowAnim: number
  transitionAnim: number
  animatorAnim: number
  screenTimeoutMs: number
}

export interface SysUIState {
  iconBlacklist: string[] | null
  clockSeconds: boolean
  batteryPercent: boolean
  quickCount: number
  immersive: boolean
  captiveDisabled: boolean
}

export interface TweaksState {
  display: DisplayState
  sysui: SysUIState
}

// ---------------------------------------------------------------------------
// 电池
// ---------------------------------------------------------------------------

export interface BatteryInfo {
  level: number
  scale: number
  status: number
  statusText: string
  health: number
  healthText: string
  present: boolean
  acPowered: boolean
  usbPowered: boolean
  wirelessPowered: boolean
  voltageMv: number
  voltageV: number
  temperatureC: number
  currentUa: number
  currentMa: number
  powerW: number
  chargeCounterMah: number
  maxChargingMa: number
  technology: string
  capacityLevel: string
}

export interface BatteryMock {
  level: number
  ac: boolean | null
  usb: boolean | null
  wireless: boolean | null
  unplug: boolean
  fullReset: boolean
  discharging: boolean
}

// ---------------------------------------------------------------------------
// CPU
// ---------------------------------------------------------------------------

export interface CpuCore {
  index: number
  cur: number
  min: number
  max: number
  governor: string
  freqs: number[] | null
}

export interface CpuCluster {
  index: number
  cores: number[] | null
  min: number
  max: number
  cur: number
  governor: string
  freqs: number[] | null
}

export interface CpuSnapshot {
  cores: CpuCore[] | null
  clusters: CpuCluster[] | null
}

// ---------------------------------------------------------------------------
// 应用详情与脚本
// ---------------------------------------------------------------------------

export interface AppDetail {
  package: string
  versionName: string
  versionCode: number
  minSdk: string
  targetSdk: string
  firstInstall: string
  lastUpdate: string
  dataDir: string
  uid: string
  codePath: string
  splitPaths: string[] | null
  flags: string[] | null
  enabled: boolean
  system: boolean
}

export interface ScriptResult {
  steps: number
  failures: number
}
