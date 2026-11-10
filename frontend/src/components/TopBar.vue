<script setup lang="ts">
import { computed, ref } from 'vue'
import { ChevronDown, RefreshCw, Smartphone, Usb, Loader2, AlertTriangle, Sun, Moon } from 'lucide-vue-next'
import { state, devices, currentDevice, refresh, selectDevice, toolsReady, toast, maskSerial, setTheme } from '../store'
import { themeState, toggleTheme } from '../theme'
import { api } from '../api'

const showList = ref(false)

/** 深色时显示太阳（点击变亮），浅色时显示月亮 */
function flipTheme() {
  setTheme(toggleTheme())
}

const label = computed(() => {
  const d = currentDevice.value
  if (!d) return '未连接设备'
  const serial = maskSerial(d.serial)
  const name = d.model ? `${d.model} · ${serial}` : serial
  return name
})

const stateText = (s: string) => {
  switch (s) {
    case 'device': return '在线'
    case 'fastboot': return 'Fastboot'
    case 'unauthorized': return '未授权'
    case 'offline': return '离线'
    default: return s
  }
}

function pick(serial: string) {
  selectDevice(serial)
  showList.value = false
}

async function doRefresh() {
  await refresh()
  toast('info', '设备列表已刷新', 1600)
}

async function cancel() {
  await api.cancelOperation()
}
</script>

<template>
  <header class="flex h-14 shrink-0 items-center gap-3 border-b border-ink-700 bg-ink-900 px-4">
    <!-- 设备选择 -->
    <div class="relative">
      <button
        class="flex h-9 min-w-[260px] items-center gap-2 rounded border border-ink-600 bg-ink-850 px-2.5 text-left transition-colors hover:border-ink-600 hover:bg-ink-800"
        @click="showList = !showList"
      >
        <Smartphone class="h-4 w-4 shrink-0 text-muted" />
        <span class="min-w-0 flex-1 truncate text-[12px] text-slate-200">{{ label }}</span>
        <span
          v-if="currentDevice"
          class="tag"
          :class="currentDevice.state === 'device' ? 'tag-ok' : currentDevice.state === 'fastboot' ? 'tag-accent' : 'tag-warn'"
        >
          {{ stateText(currentDevice.state) }}
        </span>
        <ChevronDown class="h-3.5 w-3.5 shrink-0 text-muted" />
      </button>

      <div v-if="showList" class="fixed inset-0 z-30" @click="showList = false" />
      <div
        v-if="showList"
        class="absolute left-0 top-10 z-40 w-[340px] overflow-hidden rounded border border-ink-600 bg-ink-850 shadow-2xl"
      >
        <div class="border-b border-ink-700 px-3 py-2 text-[10.5px] uppercase tracking-wider text-muted">
          已发现设备
        </div>
        <div v-if="devices.length === 0" class="px-3 py-6 text-center text-[12px] text-muted">
          未检测到设备，请连接 USB 并开启 USB 调试
        </div>
        <button
          v-for="d in devices"
          :key="d.serial"
          class="flex w-full items-center gap-2 px-3 py-2 text-left hover:bg-ink-800"
          @click="pick(d.serial)"
        >
          <Usb class="h-3.5 w-3.5 text-muted" />
          <div class="min-w-0 flex-1">
            <div class="truncate text-[12px] text-slate-200">{{ d.model || d.serial }}</div>
            <div class="truncate text-[10px] text-muted">{{ maskSerial(d.serial) }}</div>
          </div>
          <span
            class="tag"
            :class="d.state === 'device' ? 'tag-ok' : d.state === 'fastboot' ? 'tag-accent' : 'tag-warn'"
          >{{ stateText(d.state) }}</span>
        </button>
      </div>
    </div>

    <!-- 模式 -->
    <div class="flex items-center rounded border border-ink-600 bg-ink-850 p-0.5">
      <span
        class="rounded-sm px-2 py-1 text-[11px]"
        :class="state.mode === 'adb' ? 'bg-accent/20 text-accent' : 'text-muted'"
      >ADB</span>
      <span
        class="rounded-sm px-2 py-1 text-[11px]"
        :class="state.mode === 'fastboot' ? 'bg-accent/20 text-accent' : 'text-muted'"
      >Fastboot</span>
    </div>

    <div class="flex-1" />

    <!-- 进度 -->
    <div v-if="state.progress" class="flex w-[220px] items-center gap-2">
      <div class="min-w-0 flex-1">
        <div class="mb-1 flex justify-between text-[10px] text-muted">
          <span class="truncate">{{ state.progress.label }}</span>
          <span class="mono ml-2">{{ state.progress.percent }}%</span>
        </div>
        <div class="h-1 overflow-hidden rounded-sm bg-ink-700">
          <div
            class="h-full bg-accent transition-all duration-200"
            :style="{ width: Math.max(2, state.progress.percent) + '%' }"
          />
        </div>
      </div>
    </div>

    <!-- 工具状态 -->
    <div
      class="flex items-center gap-1.5 rounded border px-2 py-1 text-[11px]"
      :class="toolsReady ? 'border-ok/30 bg-ok/10 text-ok' : 'border-warn/30 bg-warn/10 text-warn'"
      :title="state.env?.tools.hint"
    >
      <AlertTriangle v-if="!toolsReady" class="h-3.5 w-3.5" />
      <span v-else class="h-1.5 w-1.5 rounded-full bg-ok" />
      {{ toolsReady ? '工具就绪' : '缺少平台工具' }}
    </div>

    <!-- 忙碌 -->
    <button v-if="state.busy" class="btn btn-sm btn-danger" @click="cancel">
      <Loader2 class="h-3.5 w-3.5 animate-spin" />
      中止
    </button>
    <button v-else class="btn btn-icon" title="刷新设备" @click="doRefresh">
      <RefreshCw class="h-4 w-4" />
    </button>

    <!-- 主题 -->
    <button
      class="btn btn-icon"
      :title="themeState.scheme === 'dark' ? '切换到浅色' : '切换到深色'"
      @click="flipTheme"
    >
      <Sun v-if="themeState.scheme === 'dark'" class="h-4 w-4" />
      <Moon v-else class="h-4 w-4" />
    </button>
  </header>
</template>
