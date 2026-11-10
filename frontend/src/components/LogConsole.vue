<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { ChevronDown, ChevronUp, Trash2, Terminal, Download } from 'lucide-vue-next'
import { state, clearLogs, toast } from '../store'
import { api } from '../api'

const bodyRef = ref<HTMLElement | null>(null)

watch(
  () => state.logs.length,
  async () => {
    if (!state.logOpen) return
    await nextTick()
    const el = bodyRef.value
    if (el) el.scrollTop = el.scrollHeight
  }
)

function levelClass(level: string): string {
  switch (level) {
    case 'ok': return 'text-ok'
    case 'warn': return 'text-warn'
    case 'err': return 'text-danger'
    case 'cmd': return 'text-accent'
    case 'info': return 'text-slate-300'
    default: return 'text-slate-400'
  }
}

async function exportLogs() {
  if (state.logs.length === 0) {
    toast('warn', '暂无日志可导出')
    return
  }
  const text = state.logs.map((l) => `[${l.time}] [${l.level}] ${l.text}`).join('\n')
  const name = `droidforge-log-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}.txt`
  const path = await api.saveTextFile(name, text)
  if (path) toast('ok', '日志已导出')
}
</script>

<template>
  <section class="shrink-0 border-t border-ink-700 bg-ink-900">
    <div class="flex h-8 items-center gap-2 px-3">
      <Terminal class="h-3.5 w-3.5 text-muted" />
      <span class="text-[11px] text-slate-400">操作日志</span>
      <span class="tag">{{ state.logs.length }}</span>
      <div class="flex-1" />
      <button class="btn btn-sm btn-icon" title="导出日志" @click="exportLogs">
        <Download class="h-3.5 w-3.5" />
      </button>
      <button class="btn btn-sm btn-icon" title="清空" @click="clearLogs">
        <Trash2 class="h-3.5 w-3.5" />
      </button>
      <button class="btn btn-sm btn-icon" :title="state.logOpen ? '收起' : '展开'" @click="state.logOpen = !state.logOpen">
        <ChevronUp v-if="state.logOpen" class="h-3.5 w-3.5" />
        <ChevronDown v-else class="h-3.5 w-3.5" />
      </button>
    </div>
    <div
      v-show="state.logOpen"
      ref="bodyRef"
      class="h-[168px] overflow-y-auto border-t border-ink-800 bg-ink-950 px-3 py-2 font-mono text-[11px]"
    >
      <div v-if="state.logs.length === 0" class="py-6 text-center text-[11px] text-slate-600">
        暂无日志。连接设备后，操作过程会实时显示在这里。
      </div>
      <div v-for="(l, i) in state.logs" :key="i" class="log-line flex gap-2">
        <span class="shrink-0 text-slate-600">{{ l.time }}</span>
        <span class="shrink-0" :class="levelClass(l.level)">›</span>
        <span :class="levelClass(l.level)">{{ l.text }}</span>
      </div>
    </div>
  </section>
</template>
