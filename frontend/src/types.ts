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
  /** physical = by-name 物理分区；logical = super 映射出的逻辑分区 */
  kind?: 'physical' | 'logical' | 'unknown'
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

/** adb shell 在本机能否写入系统设置。MIUI / HyperOS 会把权限收走。 */
export interface TweaksCaps {
  systemWritable: boolean
  secureWritable: boolean
  globalWritable: boolean
  /** 给用户看的一句话解释；全部可写时为空串 */
  note: string
}

export interface TweaksState {
  display: DisplayState
  sysui: SysUIState
  caps: TweaksCaps
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

// ---------------------------------------------------------------------------
// 动态分区（super）
// ---------------------------------------------------------------------------

export interface SuperExtent {
  numSectors: number
  /** 0 = linear（读物理扇区），1 = zero */
  targetType: number
  targetData: number
  targetSource: number
}

export interface SuperPartition {
  name: string
  attributes: number
  group: string
  groupIndex: number
  sizeBytes: number
  extents: SuperExtent[] | null
  readOnly: boolean
  slotSuffixed: boolean
  disabled: boolean
}

export interface SuperGroup {
  name: string
  flags: number
  maxSize: number
}

export interface SuperBlockDevice {
  name: string
  firstLogicalSector: number
  alignment: number
  size: number
}

export interface SuperMetadata {
  path: string
  major: number
  minor: number
  slot: number
  geometry: {
    metadataMaxSize: number
    metadataSlotCount: number
    logicalBlockSize: number
  }
  partitions: SuperPartition[] | null
  groups: SuperGroup[] | null
  blockDevices: SuperBlockDevice[] | null
  totalSize: number
  checksumOk: boolean
  sparse: boolean
  note: string
}

export interface SuperBuildPartition {
  name: string
  path: string
  group: string
  sizeBytes: number
}

/** 提交给合成接口的最小字段集 */
export interface SuperBuildInput {
  name: string
  path: string
  group: string
}

export interface SuperBuildOptions {
  label: string
  defaultGroup: string
  blockSize: number
  alignment: number
  slotCount: number
  outputSize: number
}

export interface SplitResult {
  name: string
  path: string
  size: number
}

// ---------------------------------------------------------------------------
// 下载
// ---------------------------------------------------------------------------

export interface DownloadProbe {
  url: string
  fileName: string
  size: number
  supportsRange: boolean
  acceptRanges: string
  etag: string
  lastModified: string
}

export interface DownloadResult {
  path: string
  size: number
  threads: number
  supportsRange: boolean
  resumed: boolean
  note: string
}

// ---------------------------------------------------------------------------
// 引导校验（vbmeta / AVB）
// ---------------------------------------------------------------------------

export interface VBMetaDescriptor {
  tag: number
  kind: string
  class: number
  partitionName?: string
  imageSize?: number
  flags?: number
  hashAlgorithm?: string
  digest?: string
  salt?: string
  treeOffset?: number
  treeSize?: number
  dataBlockSize?: number
  hashBlockSize?: number
  fecNumRoots?: number
  fecOffset?: number
  fecSize?: number
  rootDigest?: string
  dmVerityVersion?: number
  cmdline?: string
  rollbackIndexLocation?: number
  publicKeyLen?: number
  key?: string
  value?: string
  size: number
}

export interface VBMetaImage {
  path: string
  vbmetaOffset: number
  vbmetaSize: number
  footerOriginalSize: number
  versionMajor: number
  versionMinor: number
  algorithmType: number
  algorithm: string
  rollbackIndex: number
  rollbackIndexLocation: number
  hashSize: number
  signatureSize: number
  publicKeySize: number
  flags: number
  hashtreeDisabled: boolean
  verificationDisabled: boolean
  releaseString: string
  descriptors: VBMetaDescriptor[]
  selfHash: string
  notes: string[]
}

export interface VBMetaReport {
  image: VBMetaImage
  summary: string
  level: string
}

export interface ARBHit {
  offset: number
  marker: string
  encoding: string
  value: number
  valueOffset: number
  delta: number
}

export interface ARBFileReport {
  path: string
  size: number
  scanned: number
  format: string
  hits: ARBHit[]
  values: number[]
  confidence: string
  note: string
}

export interface ARBVerdict {
  level: string
  title: string
  detail: string
}

export interface ARBReport {
  file: ARBFileReport
  verdict: ARBVerdict
  guest: number
  guestOk: boolean
}

// ---------------------------------------------------------------------------
// 备份
// ---------------------------------------------------------------------------

export interface BackupCategory {
  id: string
  label: string
  desc: string
  kind: string
  paths: string[]
}

export interface BackupCategoryPlan extends BackupCategory {
  size: number
  files: number
  exists: boolean
  note: string
}

export interface BackupPlan {
  serial: string
  categories: BackupCategoryPlan[]
  totalSize: number
  totalFiles: number
}

export interface BackupCategoryResult {
  id: string
  label: string
  files: number
  bytes: number
  skipped: number
  error: string
}

export interface BackupResult {
  outDir: string
  files: number
  bytes: number
  skipped: number
  categories: BackupCategoryResult[]
  manifest: string
}

export interface BackupOptions {
  outDir: string
  categoryIds: string[]
  skipExisting: boolean
}

// ---------------------------------------------------------------------------
// 模块
// ---------------------------------------------------------------------------

export interface MagiskModule {
  path: string
  fileName: string
  size: number
  id: string
  name: string
  version: string
  versionCode: string
  author: string
  description: string
  valid: boolean
  error: string
}

export interface MagiskManager {
  kind: string
  version: string
  rootOk: boolean
  note: string
}

export interface MagiskInstallResult {
  module: MagiskModule
  ok: boolean
  output: string
  error: string
}

// ---------------------------------------------------------------------------
// 投屏
// ---------------------------------------------------------------------------

export interface ScreenBinary {
  path: string
  found: boolean
  version: string
  hint: string
}

export interface ScreenOptions {
  serial: string
  adbPath: string
  bitRate: string
  maxSize: number
  maxFps: number
  turnScreenOff: boolean
  stayAwake: boolean
  noAudio: boolean
  alwaysOnTop: boolean
  record: string
}

export interface ScreenRunStatus {
  running: boolean
  tail: string
}

// ---------------------------------------------------------------------------
// EDL 9008
// ---------------------------------------------------------------------------

export interface EdlDevice {
  path: string
  vid: number
  pid: number
  name: string
}

export interface EdlLoaderInfo {
  path: string
  name: string
  size: number
  memory: string
  program: string
  valid: boolean
  note: string
}

export interface EdlPartition {
  index: number
  name: string
  typeGuid: string
  uniqueGuid: string
  firstLba: number
  lastLba: number
  sectors: number
  attributes: number
}

export interface EdlGPT {
  sectorSize: number
  lun: number
  diskGuid: string
  firstUsableLba: number
  lastUsableLba: number
  partitions: EdlPartition[]
  checksumOk: boolean
  note: string
}

export interface EdlStatus {
  connected: boolean
  device: string
  loader: string
  sectorSize: number
  partitions: number
}
