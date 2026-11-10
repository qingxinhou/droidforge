/**
 * 命令面板的开关状态。
 *
 * 单独放一个模块，是因为侧栏的搜索按钮、顶栏、全局快捷键都要能打开它，
 * 而组件之间不该互相 import。
 */
import { reactive } from 'vue'

export const paletteState = reactive({ open: false })

export function openPalette() {
  paletteState.open = true
}

export function closePalette() {
  paletteState.open = false
}

export function togglePalette() {
  paletteState.open = !paletteState.open
}
