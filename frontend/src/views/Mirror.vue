<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import {
  AlertTriangle,
  CircleStop,
  FolderOpen,
  Loader2,
  Monitor,
  Play,
  RefreshCw,
  Video,
} from 'lucide-vue-next'
import { api } from '../api'
import { currentDevice, needAdb, toast } from '../store'
import type { ScreenBinary } from '../types'

const bin = ref<ScreenBinary | null>(null)
const checking = ref(false)
const starting = ref(false)
const running = ref(false)
const tail = ref('')

const bitRate = ref('8M')
const maxSize = ref(1024)
const maxFps = ref(60)
const turnScreenOff = ref(false)
const stayAwake = ref(true)
const noAudio = ref(true)
const alwaysOnTop = ref(false)
const record = ref('')

const serial = computed(() => currentDevice.value?.serial ?? '')

let timer: number | undefined

async function detect() {
  checking.value = true
  try {
    bin.value = await api.detectScrcpy()
    if (!bin.value.found) toast('warn', bin.value.hint, 8000)
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
  } finally {
    checking.value = false
  }
}

async function start() {
  if (!needAdb()) return
  starting.value = true
  try {
    const msg = await api.startScreen({
      serial: serial.value,
      adbPath: '',
      bitRate: bitRate.value,
      maxSize: maxSize.value,
      maxFps: maxFps.value,
      turnScreenOff: turnScreenOff.value,
      stayAwake: stayAwake.value,
      noAudio: noAudio.value,
      alwaysOnTop: alwaysOnTop.value,
      record: record.value,
    })
    running.value = true
    toast('ok', msg || '投屏已启动')
  } catch (e: any) {
    toast('err', e?.message ?? String(e), 8000)
  } finally {
    starting.value = false
  }
}

async function stop() {
  try {
    await api.stopScreen()
    running.value = false
    tail.value = ''
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
  }
}

async function refreshStatus() {
  try {
    const s = await api.screenStatus()
    // 进程自己退出（比如窗口被关掉）时也要反映出来。
    if (running.value && !s.running) {
      running.value = false
      if (s.tail) toast('info', '投屏已结束', 4000)
    }
    if (s.running) {
      running.value = true
      tail.value = s.tail
    }
  } catch {
    // 状态轮询失败不打扰用户
  }
}

async function pickRecord() {
  const p = await api.pickSaveFile('选择录制保存位置', 'scrcpy.mp4')
  if (p) record.value = p
}

onMounted(async () => {
  await detect()
  await refreshStatus()
  timer = window.setInterval(refreshStatus, 2500)
})

onBeforeUnmount(() => {
  if (timer) window.clearInterval(timer)
})
</script>

<template>
  <div>
    <div class="mb-4 flex items-start justify-between gap-4">
      <div>
        <h1 class="text-[17px] font-medium text-slate-100">投屏</h1>
        <p class="mt-1 text-[11.5px] text-muted">
          调用本机的 scrcpy 开一个独立窗口，参数在这里调，画面由 scrcpy 自己渲染
        </p>
      </div>
      <div class="flex gap-1.5">
        <button class="btn btn-sm" :disabled="checking" @click="detect">
          <RefreshCw class="h-3.5 w-3.5" :class="checking ? 'animate-spin' : ''" />重新检测
        </button>
        <button v-if="running" class="btn btn-danger btn-sm" @click="stop">
          <CircleStop class="h-3.5 w-3.5" />停止
        </button>
        <button v-else class="btn btn-primary btn-sm" :disabled="starting || !bin?.found" @click="start">
          <Loader2 v-if="starting" class="h-3.5 w-3.5 animate-spin" />
          <Play v-else class="h-3.5 w-3.5" />
          启动投屏
        </button>
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <span class="panel-title">scrcpy</span>
        <span v-if="bin" class="tag" :class="bin.found ? 'tag-ok' : 'tag-warn'">
          {{ bin.found ? '已找到' : '未安装' }}
        </span>
      </div>
      <div class="px-4 py-3">
        <div v-if="bin?.found" class="kv">
          <span class="kv-k">路径</span>
          <span class="kv-v mono truncate">{{ bin.path }}</span>
        </div>
        <div v-if="bin?.version" class="kv mt-1">
          <span class="kv-k">版本</span>
          <span class="kv-v mono">{{ bin.version }}</span>
        </div>
        <div v-if="!bin?.found" class="flex items-start gap-1.5 text-[11.5px] leading-relaxed text-warn">
          <AlertTriangle class="mt-0.5 h-3.5 w-3.5 shrink-0" />
          <span>{{ bin?.hint }}</span>
        </div>
        <div v-else-if="bin.hint" class="mt-2 text-[11px] text-warn">{{ bin.hint }}</div>
      </div>
    </div>

    <div class="panel mt-3">
      <div class="panel-head">
        <span class="panel-title">画面参数</span>
        <span class="text-[11px] text-muted">启动后改参数需要先停止</span>
      </div>
      <div class="grid grid-cols-3 gap-x-6 gap-y-3 px-4 py-3">
        <label class="block">
          <span class="mb-1 block text-[11px] text-muted">码率</span>
          <input v-model="bitRate" class="input mono w-full" placeholder="8M" />
        </label>
        <label class="block">
          <span class="mb-1 block text-[11px] text-muted">长边上限</span>
          <input v-model.number="maxSize" type="number" class="input mono w-full" placeholder="1024（0 = 原生）" />
        </label>
        <label class="block">
          <span class="mb-1 block text-[11px] text-muted">帧率上限</span>
          <input v-model.number="maxFps" type="number" class="input mono w-full" placeholder="60" />
        </label>
      </div>
      <div class="grid grid-cols-2 gap-x-6 gap-y-2 border-t border-ink-700 px-4 py-3">
        <label class="flex items-center gap-2 text-[11.5px] text-slate-300">
          <input v-model="turnScreenOff" type="checkbox" class="h-3.5 w-3.5 accent-accent" />
          投屏时关闭设备屏幕（省电，防止烧屏）
        </label>
        <label class="flex items-center gap-2 text-[11.5px] text-slate-300">
          <input v-model="stayAwake" type="checkbox" class="h-3.5 w-3.5 accent-accent" />
          保持设备常亮
        </label>
        <label class="flex items-center gap-2 text-[11.5px] text-slate-300">
          <input v-model="noAudio" type="checkbox" class="h-3.5 w-3.5 accent-accent" />
          不转发音频（需要 scrcpy 2.0+）
        </label>
        <label class="flex items-center gap-2 text-[11.5px] text-slate-300">
          <input v-model="alwaysOnTop" type="checkbox" class="h-3.5 w-3.5 accent-accent" />
          窗口置顶
        </label>
      </div>
      <div class="border-t border-ink-700 px-4 py-3">
        <div class="mb-1 text-[11px] text-muted">同时录制（留空则不录）</div>
        <div class="flex gap-2">
          <input class="input mono flex-1" :value="record" readonly placeholder="C:\...\scrcpy.mp4" />
          <button class="btn btn-sm" @click="pickRecord"><FolderOpen class="h-3.5 w-3.5" />选择</button>
          <button v-if="record" class="btn btn-sm" @click="record = ''">清除</button>
        </div>
      </div>
    </div>

    <div v-if="running" class="panel mt-3">
      <div class="panel-head">
        <span class="panel-title">运行中</span>
        <span class="tag tag-ok">PID 运行</span>
      </div>
      <div class="px-4 py-3 text-[11.5px] text-muted">
        <Monitor class="mr-1 inline h-3.5 w-3.5" />
        scrcpy 已开在独立窗口里。关掉那个窗口等同于停止投屏。
      </div>
      <pre v-if="tail" class="max-h-32 overflow-y-auto border-t border-ink-700 px-4 py-2 text-[10.5px] text-muted">{{ tail }}</pre>
    </div>

    <div v-if="record && running" class="mt-3 flex items-center gap-1.5 text-[11px] text-muted">
      <Video class="h-3.5 w-3.5" />正在录制到 {{ record }}
    </div>
  </div>
</template>
