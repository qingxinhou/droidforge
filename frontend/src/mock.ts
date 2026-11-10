/**
 * 界面预览用的假数据层。
 *
 * 只有在真正的 Wails 运行时不存在时才生效（即用 `npm run dev` 直接在浏览器里打开），
 * 目的是让前端能脱离 Go 后端单独调试与截图。打包后的应用由 Wails 注入 window.go，
 * 这里会立刻 return，不产生任何影响。
 */

/* eslint-disable @typescript-eslint/no-explicit-any */

const config = {
  customAdb: '',
  customFastboot: '',
  mirror: '',
  cacheDir: 'C:\\Users\\demo\\AppData\\Roaming\\DroidForge\\cache',
  outputDir: 'C:\\Users\\demo\\DroidForge',
  lastSerial: 'PJZ110',
  autoRefresh: true,
  confirmDanger: true,
  theme: 'system',
  themeExplicit: false,
  keepAwake: true,
  otaUserAgent: '',
  otaReferer: '',
  startPage: '/',
  uiScale: 1,
  cpuIntervalMs: 1500,
  pushDir: 'C:\\Users\\demo\\DroidForge',
  hideIds: false,
  installReinstall: true,
  // 默认 0 = 未同意，dev 预览时会先走一遍免责协议门（点同意后本次会话不再出现）。
  // 只想调界面、不想每次过协议门，把这里改成 1。
  disclaimerVersion: 0,
  disclaimerAcceptedAt: '2026-11-10T01:18:00Z',
}

const environment = {
  tools: {
    adbPath: 'C:\\platform-tools\\adb.exe',
    fastbootPath: 'C:\\platform-tools\\fastboot.exe',
    adbVersion: 'Android Debug Bridge version 1.0.41',
    fastbootVersion: 'fastboot version 35.0.2',
    source: 'platform-tools',
    ready: true,
    hint: '',
  },
  adbDevices: [
    { serial: 'PJZ110', state: 'device', model: '一加 13', product: 'oneplus_13', device: 'oneplus13', transportId: '1' },
  ],
  fastbootDevices: [],
  config,
  os: 'windows',
  arch: 'amd64',
  appVersion: '1.6.0',
}

const deviceInfo = {
  serial: 'PJZ110',
  model: '一加 13',
  brand: 'OnePlus',
  device: 'oneplus13',
  productName: 'PJZ110',
  androidVersion: '15',
  sdk: 35,
  securityPatch: '2026-08-05',
  buildId: 'AP4A.250705.002',
  buildFingerprint: 'OnePlus/PJZ110/oneplus13:15/AP4A.250705.002:user/release-keys',
  buildType: 'user',
  buildTags: 'release-keys',
  abi: 'arm64-v8a',
  abiList: 'arm64-v8a,armeabi-v7a,armeabi',
  soc: 'Snapdragon 8 Elite (SM8750)',
  kernel: '6.6.30-android15-8-g3f2b1c0',
  arch: 'aarch64',
  slot: 'a',
  isAb: true,
  hasDynamicPartitions: true,
  verifiedBoot: 'orange',
  flashLocked: 'false',
  debuggable: false,
  selinux: 'Enforcing',
  uptime: '2 天 14 小时 22 分',
  memoryTotalKb: 16_318_464,
  storageTotalKb: 489_835_520,
  storageFreeKb: 212_664_320,
  batteryLevel: 78,
  root: {
    rooted: true,
    solutions: ['KernelSU'],
    suPath: '/system/bin/ksud',
    magiskVersion: '',
    kernelSUVersion: 'v1.0.1',
    apatchVersion: '',
    zygisk: false,
    detail: 'KernelSU 内核模式',
  },
}

const partitions = [
  { name: 'boot', block: '/dev/block/sde11', sizeBytes: 100_663_296, slot: 'a', dynamic: false, mountPoint: '', fsType: '' },
  { name: 'init_boot', block: '/dev/block/sde12', sizeBytes: 8_388_608, slot: 'a', dynamic: false, mountPoint: '', fsType: '' },
  { name: 'vendor_boot', block: '/dev/block/sde13', sizeBytes: 100_663_296, slot: 'a', dynamic: false, mountPoint: '', fsType: '' },
  { name: 'dtbo', block: '/dev/block/sde14', sizeBytes: 25_165_824, slot: 'a', dynamic: false, mountPoint: '', fsType: '' },
  { name: 'vbmeta', block: '/dev/block/sde15', sizeBytes: 65_536, slot: 'a', dynamic: false, mountPoint: '', fsType: '' },
  { name: 'super', block: '/dev/block/sde22', sizeBytes: 9_126_805_504, slot: '', dynamic: true, mountPoint: '', fsType: '' },
  { name: 'userdata', block: '/dev/block/sda12', sizeBytes: 501_444_657_152, slot: '', dynamic: false, mountPoint: '/data', fsType: 'f2fs' },
  { name: 'metadata', block: '/dev/block/sda11', sizeBytes: 16_777_216, slot: '', dynamic: false, mountPoint: '', fsType: '' },
  { name: 'persist', block: '/dev/block/sda3', sizeBytes: 33_554_432, slot: '', dynamic: false, mountPoint: '/mnt/vendor/persist', fsType: 'ext4' },
]

const cpuSnapshot = {
  cores: [
    { index: 0, cur: 1_900_000, min: 364_000, max: 2_189_000, governor: 'walt', freqs: null },
    { index: 1, cur: 1_900_000, min: 364_000, max: 2_189_000, governor: 'walt', freqs: null },
    { index: 2, cur: 2_400_000, min: 364_000, max: 2_189_000, governor: 'walt', freqs: null },
    { index: 3, cur: 1_100_000, min: 364_000, max: 2_189_000, governor: 'walt', freqs: null },
    { index: 4, cur: 2_700_000, min: 537_000, max: 3_150_000, governor: 'walt', freqs: null },
    { index: 5, cur: 3_150_000, min: 537_000, max: 3_150_000, governor: 'walt', freqs: null },
    { index: 6, cur: 2_900_000, min: 537_000, max: 3_764_000, governor: 'walt', freqs: null },
    { index: 7, cur: 3_764_000, min: 537_000, max: 3_764_000, governor: 'walt', freqs: null },
  ],
  clusters: [
    { index: 0, cores: [0, 1, 2, 3], min: 364_000, max: 2_189_000, cur: 1_900_000, governor: 'walt', freqs: null },
    { index: 1, cores: [4, 5], min: 537_000, max: 3_150_000, cur: 3_150_000, governor: 'walt', freqs: null },
    { index: 2, cores: [6, 7], min: 537_000, max: 3_764_000, cur: 3_764_000, governor: 'walt', freqs: null },
  ],
}

const apps = [
  { package: 'com.tencent.mm', label: '微信', version: '8.0.54', system: false, enabled: true, apkPath: '/data/app/~~x/com.tencent.mm/base.apk', sizeBytes: 412_336_128 },
  { package: 'com.tencent.mobileqq', label: 'QQ', version: '9.1.20', system: false, enabled: true, apkPath: '/data/app/~~y/com.tencent.mobileqq/base.apk', sizeBytes: 336_592_896 },
  { package: 'com.taobao.taobao', label: '淘宝', version: '10.42.0', system: false, enabled: true, apkPath: '/data/app/~~z/com.taobao.taobao/base.apk', sizeBytes: 268_435_456 },
  { package: 'com.jingdong.app.mall', label: '京东', version: '13.6.0', system: false, enabled: true, apkPath: '/data/app/~~a/com.jingdong.app.mall/base.apk', sizeBytes: 210_763_776 },
  { package: 'com.ss.android.ugc.aweme', label: '抖音', version: '31.8.0', system: false, enabled: true, apkPath: '/data/app/~~b/com.ss.android.ugc.aweme/base.apk', sizeBytes: 386_547_712 },
  { package: 'com.xingin.xhs', label: '小红书', version: '8.32.0', system: false, enabled: true, apkPath: '/data/app/~~c/com.xingin.xhs/base.apk', sizeBytes: 224_395_264 },
  { package: 'com.tencent.tmgp.sgame', label: '王者荣耀', version: '9.4.0', system: false, enabled: true, apkPath: '/data/app/~~d/com.tencent.tmgp.sgame/base.apk', sizeBytes: 1_610_612_736 },
  { package: 'com.miHoYo.Yuanshen', label: '原神', version: '5.6.0', system: false, enabled: true, apkPath: '/data/app/~~e/com.miHoYo.Yuanshen/base.apk', sizeBytes: 2_147_483_648 },
  { package: 'com.google.android.gms', label: 'Google Play 服务', version: '25.30.31', system: true, enabled: true, apkPath: '/system/priv-app/GmsCore/GmsCore.apk', sizeBytes: 168_820_736 },
  { package: 'com.google.android.youtube', label: 'YouTube', version: '19.32.36', system: true, enabled: false, apkPath: '/system/app/YouTube/YouTube.apk', sizeBytes: 134_217_728 },
  { package: 'com.android.chrome', label: 'Chrome', version: '127.0.6533', system: true, enabled: true, apkPath: '/system/app/Chrome/Chrome.apk', sizeBytes: 251_658_240 },
  { package: 'com.android.settings', label: '设置', version: '15', system: true, enabled: true, apkPath: '/system/priv-app/Settings/Settings.apk', sizeBytes: 62_914_560 },
]

const remoteOta = {
  url: 'https://mirror.freedif.org/lineageos/full/raven/20261102/lineage-24.0-20261102-nightly-raven-signed.zip',
  fileName: 'lineage-24.0-20261102-nightly-raven-signed.zip',
  totalSize: 1_437_843_456,
  supportsRange: true,
  container: 'zip',
  payloadOffset: 4610,
  payloadSize: 1_412_433_920,
  version: 2,
  blockSize: 4096,
  full: true,
  partitions: [
    { name: 'abl', size: 1_048_576, compressed: 524_288, operations: 2, incremental: false },
    { name: 'bl1', size: 12_288, compressed: 12_288, operations: 1, incremental: false },
    { name: 'boot', size: 67_108_864, compressed: 13_107_200, operations: 32, incremental: false },
    { name: 'dtbo', size: 16_777_216, compressed: 41_984, operations: 3, incremental: false },
    { name: 'init_boot', size: 8_388_608, compressed: 2_097_152, operations: 1, incremental: false },
    { name: 'modem', size: 152_043_520, compressed: 78_643_200, operations: 48, incremental: false },
    { name: 'product', size: 1_073_741_824, compressed: 419_430_400, operations: 1042, incremental: false },
    { name: 'system', size: 1_006_632_960, compressed: 398_458_880, operations: 986, incremental: false },
    { name: 'system_ext', size: 402_653_184, compressed: 158_334_976, operations: 412, incremental: false },
    { name: 'vbmeta', size: 8_192, compressed: 8_192, operations: 1, incremental: false },
    { name: 'vbmeta_system', size: 4_096, compressed: 4_096, operations: 1, incremental: false },
    { name: 'vendor', size: 318_767_104, compressed: 128_974_848, operations: 358, incremental: false },
    { name: 'vendor_boot', size: 67_108_864, compressed: 18_874_368, operations: 12, incremental: false },
    { name: 'vendor_dlkm', size: 33_554_432, compressed: 15_728_640, operations: 24, incremental: false },
  ],
  note: '完整 OTA，支持按分区提取',
}

const tweaksState = {
  display: {
    physicalSize: '1440x3168',
    overrideSize: '1440x3168',
    physicalDensity: '560',
    overrideDensity: '560',
    fontScale: 1,
    windowAnim: 1,
    transitionAnim: 1,
    animatorAnim: 1,
    screenTimeoutMs: 30000,
  },
  sysui: {
    iconBlacklist: ['rotate', 'headset', 'volume'],
    clockSeconds: true,
    batteryPercent: true,
    quickCount: 6,
    immersive: false,
    captiveDisabled: true,
  },
}

const batteryInfo = {
  level: 78,
  scale: 100,
  status: 2,
  statusText: '充电中',
  health: 2,
  healthText: '良好',
  present: true,
  acPowered: true,
  usbPowered: true,
  wirelessPowered: false,
  voltageMv: 4_312,
  voltageV: 4.312,
  temperatureC: 32.4,
  currentUa: -2_450_000,
  currentMa: -2_450,
  powerW: 10.56,
  chargeCounterMah: 4_218,
  maxChargingMa: 5_000,
  technology: 'Li-ion',
  capacityLevel: 'normal',
}

const rootSolutions = [
  { id: 'kernelsu', name: 'KernelSU', vendor: 'tiann', description: '内核级 root，基于 GKI 内核，隐蔽性最好', repo: 'tiann/KernelSU', assetGlob: 'KernelSU_.*\\.apk', website: 'https://kernelsu.org', family: 'kernelsu' },
  { id: 'kernelsu-next', name: 'KernelSU Next', vendor: 'KernelSU-Next', description: '社区分支，支持更多内核版本与非 GKI 设备', repo: 'KernelSU-Next/KernelSU-Next', assetGlob: 'KernelSU_.*\\.apk', website: 'https://github.com/KernelSU-Next', family: 'kernelsu' },
  { id: 'magisk', name: 'Magisk 官方', vendor: 'topjohnwu', description: '最主流的系统级 root，模块生态最完整', repo: 'topjohnwu/Magisk', assetGlob: 'Magisk-.*\\.apk', website: 'https://github.com/topjohnwu/Magisk', family: 'magisk' },
  { id: 'magisk-alpha', name: 'Magisk Alpha（阿尔法面具）', vendor: 'vvb2060', description: '增强隐藏能力，适合对抗检测', repo: 'vvb2060/Magisk', assetGlob: '.*\\.apk', website: 'https://github.com/vvb2060/Magisk', family: 'magisk' },
  { id: 'magisk-delta', name: 'Magisk Delta（德尔塔面具）', vendor: 'HuskyDG', description: '保留 MagiskHide 与 SUList', repo: 'HuskyDG/magisk-files', assetGlob: '.*\\.apk', website: 'https://huskydg.github.io/magisk-files/', family: 'magisk' },
  { id: 'apatch', name: 'APatch', vendor: 'bmax121', description: '基于 KernelPatch 的内核 root', repo: 'bmax121/APatch', assetGlob: 'APatch_.*\\.apk', website: 'https://apatch.dev', family: 'apatch' },
]


const handlers: Record<string, (...args: any[]) => any> = {
  GetEnvironment: () => environment,
  RefreshDevices: () => environment,
  GetDeviceInfo: () => deviceInfo,
  GetPartitions: () => partitions,
  GetConfig: () => config,
  SaveConfig: () => undefined,
  // 预览免责协议门：dev 下每次启动都会弹，点同意后本次会话不再出现
  AcceptDisclaimer: (v: number) => {
    environment.config.disclaimerVersion = v
  },
  DeclineDisclaimer: () => undefined,
  ResetDisclaimer: () => {
    environment.config.disclaimerVersion = 0
  },
  SetToolsPaths: () => environment,
  GetTweaks: () => tweaksState,
  ReadBattery: () => batteryInfo,
  ReadCpu: () => cpuSnapshot,
  GetRootSolutions: () => rootSolutions,
  GetRootStatus: () => deviceInfo.root,
  ListApps: () => apps,
  ListAppsDetail: () => apps,
  GetAppDetail: (serial: string, pkg: string) => ({
    package: pkg,
    versionName: apps.find((a) => a.package === pkg)?.version ?? '1.0.0',
    versionCode: 4200,
    minSdk: '26',
    targetSdk: '35',
    firstInstall: '2026-03-11 09:22:14',
    lastUpdate: '2026-10-28 21:05:33',
    dataDir: `/data/user/0/${pkg}`,
    uid: '10234',
    codePath: `/data/app/~~x/${pkg}/base.apk`,
    splitPaths: [`/data/app/~~x/${pkg}/split_config.arm64_v8a.apk`],
    flags: ['DEBUGGABLE'],
    enabled: true,
    system: false,
  }),
  AppAction: () => undefined,
  AdbShell: (serial: string, cmd: string) => {
    if (cmd.includes('getprop ro.product.model')) return '一加 13'
    if (cmd.includes('wm size')) return 'Physical size: 1440x3168'
    if (cmd.includes('pm list packages')) return apps.map((a) => `package:${a.package}`).join('\n')
    return ''
  },
  InspectRemoteOta: () => remoteOta,
  InspectPackage: (path: string) => ({
    path,
    type: 'zip',
    kind: 'ota',
    totalSize: 1_437_843_456,
    entries: [
      { name: 'payload.bin', size: 1_412_433_920, kind: 'file', partition: 'payload' },
      { name: 'payload_properties.txt', size: 1_024, kind: 'file', partition: '' },
      { name: 'META-INF/com/android/metadata', size: 512, kind: 'file', partition: '' },
    ],
    partitions: remoteOta.partitions.map((p) => p.name),
    hasPayload: true,
    note: 'Android OTA 整包（含 payload.bin）',
  }),
  OtaOutputDir: (name: string) => `C:\\Users\\demo\\DroidForge\\${name.replace(/\.[^.]+$/, '')}`,
  IdentifyImage: () => 'Android boot image v4',
  SaveOtaRequest: () => undefined,
  SaveTextFile: () => '',
  PickFile: () => '',
  PickDirectory: () => '',
  PickFiles: () => [],
  SaveFile: () => '',
  CopyText: () => undefined,
  RevealInFolder: () => undefined,
  OpenPath: () => undefined,
  OpenURL: () => undefined,
}

/** 把假数据注入成 Wails 绑定，并模拟事件推送。 */
export function installMock() {
  // 生产构建里 import.meta.env.DEV 为 false，整个分支会被摇树移除。
  // 这一步不能只依赖 window.go 是否存在：Wails 注入绑定的时机晚于 main.ts 执行，
  // 抢先写入 window.go 会让真实绑定失效。
  if (!import.meta.env.DEV) return
  if ((window as any).go) return

  const app = new Proxy(
    {},
    {
      get(_target, prop: string) {
        const fn = handlers[prop]
        if (fn) return (...args: any[]) => Promise.resolve(fn(...args))
        return () => Promise.resolve(null)
      },
    },
  )
  ;(window as any).go = { main: { App: app } }

  const listeners: Record<string, ((payload: any) => void)[]> = {}
  ;(window as any).runtime = {
    EventsOn(name: string, cb: (payload: any) => void) {
      ;(listeners[name] ||= []).push(cb)
    },
    EventsOff() {},
  }

  const emit = (name: string, payload: any) => (listeners[name] ?? []).forEach((fn) => fn(payload))
  const log = (level: string, text: string, time: string) => emit('log', { level, text, time })

  window.setTimeout(() => {
    log('info', '搞机工坊已启动', '09:41:02')
    log('ok', '平台工具就绪（platform-tools）', '09:41:02')
    log('info', '发现设备 PJZ110（一加 13）', '09:41:03')
    log('cmd', '$ adb shell getprop ro.build.version.release', '09:41:05')
    log('out', '15', '09:41:05')
    log('cmd', '$ adb shell wm size', '09:41:06')
    log('out', 'Physical size: 1440x3168', '09:41:06')
  }, 400)

}
