<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { Signal, Wifi, WifiOff, Eye, RotateCcw, Check } from 'lucide-vue-next'
import { api } from '../api'
import { state, toast, needAdb, run } from '../store'
import type { SysUIState } from '../types'

const s = ref<SysUIState | null>(null)
const busy = ref(false)

const icons = [
  { key: 'rotate', label: '旋转锁定' },
  { key: 'volume', label: '音量' },
  { key: 'bluetooth', label: '蓝牙' },
  { key: 'alarm_clock', label: '闹钟' },
  { key: 'zen', label: '勿扰' },
  { key: 'mute', label: '静音' },
  { key: 'speakerphone', label: '扬声器' },
  { key: 'wifi', label: 'Wi-Fi' },
  { key: 'mobile', label: '移动信号' },
  { key: 'airplane', label: '飞行模式' },
  { key: 'battery', label: '电池图标' },
  { key: 'nfc', label: 'NFC' },
  { key: 'cast', label: '投屏' },
  { key: 'location', label: '定位' },
  { key: 'headset', label: '耳机' },
  { key: 'vpn', label: 'VPN' },
  { key: 'hotspot', label: '热点' },
  { key: 'clock', label: '时钟' },
]

const selected = ref<string[]>([])

async function load() {
  if (!state.serial || state.mode !== 'adb') return
  try {
    const t = await api.getTweaks(state.serial)
    s.value = t.sysui
    selected.value = [...(t.sysui.iconBlacklist ?? [])]
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
  }
}

function toggleIcon(key: string) {
  const i = selected.value.indexOf(key)
  if (i >= 0) selected.value.splice(i, 1)
  else selected.value.push(key)
}

async function applyIcons() {
  if (!needAdb()) return
  await run(selected.value.length ? `已隐藏 ${selected.value.length} 个状态栏图标` : '状态栏图标已全部显示', () =>
    api.setIconBlacklist(state.serial, selected.value),
  )
  await load()
}

async function toggleFlag(field: 'clockSeconds' | 'batteryPercent' | 'immersive' | 'captiveDisabled') {
  if (!needAdb() || !s.value) return
  const next = !s.value[field]
  busy.value = true
  try {
    if (field === 'clockSeconds') await api.setClockSeconds(state.serial, next)
    if (field === 'batteryPercent') await api.setBatteryPercent(state.serial, next)
    if (field === 'immersive') await api.setImmersive(state.serial, next)
    if (field === 'captiveDisabled') await api.setCaptivePortal(state.serial, next)
    toast('ok', '已更新')
    await load()
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
  } finally {
    busy.value = false
  }
}

async function setQuick(n: number) {
  if (!needAdb()) return
  await run(`快捷开关数 → ${n}`, () => api.setQuickCount(state.serial, n))
  await load()
}

async function resetAll() {
  if (!needAdb()) return
  await run('系统界面设置已恢复', async () => {
    await api.setIconBlacklist(state.serial, [])
    await api.setClockSeconds(state.serial, false)
    await api.setBatteryPercent(state.serial, false)
    await api.setImmersive(state.serial, false)
    await api.setCaptivePortal(state.serial, false)
  })
  await load()
}

watch(() => [state.serial, state.mode], () => void load())
onMounted(() => void load())
</script>

<template>
  <div class="p-5">
    <div class="mb-4 flex items-end justify-between">
      <div>
        <h1 class="text-[15px] font-medium text-slate-100">界面定制</h1>
        <p class="mt-1 text-[11.5px] text-muted">
          状态栏图标、时钟、电量显示、快捷开关、沉浸模式与 Wi-Fi 检测。
        </p>
      </div>
      <div class="flex gap-2">
        <button class="btn" @click="load"><RotateCcw class="h-3.5 w-3.5" /> 重新读取</button>
        <button class="btn btn-danger" @click="resetAll">全部恢复</button>
      </div>
    </div>

    <div v-if="state.mode !== 'adb'" class="panel py-16 text-center text-[12px] text-muted">
      该功能需要设备处于 ADB 模式
    </div>

    <div v-else class="grid grid-cols-3 gap-4">
      <!-- 状态栏图标 -->
      <div class="panel col-span-2">
        <div class="panel-header">
          <span class="panel-title"><Signal class="h-3.5 w-3.5" /> 状态栏图标</span>
          <span class="text-[10px] text-muted">勾选 = 隐藏</span>
        </div>
        <div class="p-4">
          <div class="grid grid-cols-6 gap-1.5">
            <button
              v-for="i in icons"
              :key="i.key"
              class="flex items-center gap-1.5 rounded border px-2 py-1.5 text-[10.5px] transition-colors"
              :class="selected.includes(i.key)
                ? 'border-accent/50 bg-accent/15 text-accent'
                : 'border-ink-600 bg-ink-900 text-slate-400 hover:border-accent/30'"
              @click="toggleIcon(i.key)"
            >
              <Check v-if="selected.includes(i.key)" class="h-3 w-3" />
              <span v-else class="h-3 w-3 rounded-sm border border-ink-600" />
              {{ i.label }}
            </button>
          </div>
          <div class="mt-3 flex items-center justify-between">
            <span class="text-[10px] text-muted">
              已隐藏 {{ selected.length }} 个图标 —— 键值：icon_blacklist
            </span>
            <button class="btn btn-primary" @click="applyIcons">应用</button>
          </div>
        </div>
      </div>

      <!-- 开关 -->
      <div class="panel">
        <div class="panel-header"><span class="panel-title"><Eye class="h-3.5 w-3.5" /> 显示开关</span></div>
        <div class="p-4 space-y-1">
          <button class="flex w-full items-center justify-between rounded px-2 py-2 text-[11.5px] hover:bg-ink-800" @click="toggleFlag('clockSeconds')">
            <span class="text-slate-300">时钟显示秒</span>
            <span class="tag" :class="s?.clockSeconds ? 'tag-ok' : 'tag'">{{ s?.clockSeconds ? '开' : '关' }}</span>
          </button>
          <button class="flex w-full items-center justify-between rounded px-2 py-2 text-[11.5px] hover:bg-ink-800" @click="toggleFlag('batteryPercent')">
            <span class="text-slate-300">状态栏电量百分比</span>
            <span class="tag" :class="s?.batteryPercent ? 'tag-ok' : 'tag'">{{ s?.batteryPercent ? '开' : '关' }}</span>
          </button>
          <button class="flex w-full items-center justify-between rounded px-2 py-2 text-[11.5px] hover:bg-ink-800" @click="toggleFlag('immersive')">
            <span class="text-slate-300">全局沉浸模式</span>
            <span class="tag" :class="s?.immersive ? 'tag-ok' : 'tag'">{{ s?.immersive ? '开' : '关' }}</span>
          </button>
          <button class="flex w-full items-center justify-between rounded px-2 py-2 text-[11.5px] hover:bg-ink-800" @click="toggleFlag('captiveDisabled')">
            <span class="flex items-center gap-1.5 text-slate-300">
              <WifiOff class="h-3 w-3" /> 关闭 Wi-Fi 网络检测
            </span>
            <span class="tag" :class="s?.captiveDisabled ? 'tag-ok' : 'tag'">{{ s?.captiveDisabled ? '开' : '关' }}</span>
          </button>
        </div>
      </div>

      <!-- 快捷开关 -->
      <div class="panel col-span-3">
        <div class="panel-header"><span class="panel-title"><Wifi class="h-3.5 w-3.5" /> 下拉快捷开关数量</span></div>
        <div class="p-4">
          <div class="flex items-center gap-2">
            <button v-for="n in [3, 4, 5, 6, 7, 8]" :key="n" class="btn flex-1" :class="s?.quickCount === n ? 'btn-primary' : ''" @click="setQuick(n)">
              {{ n }}
            </button>
          </div>
          <p class="mt-3 text-[10px] leading-relaxed text-muted">
            控制下拉通知栏第一屏的快捷开关数量，仅在部分 ROM 上生效（键值 sysui_qqs_count）。
            关闭 Wi-Fi 检测后，连接无外网的 Wi-Fi 不会再显示 x 或感叹号。
          </p>
        </div>
      </div>
    </div>
  </div>
</template>
