import { computed, reactive } from 'vue'
import { api } from './api'
import { applyTheme, type ThemeMode } from './theme'
import type { AdbDevice, Environment, LogEntry, Progress, Toast } from './types'

type Mode = 'adb' | 'fastboot'

export const state = reactive({
  ready: false,
  env: null as Environment | null,
  serial: '',
  mode: 'adb' as Mode,
  logs: [] as LogEntry[],
  progress: null as Progress | null,
  toasts: [] as Toast[],
  busy: false,
  logOpen: true,
  pendingImage: '',
})

let toastSeq = 0
let refreshTimer: number | undefined

export const devices = computed<AdbDevice[]>(() => {
  const env = state.env
  if (!env) return []
  const list: AdbDevice[] = []
  for (const d of env.adbDevices ?? []) {
    list.push(d)
  }
  for (const s of env.fastbootDevices ?? []) {
    if (!list.some((d) => d.serial === s)) {
      list.push({ serial: s, state: 'fastboot', model: '', product: '', device: '', transportId: '' })
    }
  }
  return list
})

export const currentDevice = computed<AdbDevice | null>(() => {
  return devices.value.find((d) => d.serial === state.serial) ?? null
})

export const toolsReady = computed(() => !!state.env?.tools.ready)

// 后端每个绑定方法失败时都会 a.log("err", ...)，前端 run() 又 push 一次，
// 同一条错误会在日志里出现两遍（真机日志里成对出现、每段 17 行堆栈）。
// 这里按「同文本 + 短时间窗」去重，既保住后端那一份，也不丢前端独有的错误。
let lastErrText = ''
let lastErrAt = 0

export function pushLog(entry: LogEntry) {
  if (entry.level === 'err') {
    const now = Date.now()
    if (entry.text === lastErrText && now - lastErrAt < 1200) {
      lastErrAt = now
      return
    }
    lastErrText = entry.text
    lastErrAt = now
  }
  state.logs.push(entry)
  if (state.logs.length > 3000) {
    state.logs.splice(0, state.logs.length - 3000)
  }
}

export function toast(type: Toast['type'], text: string, ms = 3200) {
  const id = ++toastSeq
  state.toasts.push({ id, type, text })
  window.setTimeout(() => {
    const idx = state.toasts.findIndex((t) => t.id === id)
    if (idx >= 0) state.toasts.splice(idx, 1)
  }, ms)
}

export function clearLogs() {
  state.logs = []
}

export async function init() {
  bindEvents()
  await refresh()
  applyUIScale(state.env?.config.uiScale ?? 1)
  applyTheme(normalizeTheme(state.env?.config.theme), false)
  state.ready = true
  startAutoRefresh()
}

/** 配置里的 theme 可能来自旧版本或被手工改过，统一收敛到合法值。 */
function normalizeTheme(v?: string): ThemeMode {
  return v === 'dark' || v === 'light' ? v : 'system'
}

/** 切换主题并写回配置。手动选过就不再回落到「跟随系统」。 */
export function setTheme(mode: ThemeMode) {
  applyTheme(mode)
  const cfg = state.env?.config
  if (cfg) {
    cfg.theme = mode
    cfg.themeExplicit = true
    api.saveConfig(cfg).catch(() => {})
  }
}

/** 按比例缩放整个界面 */
export function applyUIScale(scale: number) {
  const v = Math.min(1.4, Math.max(0.8, scale || 1))
  document.documentElement.style.setProperty('zoom', String(v))
}

/** 隐藏设备标识时对序列号做掩码 */
export function maskSerial(serial: string): string {
  if (!state.env?.config.hideIds || !serial) return serial
  if (serial.length <= 6) return '••••••'
  return serial.slice(0, 3) + '••••' + serial.slice(-2)
}

function bindEvents() {
  const rt = (window as any).runtime
  if (!rt) return
  rt.EventsOn('log', (e: LogEntry) => pushLog(e))
  rt.EventsOn('progress', (e: Progress) => {
    state.progress = e
    if (e.done) {
      window.setTimeout(() => {
        if (state.progress && state.progress.done) state.progress = null
      }, 1500)
    }
  })
}

export async function refresh() {
  try {
    const env = await api.getEnvironment()
    state.env = env
    // 保持选中设备有效
    const all = [...(env.adbDevices ?? []).map((d) => d.serial), ...(env.fastbootDevices ?? [])]
    if (state.serial && !all.includes(state.serial)) {
      state.serial = all[0] ?? ''
    }
    if (!state.serial && all.length === 1) {
      state.serial = all[0]
    }
    if (!state.serial && env.config.lastSerial && all.includes(env.config.lastSerial)) {
      state.serial = env.config.lastSerial
    }
    syncMode()
  } catch (e) {
    console.error(e)
  }
}

export function syncMode() {
  const env = state.env
  if (!env || !state.serial) return
  const isAdb = (env.adbDevices ?? []).some((d) => d.serial === state.serial)
  const isFb = (env.fastbootDevices ?? []).includes(state.serial)
  if (isFb && !isAdb) {
    state.mode = 'fastboot'
  } else if (isAdb) {
    state.mode = 'adb'
  }
}

export function selectDevice(serial: string) {
  state.serial = serial
  syncMode()
  const cfg = state.env?.config
  if (cfg) {
    cfg.lastSerial = serial
    api.saveConfig(cfg).catch(() => {})
  }
}

function startAutoRefresh() {
  if (refreshTimer) window.clearInterval(refreshTimer)
  refreshTimer = window.setInterval(() => {
    if (!state.env?.config.autoRefresh) return
    if (state.busy) return
    refresh()
  }, 4000)
}

/** 统一包装耗时操作：置忙、捕获错误、提示 */
export async function run<T>(label: string, fn: () => Promise<T>): Promise<T | null> {
  if (state.busy) {
    toast('warn', '已有操作正在进行')
    return null
  }
  state.busy = true
  try {
    const res = await fn()
    if (label) toast('ok', label)
    return res
  } catch (e: any) {
    const msg = typeof e === 'string' ? e : e?.message ?? String(e)
    toast('err', msg, 6000)
    pushLog({ level: 'err', text: msg, time: new Date().toLocaleTimeString('zh-CN', { hour12: false }) })
    return null
  } finally {
    state.busy = false
    refresh()
  }
}

export function needDevice(): boolean {
  if (!state.serial) {
    toast('warn', '请先选择设备')
    return false
  }
  return true
}

export function needAdb(): boolean {
  if (!needDevice()) return false
  if (state.mode !== 'adb') {
    toast('warn', '该功能需要设备处于 ADB 模式')
    return false
  }
  return true
}

export function needFastboot(): boolean {
  if (!needDevice()) return false
  if (state.mode !== 'fastboot') {
    toast('warn', '该功能需要设备处于 Fastboot 模式')
    return false
  }
  return true
}

export function fmtBytes(n: number): string {
  if (!n || n < 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v >= 100 ? v.toFixed(0) : v.toFixed(1)} ${units[i]}`
}
