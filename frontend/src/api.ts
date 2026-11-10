import type {
  AppDetail,
  AppInfo,
  BatteryInfo,
  BatteryMock,
  BootImageInfo,
  Config,
  CpuSnapshot,
  DeviceInfo,
  DownloadProbe,
  DownloadResult,
  Environment,
  FlashStep,
  InstallResult,
  PackageInfo,
  Partition,
  PayloadInfo,
  RemoteOtaInfo,
  RootStatus,
  ScriptResult,
  Solution,
  SplitResult,
  SuperBuildInput,
  SuperBuildOptions,
  SuperBuildPartition,
  SuperMetadata,
  TweaksState,
} from './types'

type GoFn = (...args: any[]) => Promise<any>

interface GoApp {
  [key: string]: GoFn
}

function app(): GoApp | undefined {
  return (window as any).go?.main?.App
}

export function call<T>(method: string, ...args: any[]): Promise<T> {
  const fn = app()?.[method]
  if (!fn) {
    return Promise.reject(new Error('Wails 运行时尚未就绪，请稍候重试'))
  }
  return fn(...args) as Promise<T>
}

export const api = {
  // 环境
  getEnvironment: () => call<Environment>('GetEnvironment'),
  refreshDevices: () => call<Environment>('RefreshDevices'),
  setToolsPaths: (adbPath: string, fastbootPath: string) =>
    call<Environment>('SetToolsPaths', adbPath, fastbootPath),
  downloadPlatformTools: () => call<string>('DownloadPlatformTools'),

  // 设备
  getDeviceInfo: (serial: string) => call<DeviceInfo>('GetDeviceInfo', serial),
  getPartitions: (serial: string) => call<Partition[]>('GetPartitions', serial),

  // ADB
  adbShell: (serial: string, command: string) => call<string>('AdbShell', serial, command),
  adbReboot: (serial: string, target: string) => call<void>('AdbReboot', serial, target),
  adbPush: (serial: string, local: string, remote: string) =>
    call<void>('AdbPush', serial, local, remote),
  adbPull: (serial: string, remote: string, local: string) =>
    call<void>('AdbPull', serial, remote, local),
  adbInstall: (serial: string, apk: string, reinstall: boolean) =>
    call<void>('AdbInstall', serial, apk, reinstall),
  listApps: (serial: string, includeSystem: boolean) =>
    call<AppInfo[]>('ListApps', serial, includeSystem),
  uninstallApp: (serial: string, pkg: string, keepData: boolean) =>
    call<void>('UninstallApp', serial, pkg, keepData),
  screenshot: (serial: string) => call<string>('Screenshot', serial),
  wirelessConnect: (addr: string) => call<string>('WirelessConnect', addr),
  enableWireless: (serial: string, port: number) => call<string>('EnableWireless', serial, port),

  // Fastboot
  fastbootDevices: () => call<string[]>('FastbootDevices'),
  fastbootReboot: (serial: string, target: string) =>
    call<void>('FastbootReboot', serial, target),
  flashPartition: (serial: string, partition: string, image: string) =>
    call<void>('FlashPartition', serial, partition, image),
  erasePartition: (serial: string, partition: string) =>
    call<void>('ErasePartition', serial, partition),
  setActiveSlot: (serial: string, slot: string) => call<void>('SetActiveSlot', serial, slot),
  flashPlan: (serial: string, steps: FlashStep[]) => call<void>('FlashPlan', serial, steps),

  // 文件
  pickFile: (title: string, pattern: string) => call<string>('PickFile', title, pattern),
  pickDirectory: (title: string) => call<string>('PickDirectory', title),
  pickSaveFile: (title: string, defaultName: string) =>
    call<string>('PickSaveFile', title, defaultName),
  openPath: (path: string) => call<void>('OpenPath', path),
  revealInFolder: (path: string) => call<void>('RevealInFolder', path),
  openUrl: (url: string) => call<void>('OpenURL', url),
  copyText: (text: string) => call<void>('CopyText', text),
  saveTextFile: (defaultName: string, content: string) =>
    call<string>('SaveTextFile', defaultName, content),

  // 镜像 / 包
  inspectPackage: (path: string) => call<PackageInfo>('InspectPackage', path),
  identifyImage: (path: string) => call<string>('IdentifyImage', path),
  extractPackage: (path: string, names: string[], outDir: string) =>
    call<string[]>('ExtractPackage', path, names, outDir),
  inspectBootImage: (path: string) => call<BootImageInfo>('InspectBootImage', path),
  unpackBootImage: (path: string, outDir: string) =>
    call<string[]>('UnpackBootImage', path, outDir),
  inspectPayload: (path: string) => call<PayloadInfo>('InspectPayload', path),
  unsparseImage: (src: string, dst: string) => call<string>('UnsparseImage', src, dst),
  buildFlashPlan: (dir: string) => call<FlashStep[]>('BuildFlashPlan', dir),

  // 远程 OTA
  inspectRemoteOta: (link: string) => call<RemoteOtaInfo>('InspectRemoteOta', link),
  extractRemoteOta: (link: string, names: string[], outDir: string) =>
    call<string[]>('ExtractRemoteOta', link, names, outDir),
  downloadOta: (link: string) => call<string>('DownloadOta', link),
  otaOutputDir: (fileName: string) => call<string>('OtaOutputDir', fileName),
  saveOtaRequest: (ua: string, referer: string) =>
    call<void>('SaveOtaRequest', ua, referer),

  // Root
  getRootSolutions: () => call<Solution[]>('GetRootSolutions'),
  getRootStatus: (serial: string) => call<RootStatus>('GetRootStatus', serial),
  extractBoot: (serial: string, partition: string, outPath: string) =>
    call<string>('ExtractBoot', serial, partition, outPath),
  installRoot: (serial: string, solution: string, bootImage: string, apkPath: string) =>
    call<InstallResult>('InstallRoot', serial, solution, bootImage, apkPath),

  // 配置
  getConfig: () => call<Config>('GetConfig'),
  saveConfig: (cfg: Config) => call<void>('SaveConfig', cfg),
  cancelOperation: () => call<void>('CancelOperation'),

  // 显示 / 系统界面
  getTweaks: (serial: string) => call<TweaksState>('GetTweaks', serial),
  setResolution: (serial: string, size: string) => call<void>('SetResolution', serial, size),
  setDensity: (serial: string, density: number) => call<void>('SetDensity', serial, density),
  setFontScale: (serial: string, scale: number) => call<void>('SetFontScale', serial, scale),
  setAnimationScales: (serial: string, w: number, t: number, a: number) =>
    call<void>('SetAnimationScales', serial, w, t, a),
  setScreenTimeout: (serial: string, ms: number) => call<void>('SetScreenTimeout', serial, ms),
  setIconBlacklist: (serial: string, icons: string[]) =>
    call<void>('SetIconBlacklist', serial, icons),
  setClockSeconds: (serial: string, on: boolean) => call<void>('SetClockSeconds', serial, on),
  setBatteryPercent: (serial: string, on: boolean) => call<void>('SetBatteryPercent', serial, on),
  setQuickCount: (serial: string, n: number) => call<void>('SetQuickCount', serial, n),
  setImmersive: (serial: string, on: boolean) => call<void>('SetImmersive', serial, on),
  setCaptivePortal: (serial: string, disabled: boolean) =>
    call<void>('SetCaptivePortal', serial, disabled),
  sendKeyEvent: (serial: string, code: string) => call<void>('SendKeyEvent', serial, code),
  sendInputText: (serial: string, text: string) => call<void>('SendInputText', serial, text),

  // 电池 / CPU
  readBattery: (serial: string) => call<BatteryInfo>('ReadBattery', serial),
  mockBattery: (serial: string, m: BatteryMock) => call<void>('MockBattery', serial, m),
  resetBattery: (serial: string) => call<void>('ResetBattery', serial),
  readCpu: (serial: string) => call<CpuSnapshot>('ReadCpu', serial),

  // 应用增强
  listAppsDetail: (serial: string, includeSystem: boolean) =>
    call<AppInfo[]>('ListAppsDetail', serial, includeSystem),
  appAction: (serial: string, pkg: string, action: string) =>
    call<void>('AppAction', serial, pkg, action),
  getAppDetail: (serial: string, pkg: string) => call<AppDetail>('GetAppDetail', serial, pkg),
  extractApps: (serial: string, pkgs: string[]) => call<string[]>('ExtractApps', serial, pkgs),
  installApks: (serial: string, apks: string[], reinstall: boolean, split: boolean) =>
    call<void>('InstallApks', serial, apks, reinstall, split),
  pickFiles: (title: string, pattern: string) => call<string[]>('PickFiles', title, pattern),
  readTextFile: (path: string) => call<string>('ReadTextFile', path),

  // 控机脚本
  runScript: (serial: string, source: string) =>
    call<ScriptResult>('RunScript', serial, source),

  // 风险提示与免责声明
  acceptDisclaimer: (version: number) => call<void>('AcceptDisclaimer', version),
  declineDisclaimer: () => call<void>('DeclineDisclaimer'),
  resetDisclaimer: () => call<void>('ResetDisclaimer'),

  // 动态分区（super）
  inspectSuper: (path: string) => call<SuperMetadata>('InspectSuper', path),
  splitSuper: (path: string, names: string[], outDir: string) =>
    call<SplitResult[]>('SplitSuper', path, names, outDir),
  scanSuperParts: (dir: string) => call<SuperBuildPartition[]>('ScanSuperParts', dir),
  mergeSuper: (parts: SuperBuildInput[], outPath: string, opt: SuperBuildOptions) =>
    call<SuperMetadata>('MergeSuper', parts, outPath, opt),
  generateFlashScript: (imagesDir: string, outDir: string, slot: string, reboot: boolean, wipe: boolean) =>
    call<string[]>('GenerateFlashScript', imagesDir, outDir, slot, reboot, wipe),

  // 下载
  probeDownload: (link: string) => call<DownloadProbe>('ProbeDownload', link),
  downloadFile: (link: string, dst: string, threads: number) =>
    call<DownloadResult>('DownloadFile', link, dst, threads),
}
