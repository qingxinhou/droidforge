<script setup lang="ts">
import { computed } from 'vue'
import {
  Power,
  RotateCcw,
  Layers,
  Zap,
  Download,
  PowerOff,
  Usb,
  ShieldAlert,
} from 'lucide-vue-next'
import { api } from '../api'
import { state, run, needDevice, toast } from '../store'
import { confirmDialog } from '../dialog'

interface Target {
  key: string
  label: string
  desc: string
  icon: any
  danger?: boolean
  confirm?: string
}

const adbTargets: Target[] = [
  { key: '', label: '重启系统', desc: '正常重启到 Android 系统', icon: RotateCcw },
  { key: 'recovery', label: 'Recovery', desc: '进入恢复模式', icon: Layers },
  { key: 'bootloader', label: 'Bootloader', desc: '进入 fastboot 模式，用于刷机', icon: Usb },
  { key: 'fastboot', label: 'fastbootd', desc: '进入用户空间 fastbootd，刷动态分区', icon: Usb },
  { key: 'edl', label: 'EDL 9008', desc: '高通紧急下载模式，救砖用', icon: Zap, danger: true, confirm: 'EDL 模式通常需要授权或短接，进入后设备可能无响应，需专用工具退出。确认继续？' },
  { key: 'sideload', label: 'Sideload', desc: '进入 recovery sideload 模式', icon: Download, danger: true, confirm: 'Sideload 模式需要设备已进入 Recovery，确认继续？' },
  { key: '__shutdown', label: '关机', desc: '关闭设备电源', icon: PowerOff, danger: true, confirm: '确认关闭设备？' },
]

const fbTargets: Target[] = [
  { key: '', label: '重启系统', desc: '从 bootloader 重启到系统', icon: RotateCcw },
  { key: 'bootloader', label: 'Bootloader', desc: '重启回 bootloader', icon: Usb },
  { key: 'recovery', label: 'Recovery', desc: '进入恢复模式', icon: Layers },
  { key: 'fastboot', label: 'fastbootd', desc: '进入 fastbootd', icon: Usb },
  { key: 'edl', label: 'EDL 9008', desc: '进入紧急下载模式', icon: Zap, danger: true, confirm: 'EDL 模式风险较高，确认继续？' },
]

const targets = computed(() => (state.mode === 'fastboot' ? fbTargets : adbTargets))

async function trigger(t: Target) {
  if (!needDevice()) return
  if (t.danger) {
    const ok = await confirmDialog({
      title: '危险操作确认',
      message: t.confirm ?? '该操作可能导致设备无法正常启动，确认继续？',
      danger: true,
      confirmText: '仍然执行',
    })
    if (!ok) return
  }

  if (state.mode === 'fastboot') {
    await run(`已发送：${t.label}`, () => api.fastbootReboot(state.serial, t.key))
    return
  }
  if (t.key === '__shutdown') {
    await run('已发送关机指令', () => api.adbShell(state.serial, 'reboot -p'))
    return
  }
  await run(`已发送：${t.label}`, () => api.adbReboot(state.serial, t.key))
}

function isDanger(key: string) {
  return key === 'edl' || key === 'sideload' || key === '__shutdown'
}
</script>

<template>
  <div class="p-5">
    <div class="mb-4">
      <h1 class="text-[15px] font-medium text-slate-100">重启与模式</h1>
      <p class="mt-1 text-[11.5px] text-muted">
        当前模式：<span class="text-slate-300">{{ state.mode === 'adb' ? 'ADB' : 'Fastboot' }}</span>
        · 不同模式支持的目标略有差异
      </p>
    </div>

    <div class="grid grid-cols-3 gap-3">
      <button
        v-for="t in targets"
        :key="t.key"
        class="panel card-hover flex items-start gap-3 p-4 text-left"
        @click="trigger(t)"
      >
        <div
          class="flex h-9 w-9 shrink-0 items-center justify-center rounded border"
          :class="isDanger(t.key) ? 'border-danger/30 bg-danger/10 text-danger' : 'border-ink-600 bg-ink-800 text-accent'"
        >
          <component :is="t.icon" class="h-4 w-4" />
        </div>
        <div class="min-w-0">
          <div class="text-[12.5px] text-slate-200">{{ t.label }}</div>
          <div class="mt-0.5 text-[10.5px] leading-relaxed text-muted">{{ t.desc }}</div>
        </div>
      </button>
    </div>

    <div class="panel mt-4 flex items-start gap-2 border-warn/20 bg-warn/5 px-4 py-3">
      <ShieldAlert class="mt-0.5 h-4 w-4 shrink-0 text-warn" />
      <div class="text-[11.5px] leading-relaxed text-slate-300">
        <div class="text-warn">提示</div>
        <div class="mt-1 text-muted">
          进入 Bootloader 后，ADB 功能将不可用，请在本工具右上角切换到 Fastboot 模式继续操作。
          EDL / Sideload 属于特殊模式，普通刷机请勿使用。
        </div>
      </div>
    </div>
  </div>
</template>
