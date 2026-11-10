/**
 * 主题切换。
 *
 * 深色 / 浅色由 <html class="light"> 控制，具体颜色全部在 src/style.css 的
 * CSS 变量里，组件不用关心当前是哪个主题。
 *
 * 'system' 会跟随操作系统外观（Windows 的"应用模式" / macOS 的外观）。
 */
import { reactive } from 'vue'

export type ThemeMode = 'dark' | 'light' | 'system'

const STORAGE_KEY = 'droidforge.theme'

export const themeState = reactive({
  mode: 'dark' as ThemeMode,
  /** 实际生效的外观，'system' 会被解析成 dark 或 light */
  scheme: 'dark' as 'dark' | 'light',
})

const lightQuery = () => window.matchMedia('(prefers-color-scheme: light)')

function resolve(mode: ThemeMode): 'dark' | 'light' {
  if (mode === 'system') return lightQuery().matches ? 'light' : 'dark'
  return mode
}

let watching = false

function watchSystem() {
  if (watching) return
  watching = true
  lightQuery().addEventListener('change', () => {
    if (themeState.mode === 'system') applyTheme('system', false)
  })
}

/**
 * 应用主题。
 * @param persist 是否写入 localStorage（'system' 的跟随变化不该反复写）
 */
export function applyTheme(mode: ThemeMode, persist = true) {
  const scheme = resolve(mode)
  themeState.mode = mode
  themeState.scheme = scheme

  const root = document.documentElement
  root.classList.toggle('light', scheme === 'light')
  root.style.colorScheme = scheme

  if (persist) {
    try {
      localStorage.setItem(STORAGE_KEY, mode)
    } catch {
      /* 隐私模式或磁盘只读，忽略 */
    }
  }
  if (mode === 'system') watchSystem()
}

/** 首帧之前先按本地缓存套一次，避免从浅色切到深色时闪一下。 */
export function bootstrapTheme() {
  // 没存过（首次启动、清过缓存）就跟随系统：白天不会一上来就怼一块黑界面。
  let saved: ThemeMode = 'system'
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw === 'dark' || raw === 'light' || raw === 'system') saved = raw
  } catch {
    /* ignore */
  }
  applyTheme(saved, false)
}

/** 在深色 / 浅色之间往返；当前是跟随系统时，切到与系统相反的那一侧。 */
export function toggleTheme(): ThemeMode {
  const next: ThemeMode = themeState.scheme === 'dark' ? 'light' : 'dark'
  applyTheme(next)
  return next
}
