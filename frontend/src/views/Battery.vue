<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  BatteryCharging, BatteryFull, Zap, Thermometer, Activity,
  Play, Square, Download, RotateCcw, FlaskConical,
} from 'lucide-vue-next'
import { api } from '../api'
import { state, toast, needAdb, run } from '../store'
import type { BatteryInfo } from '../types'

interface Sample {
  t: number
  level: number
  power: number
  current: number
  temp: number
}

const info = ref<BatteryInfo | null>(null)
const samples = ref<Sample[]>([])
const recording = ref(false)
const intervalMs = ref(2000)
const mock = ref({ level: 50, ac: false, usb: false, discharging: false })

let timer: number | undefined

const avgPower = computed(() => {
  if (samples.value.length === 0) return 0
  const sum = samples.value.reduce((a, s) => a + s.power, 0)
  return sum / samples.value.length
})
const maxPower = computed(() => samples.value.reduce((a, s) => Math.max(a, s.power), 0))

// Android 10+ 的 SELinux 策略会拒绝 shell 读 /sys/class/power_supply/*/current_now，
// 此时 currentUa 恒为 0。不能把它当成「0 mA 真实读数」展示出去。
const currentReadable = computed(() => (info.value?.currentUa ?? 0) !== 0)

const levelPath = computed(() => buildPath(samples.value.map((s) => s.level), 100))
const powerPath = computed(() => buildPath(samples.value.map((s) => s.power), Math.max(5, maxPower.value)))

function buildPath(values: number[], max: number): string {
  if (values.length < 2) return ''
  const w = 320
  const h = 56
  const step = w / (values.length - 1)
  return values
    .map((v, i) => {
      const x = i * step
      const y = h - Math.min(1, Math.max(0, v / (max || 1))) * h
      return `${i === 0 ? 'M' : 'L'}${x.toFixed(1)},${y.toFixed(1)}`
    })
    .join(' ')
}

async function load() {
  if (!state.serial || state.mode !== 'adb') return
  try {
    info.value = await api.readBattery(state.serial)
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
  }
}

function record() {
  if (!info.value) return
  samples.value.push({
    t: Date.now(),
    level: info.value.level,
    power: +info.value.powerW.toFixed(2),
    current: +info.value.currentMa.toFixed(0),
    temp: info.value.temperatureC,
  })
  if (samples.value.length > 900) samples.value.shift()
}

function startRecording() {
  if (!needAdb()) return
  recording.value = true
  samples.value = []
  void load().then(record)
  timer = window.setInterval(async () => {
    await load()
    record()
  }, intervalMs.value)
}

function stopRecording() {
  recording.value = false
  if (timer) window.clearInterval(timer)
  timer = undefined
}

async function applyMock() {
  if (!needAdb()) return
  await run('电池模拟已应用', () =>
    api.mockBattery(state.serial, {
      level: mock.value.level,
      ac: mock.value.ac,
      usb: mock.value.usb,
      wireless: null,
      unplug: false,
      fullReset: false,
      discharging: mock.value.discharging,
    }),
  )
  await load()
}

async function reset() {
  if (!needAdb()) return
  await run('电池状态已恢复', () => api.resetBattery(state.serial))
  await load()
}

async function exportCsv() {
  if (samples.value.length === 0) {
    toast('warn', '还没有采样数据')
    return
  }
  const rows = ['时间,电量(%),电压(V),电流(mA),功率(W),温度(℃)']
  for (const s of samples.value) {
    const d = new Date(s.t)
    rows.push(
      [
        d.toLocaleTimeString('zh-CN', { hour12: false }),
        s.level,
        info.value?.voltageV?.toFixed(3) ?? '',
        s.current,
        s.power,
        s.temp,
      ].join(','),
    )
  }
  const name = `${state.serial || 'battery'}_${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}.csv`
  const p = await run('CSV 已导出', () => api.saveTextFile(name, rows.join('\n')))
  if (p) await api.revealInFolder(p)
}

const statusColor = computed(() => {
  const s = info.value?.status ?? 0
  if (s === 2) return 'text-ok'
  if (s === 3) return 'text-warn'
  if (s === 5) return 'text-accent'
  return 'text-slate-300'
})

watch(() => intervalMs.value, () => {
  if (recording.value) {
    stopRecording()
    startRecording()
  }
})
watch(() => [state.serial, state.mode], () => void load())
onMounted(() => void load())
onBeforeUnmount(stopRecording)
</script>

<template>
  <div class="p-5">
    <div class="mb-4 flex items-end justify-between">
      <div>
        <h1 class="text-[15px] font-medium text-slate-100">电池管理</h1>
        <p class="mt-1 text-[11.5px] text-muted">
          实时读数、功率采样与 CSV 导出；也可以模拟电量给截图或测试用。
        </p>
      </div>
      <div class="flex gap-2">
        <button class="btn" @click="load"><RotateCcw class="h-3.5 w-3.5" /> 刷新</button>
        <button v-if="!recording" class="btn btn-primary" @click="startRecording">
          <Play class="h-3.5 w-3.5" /> 开始统计
        </button>
        <button v-else class="btn btn-danger" @click="stopRecording">
          <Square class="h-3.5 w-3.5" /> 停止（{{ samples.length }} 点）
        </button>
        <button class="btn" @click="exportCsv"><Download class="h-3.5 w-3.5" /> 导出 CSV</button>
      </div>
    </div>

    <div v-if="state.mode !== 'adb'" class="panel py-16 text-center text-[12px] text-muted">
      该功能需要设备处于 ADB 模式
    </div>

    <div v-else class="grid grid-cols-4 gap-4">
      <!-- 电量总览 -->
      <div class="panel col-span-2 p-4">
        <div class="flex items-center gap-5">
          <div class="relative flex h-[92px] w-[92px] items-center justify-center">
            <svg viewBox="0 0 100 100" class="absolute inset-0 -rotate-90">
              <circle cx="50" cy="50" r="42" fill="none" stroke="var(--chart-track)" stroke-width="8" />
              <circle
                cx="50" cy="50" r="42" fill="none"
                :stroke="info?.status === 2 ? '#22c55e' : '#38bdf8'"
                stroke-width="8" stroke-linecap="round"
                :stroke-dasharray="`${(info?.level ?? 0) * 2.64} 264`"
              />
            </svg>
            <div class="text-center">
              <div class="text-[26px] font-medium leading-none text-slate-100">{{ info?.level ?? '—' }}</div>
              <div class="mt-1 text-[9px] text-muted">%</div>
            </div>
          </div>
          <div class="flex-1">
            <div class="flex items-center gap-1.5 text-[13px]" :class="statusColor">
              <BatteryCharging v-if="info?.status === 2" class="h-4 w-4" />
              <BatteryFull v-else class="h-4 w-4" />
              {{ info?.statusText || '—' }}
            </div>
            <div class="mt-2 kv"><span class="kv-k">健康</span><span class="kv-v">{{ info?.healthText || '—' }}</span></div>
            <div class="kv"><span class="kv-k">电压</span><span class="kv-v mono">{{ info?.voltageV?.toFixed(3) || '—' }} V</span></div>
            <div class="kv"><span class="kv-k">温度</span><span class="kv-v mono">{{ info?.temperatureC?.toFixed(1) || '—' }} ℃</span></div>
            <div class="kv"><span class="kv-k">供电</span>
              <span class="kv-v">
                {{ info?.acPowered ? 'AC' : '' }}
                {{ info?.usbPowered ? 'USB' : '' }}
                {{ info?.wirelessPowered ? '无线' : '' }}
                <span v-if="!info?.acPowered && !info?.usbPowered && !info?.wirelessPowered">电池</span>
              </span>
            </div>
          </div>
        </div>
      </div>

      <!-- 功率 -->
      <div class="panel col-span-2 p-4">
        <div class="mb-2 flex items-center justify-between">
          <span class="flex items-center gap-1.5 text-[11.5px] text-slate-300"><Zap class="h-3.5 w-3.5" /> 功率</span>
          <span class="text-[10px] text-muted">采样间隔 {{ intervalMs / 1000 }}s</span>
        </div>
        <div class="flex items-end gap-4">
          <div>
            <div class="text-[22px] font-medium leading-none text-accent">{{ currentReadable ? info?.powerW?.toFixed(2) : '—' }}<span class="ml-1 text-[10px] text-muted">W</span></div>
            <div class="mt-1 text-[10px] text-muted">当前</div>
          </div>
          <div>
            <div class="text-[16px] leading-none text-slate-200">{{ avgPower.toFixed(2) }}<span class="ml-1 text-[10px] text-muted">W</span></div>
            <div class="mt-1 text-[10px] text-muted">平均</div>
          </div>
          <div>
            <div class="text-[16px] leading-none text-slate-200">{{ maxPower.toFixed(2) }}<span class="ml-1 text-[10px] text-muted">W</span></div>
            <div class="mt-1 text-[10px] text-muted">峰值</div>
          </div>
          <div class="ml-auto text-right">
            <div class="text-[16px] leading-none text-slate-200">{{ currentReadable ? info?.currentMa?.toFixed(0) : '—' }}<span class="ml-1 text-[10px] text-muted">mA</span></div>
            <div class="mt-1 text-[10px] text-muted">电流</div>
          </div>
        </div>
        <svg viewBox="0 0 320 60" class="mt-3 h-[60px] w-full">
          <path v-if="currentReadable && powerPath" :d="powerPath" fill="none" stroke="#38bdf8" stroke-width="1.5" />
        </svg>
        <div v-if="!currentReadable" class="mt-1 text-[10px] text-muted">
          当前设备的电流节点被系统拒绝读取（Android 10+ 限制），实时功率不可用；电压、温度与电量仍然准确。
        </div>
      </div>

      <!-- 电量曲线 -->
      <div class="panel col-span-3 p-4">
        <div class="mb-2 flex items-center justify-between">
          <span class="flex items-center gap-1.5 text-[11.5px] text-slate-300"><Activity class="h-3.5 w-3.5" /> 电量曲线</span>
          <span class="text-[10px] text-muted">{{ samples.length }} 个采样点</span>
        </div>
        <svg viewBox="0 0 320 60" class="h-[90px] w-full">
          <defs>
            <linearGradient id="lvl" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stop-color="#22c55e" stop-opacity="0.35" />
              <stop offset="100%" stop-color="#22c55e" stop-opacity="0" />
            </linearGradient>
          </defs>
          <path v-if="levelPath" :d="levelPath" fill="none" stroke="#22c55e" stroke-width="1.5" />
        </svg>
        <div v-if="samples.length === 0" class="mt-1 text-[10px] text-muted">
          点「开始统计」后按间隔采样，可导出 CSV；页面关闭后数据不保留。
        </div>
      </div>

      <!-- 模拟 -->
      <div class="panel p-4">
        <div class="mb-2 flex items-center gap-1.5 text-[11.5px] text-slate-300">
          <FlaskConical class="h-3.5 w-3.5" /> 电池模拟
        </div>
        <div class="space-y-2">
          <div>
            <div class="mb-1 text-[10px] text-muted">电量</div>
            <div class="flex items-center gap-1">
              <input v-model.number="mock.level" type="range" min="1" max="100" class="flex-1 accent-accent" />
              <span class="mono w-8 text-right text-[11px] text-slate-200">{{ mock.level }}</span>
            </div>
          </div>
          <label class="flex cursor-pointer items-center gap-1.5 text-[10.5px] text-muted">
            <input v-model="mock.ac" type="checkbox" class="accent-accent" /> 模拟 AC 供电
          </label>
          <label class="flex cursor-pointer items-center gap-1.5 text-[10.5px] text-muted">
            <input v-model="mock.usb" type="checkbox" class="accent-accent" /> 模拟 USB 供电
          </label>
          <label class="flex cursor-pointer items-center gap-1.5 text-[10.5px] text-muted">
            <input v-model="mock.discharging" type="checkbox" class="accent-accent" /> 标记为放电中
          </label>
          <div class="flex gap-1.5 pt-1">
            <button class="btn btn-sm flex-1" @click="applyMock">应用</button>
            <button class="btn btn-sm btn-ok flex-1" @click="reset"><RotateCcw class="h-3 w-3" /> 恢复</button>
          </div>
        </div>
      </div>
    </div>

    <div class="panel mt-4 flex items-start gap-2 border-warn/20 bg-warn/5 px-3 py-2.5">
      <Thermometer class="mt-0.5 h-4 w-4 shrink-0 text-warn" />
      <div class="text-[10.5px] leading-relaxed text-muted">
        模拟值只改系统上报，不改变真实电量；拔线或重启后一般会自动恢复，建议用完点「恢复」。
        功率由电压 × 电流估算，不等于外接功率计的实测值。
      </div>
    </div>
  </div>
</template>
