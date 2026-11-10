<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { Play, Trash2, Copy, Terminal, ChevronUp } from 'lucide-vue-next'
import { api } from '../api'
import { state, toast, needAdb } from '../store'

interface Line {
  type: 'cmd' | 'out' | 'err' | 'info'
  text: string
}

const lines = ref<Line[]>([
  { type: 'info', text: 'ADB Shell 终端 · 输入命令后回车执行，支持 ↑ ↓ 翻阅历史' },
])
const input = ref('')
const history = ref<string[]>([])
const historyIndex = ref(-1)
const running = ref(false)
const bodyRef = ref<HTMLElement | null>(null)
const inputRef = ref<HTMLInputElement | null>(null)

const quick = [
  { label: '设备属性', cmd: 'getprop | grep ro.product' },
  { label: '已装应用', cmd: 'pm list packages -3' },
  { label: '电池状态', cmd: 'dumpsys battery' },
  { label: '存储占用', cmd: 'df -h /data' },
  { label: 'CPU 信息', cmd: 'cat /proc/cpuinfo | head -20' },
  { label: '内存信息', cmd: 'cat /proc/meminfo | head -5' },
  { label: '当前前台', cmd: 'dumpsys activity activities | grep mResumedActivity' },
  { label: '屏幕分辨率', cmd: 'wm size' },
]

async function scrollBottom() {
  await nextTick()
  if (bodyRef.value) bodyRef.value.scrollTop = bodyRef.value.scrollHeight
}

async function execute(cmd?: string) {
  const command = (cmd ?? input.value).trim()
  if (!command) return
  if (!needAdb()) return
  if (running.value) {
    toast('warn', '上一条命令仍在执行')
    return
  }

  lines.value.push({ type: 'cmd', text: `$ ${command}` })
  history.value.push(command)
  historyIndex.value = -1
  input.value = ''
  running.value = true
  await scrollBottom()

  try {
    const out = await api.adbShell(state.serial, command)
    if (out && out.trim()) {
      for (const l of out.replace(/\r\n/g, '\n').split('\n')) {
        lines.value.push({ type: 'out', text: l })
      }
    } else {
      lines.value.push({ type: 'info', text: '（无输出）' })
    }
  } catch (e: any) {
    lines.value.push({ type: 'err', text: e?.message ?? String(e) })
  } finally {
    running.value = false
    if (lines.value.length > 2000) lines.value.splice(0, lines.value.length - 2000)
    await scrollBottom()
    inputRef.value?.focus()
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter') {
    e.preventDefault()
    void execute()
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    if (history.value.length === 0) return
    if (historyIndex.value === -1) historyIndex.value = history.value.length - 1
    else if (historyIndex.value > 0) historyIndex.value--
    input.value = history.value[historyIndex.value] ?? ''
  } else if (e.key === 'ArrowDown') {
    e.preventDefault()
    if (historyIndex.value === -1) return
    if (historyIndex.value < history.value.length - 1) {
      historyIndex.value++
      input.value = history.value[historyIndex.value] ?? ''
    } else {
      historyIndex.value = -1
      input.value = ''
    }
  }
}

function clear() {
  lines.value = [{ type: 'info', text: '已清空' }]
}

function copyAll() {
  const text = lines.value.map((l) => (l.type === 'cmd' ? `$ ${l.text}` : l.text)).join('\n')
  api.copyText(text)
  toast('ok', '输出已复制到剪贴板')
}
</script>

<template>
  <div class="flex h-full flex-col p-5">
    <div class="mb-3 flex items-end justify-between">
      <div>
        <h1 class="text-[15px] font-medium text-slate-100">ADB 终端</h1>
        <p class="mt-1 text-[11.5px] text-muted">
          直接执行 shell 命令，等同于 <span class="mono text-slate-400">adb -s {{ state.serial || '…' }} shell</span>
        </p>
      </div>
      <div class="flex items-center gap-2">
        <button class="btn btn-sm" @click="copyAll">
          <Copy class="h-3.5 w-3.5" /> 复制
        </button>
        <button class="btn btn-sm" @click="clear">
          <Trash2 class="h-3.5 w-3.5" /> 清空
        </button>
      </div>
    </div>

    <!-- 快捷命令 -->
    <div class="mb-2 flex flex-wrap gap-1.5">
      <button
        v-for="q in quick"
        :key="q.cmd"
        class="btn btn-sm"
        @click="execute(q.cmd)"
      >
        {{ q.label }}
      </button>
    </div>

    <!-- 输出 -->
    <div
      ref="bodyRef"
      class="panel min-h-0 flex-1 overflow-y-auto bg-ink-950 px-3 py-2 font-mono text-[11.5px]"
      @click="inputRef?.focus()"
    >
      <div v-for="(l, i) in lines" :key="i" class="log-line" :class="{
        'text-accent': l.type === 'cmd',
        'text-slate-300': l.type === 'out',
        'text-danger': l.type === 'err',
        'text-slate-500': l.type === 'info',
      }">{{ l.text || ' ' }}</div>
      <div v-if="running" class="mt-1 flex items-center gap-1 text-slate-500">
        <span class="h-2 w-2 animate-pulse rounded-full bg-accent" />
        执行中…
      </div>
    </div>

    <!-- 输入 -->
    <div class="mt-2 flex items-center gap-2">
      <div class="relative flex-1">
        <span class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 font-mono text-[12px] text-accent">$</span>
        <input
          ref="inputRef"
          v-model="input"
          class="input mono pl-6"
          placeholder="输入 shell 命令，回车执行（↑↓ 翻阅历史）"
          :disabled="running"
          @keydown="onKeydown"
        />
      </div>
      <button class="btn btn-primary" :disabled="running || !input.trim()" @click="execute()">
        <Play class="h-3.5 w-3.5" /> 执行
      </button>
    </div>
  </div>
</template>
