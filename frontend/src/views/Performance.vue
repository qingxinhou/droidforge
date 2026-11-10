<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Cpu, RefreshCw, Gauge } from 'lucide-vue-next'
import { api } from '../api'
import { state, toast } from '../store'
import type { CpuSnapshot } from '../types'

const snap = ref<CpuSnapshot | null>(null)
const intervalMs = ref(state.env?.config.cpuIntervalMs ?? 1500)
const error = ref('')
let timer: number | undefined

const clusters = computed(() => snap.value?.clusters ?? [])
const cores = computed(() => snap.value?.cores ?? [])

const maxFreq = computed(() => cores.value.reduce((a, c) => Math.max(a, c.max), 0))

function fmt(f: number): string {
  if (!f) return '—'
  if (f >= 1_000_000) return (f / 1_000_000).toFixed(2) + ' GHz'
  return Math.round(f / 1000) + ' MHz'
}

function pct(core: { cur: number; max: number }): number {
  if (!core.max) return 0
  return Math.min(100, Math.round((core.cur / core.max) * 100))
}

function coreLoad(index: number): number {
  const c = cores.value.find((x) => x.index === index)
  return c ? pct(c) : 0
}

function coreFreq(index: number): number {
  const c = cores.value.find((x) => x.index === index)
  return c?.cur ?? 0
}

async function poll() {
  if (!state.serial || state.mode !== 'adb') return
  try {
    snap.value = await api.readCpu(state.serial)
    error.value = ''
  } catch (e: any) {
    error.value = e?.message ?? String(e)
  }
}

function restart() {
  if (timer) window.clearInterval(timer)
  timer = window.setInterval(poll, intervalMs.value)
}

async function saveInterval(ms: number) {
  intervalMs.value = ms
  restart()
  const cfg = state.env?.config
  if (cfg) {
    cfg.cpuIntervalMs = ms
    try {
      await api.saveConfig(cfg)
    } catch {
      /* 忽略持久化失败 */
    }
  }
  toast('ok', `刷新间隔 ${ms} ms`)
}

watch(() => [state.serial, state.mode], () => void poll())
onMounted(() => {
  void poll()
  restart()
})
onBeforeUnmount(() => {
  if (timer) window.clearInterval(timer)
})
</script>

<template>
  <div class="p-5">
    <div class="mb-4 flex items-end justify-between">
      <div>
        <h1 class="text-[15px] font-medium text-slate-100">性能监控</h1>
        <p class="mt-1 text-[11.5px] text-muted">读取各核心实时频率与调频策略，用于观察调度行为。</p>
      </div>
      <div class="flex items-center gap-2">
        <span class="text-[10px] text-muted">刷新</span>
        <button
          v-for="ms in [500, 1000, 2000, 5000]"
          :key="ms"
          class="btn btn-sm"
          :class="intervalMs === ms ? 'btn-primary' : ''"
          @click="saveInterval(ms)"
        >
          {{ ms }}ms
        </button>
        <button class="btn btn-sm" @click="poll"><RefreshCw class="h-3 w-3" /> 立即</button>
      </div>
    </div>

    <div v-if="state.mode !== 'adb'" class="panel py-16 text-center text-[12px] text-muted">
      该功能需要设备处于 ADB 模式
    </div>

    <div v-else-if="error" class="panel border-danger/30 px-4 py-8 text-center text-[12px] text-danger">
      {{ error }}
      <div class="mt-2 text-[10px] text-muted">部分设备不开放 cpufreq 节点，此时无法读取频率。</div>
    </div>

    <div v-else class="space-y-4">
      <!-- 簇 -->
      <div class="grid grid-cols-3 gap-4">
        <div v-for="c in clusters" :key="c.index" class="panel p-4">
          <div class="mb-3 flex items-center justify-between">
            <span class="flex items-center gap-1.5 text-[11.5px] text-slate-300">
              <Gauge class="h-3.5 w-3.5" /> 簇 {{ c.index + 1 }}
            </span>
            <span class="tag">{{ c.governor || '—' }}</span>
          </div>
          <div class="text-[20px] font-medium leading-none text-accent">
            {{ fmt(c.cur) }}
            <span class="ml-1 text-[10px] text-muted">/ {{ fmt(c.max) }}</span>
          </div>
          <div class="mt-1 text-[10px] text-muted">
            核心 {{ (c.cores ?? []).join('、') }} · 最低 {{ fmt(c.min) }}
          </div>
          <div class="mt-3 h-1.5 overflow-hidden rounded bg-ink-700">
            <div class="h-full bg-accent" :style="{ width: pct(c) + '%' }" />
          </div>
        </div>
      </div>

      <!-- 核心 -->
      <div class="panel">
        <div class="panel-header">
          <span class="panel-title"><Cpu class="h-3.5 w-3.5" /> 各核心</span>
          <span class="text-[10px] text-muted">最高 {{ fmt(maxFreq) }}</span>
        </div>
        <div class="grid grid-cols-4 gap-3 p-4">
          <div v-for="c in cores" :key="c.index" class="rounded border border-ink-700 bg-ink-900 p-3">
            <div class="mb-1 flex items-center justify-between text-[10px]">
              <span class="text-slate-300">核心 {{ c.index }}</span>
              <span class="mono text-muted">{{ pct(c) }}%</span>
            </div>
            <div class="mono text-[13px] text-slate-100">{{ fmt(coreFreq(c.index)) }}</div>
            <div class="mt-2 h-1.5 overflow-hidden rounded bg-ink-700">
              <div
                class="h-full transition-[width] duration-300"
                :class="pct(c) > 85 ? 'bg-warn' : 'bg-accent'"
                :style="{ width: coreLoad(c.index) + '%' }"
              />
            </div>
            <div class="mt-1.5 flex justify-between text-[9px] text-muted">
              <span>{{ fmt(c.min) }}</span>
              <span>{{ fmt(c.max) }}</span>
            </div>
          </div>
        </div>
      </div>

      <div class="panel px-4 py-3 text-[10px] leading-relaxed text-muted">
        频率来自 <span class="mono text-slate-300">/sys/devices/system/cpu/cpu*/cpufreq</span>，
        读取本身不唤醒核心，但高频轮询（500ms）会增加一点 ADB 开销。
        想固定频率请用内核调度的 <span class="mono">scaling_governor</span>，本工具不主动改。
      </div>
    </div>
  </div>
</template>
