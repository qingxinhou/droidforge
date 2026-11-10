<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  AlertTriangle,
  CheckSquare,
  FolderOpen,
  Loader2,
  Package,
  Plus,
  RefreshCw,
  ShieldCheck,
  Square,
  Trash2,
  Zap,
} from 'lucide-vue-next'
import { api } from '../api'
import { currentDevice, needAdb, run, toast, fmtBytes } from '../store'
import type { MagiskInstallResult, MagiskManager, MagiskModule } from '../types'

const manager = ref<MagiskManager | null>(null)
const modules = ref<MagiskModule[]>([])
const selected = ref<string[]>([])
const detecting = ref(false)
const installing = ref(false)
const results = ref<MagiskInstallResult[]>([])

const serial = computed(() => currentDevice.value?.serial ?? '')
const validMods = computed(() => modules.value.filter((m) => m.valid))
const selectedMods = computed(() => modules.value.filter((m) => selected.value.includes(m.path) && m.valid))

async function detect() {
  if (!needAdb()) return
  detecting.value = true
  try {
    manager.value = await api.detectRootManager(serial.value)
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
  } finally {
    detecting.value = false
  }
}

async function addOne() {
  try {
    const m = await api.pickModule()
    if (!m) return
    if (!m.valid) {
      toast('warn', `${m.fileName}：${m.error}`, 6000)
      return
    }
    if (modules.value.some((x) => x.path === m.path)) {
      toast('info', '这个包已经在列表里了')
      return
    }
    modules.value.push(m)
    selected.value.push(m.path)
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
  }
}

async function scanDir() {
  const dir = await api.pickDirectory('选择模块所在目录')
  if (!dir) return
  try {
    const found = await run('扫描模块', () => api.scanModuleDir(dir))
    if (!found) return
    let added = 0
    for (const m of found) {
      if (modules.value.some((x) => x.path === m.path)) continue
      modules.value.push(m)
      if (m.valid) {
        selected.value.push(m.path)
        added++
      }
    }
    toast('ok', `新增 ${added} 个可用模块`)
  } catch (e: any) {
    toast('err', e?.message ?? String(e), 6000)
  }
}

function toggle(p: string) {
  const i = selected.value.indexOf(p)
  if (i >= 0) selected.value.splice(i, 1)
  else selected.value.push(p)
}

function toggleAll() {
  const all = validMods.value.map((m) => m.path)
  selected.value = selected.value.length === all.length ? [] : all
}

function remove(p: string) {
  modules.value = modules.value.filter((m) => m.path !== p)
  selected.value = selected.value.filter((x) => x !== p)
}

async function install() {
  if (selectedMods.value.length === 0) return
  installing.value = true
  results.value = []
  try {
    const r = await run('安装模块', () => api.installModules(serial.value, selectedMods.value))
    if (r) {
      results.value = r
      const ok = r.filter((x) => x.ok).length
      toast(ok === r.length ? 'ok' : 'warn', `${ok}/${r.length} 个模块安装成功`)
    }
  } catch (e: any) {
    toast('err', e?.message ?? String(e), 8000)
  } finally {
    installing.value = false
  }
}

const kindLabel = computed(() => {
  switch (manager.value?.kind) {
    case 'magisk':
      return 'Magisk'
    case 'kernelsu':
      return 'KernelSU'
    case 'apatch':
      return 'APatch'
    default:
      return '未识别'
  }
})

onMounted(() => {
  if (currentDevice.value) detect()
})
</script>

<template>
  <div>
    <div class="mb-4 flex items-start justify-between gap-4">
      <div>
        <h1 class="text-[17px] font-medium text-slate-100">模块管理</h1>
        <p class="mt-1 text-[11.5px] text-muted">
          批量安装 Magisk / KernelSU / APatch 模块，装完重启生效
        </p>
      </div>
      <button class="btn btn-sm" :disabled="detecting" @click="detect">
        <RefreshCw class="h-3.5 w-3.5" :class="detecting ? 'animate-spin' : ''" />检测环境
      </button>
    </div>

    <div class="panel">
      <div class="panel-head">
        <span class="panel-title">Root 环境</span>
        <span v-if="manager" class="tag" :class="manager.rootOk && manager.kind !== 'unknown' ? 'tag-ok' : 'tag-warn'">
          {{ manager.rootOk ? 'su 可用' : '无 root' }}
        </span>
      </div>
      <div class="grid grid-cols-3 gap-x-6 px-4 py-3">
        <div class="kv"><span class="kv-k">模块管理器</span><span class="kv-v mono">{{ kindLabel }}</span></div>
        <div class="kv"><span class="kv-k">版本</span><span class="kv-v mono">{{ manager?.version || '—' }}</span></div>
        <div class="kv"><span class="kv-k">设备</span><span class="kv-v mono truncate">{{ serial || '未连接' }}</span></div>
      </div>
      <div v-if="manager?.note" class="border-t border-ink-700 px-4 py-2 text-[11px] text-muted">
        {{ manager.note }}
      </div>
    </div>

    <div class="panel mt-3">
      <div class="panel-head">
        <div class="flex items-center gap-2">
          <span class="panel-title">模块列表</span>
          <button class="btn btn-sm" @click="addOne"><Plus class="h-3.5 w-3.5" />添加</button>
          <button class="btn btn-sm" @click="scanDir"><FolderOpen class="h-3.5 w-3.5" />扫目录</button>
        </div>
        <button class="btn btn-sm" :disabled="validMods.length === 0" @click="toggleAll">
          <component
            :is="selected.length === validMods.length && validMods.length > 0 ? CheckSquare : Square"
            class="h-3.5 w-3.5"
          />
          {{ selected.length === validMods.length && validMods.length > 0 ? '取消全选' : '全选' }}
        </button>
      </div>

      <div v-if="modules.length === 0" class="px-4 py-8 text-center text-[12px] text-muted">
        <Package class="mx-auto mb-2 h-6 w-6 opacity-50" />
        添加模块 zip，或扫描一个装了很多模块的目录
      </div>

      <div v-else class="divide-y divide-ink-700/50">
        <div
          v-for="m in modules"
          :key="m.path"
          class="flex items-center gap-3 px-4 py-2.5"
          :class="m.valid ? 'hover:bg-ink-800/50' : 'opacity-60'"
        >
          <button
            v-if="m.valid"
            class="shrink-0"
            @click="toggle(m.path)"
          >
            <component
              :is="selected.includes(m.path) ? CheckSquare : Square"
              class="h-3.5 w-3.5"
              :class="selected.includes(m.path) ? 'text-accent' : 'text-slate-600'"
            />
          </button>
          <ShieldCheck v-else class="h-3.5 w-3.5 shrink-0 text-slate-600" />

          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              <span class="truncate text-[12px] text-slate-200">{{ m.name || m.fileName }}</span>
              <span v-if="m.version" class="tag">{{ m.version }}</span>
              <span v-if="!m.valid" class="tag tag-warn">不可用</span>
            </div>
            <div class="mt-0.5 truncate text-[11px] text-muted">
              {{ m.valid ? (m.description || m.fileName) : m.error }}
            </div>
          </div>

          <div class="shrink-0 text-right">
            <div class="mono text-[11px] text-muted">{{ fmtBytes(m.size) }}</div>
            <div v-if="m.author" class="text-[10.5px] text-muted">{{ m.author }}</div>
          </div>
          <button class="btn btn-sm shrink-0" title="从列表移除" @click="remove(m.path)">
            <Trash2 class="h-3.5 w-3.5" />
          </button>
        </div>
      </div>

      <div class="flex items-center justify-between border-t border-ink-700 px-4 py-3">
        <div class="flex items-start gap-1.5 text-[11px] text-warn">
          <AlertTriangle class="mt-0.5 h-3 w-3 shrink-0" />
          <span>模块来自你本机，本工具不内置也不下载任何模块。装错模块可能导致无法开机，请自行准备救砖方案。</span>
        </div>
        <button class="btn btn-primary" :disabled="installing || selectedMods.length === 0" @click="install">
          <Loader2 v-if="installing" class="h-3.5 w-3.5 animate-spin" />
          <Zap v-else class="h-3.5 w-3.5" />
          安装 {{ selectedMods.length }} 个
        </button>
      </div>
    </div>

    <div v-if="results.length" class="panel mt-3">
      <div class="panel-head">
        <span class="panel-title">安装结果</span>
        <span class="text-[11px] text-muted">重启后生效</span>
      </div>
      <div class="divide-y divide-ink-700/50">
        <div v-for="r in results" :key="r.module.path" class="px-4 py-2">
          <div class="flex items-center gap-2">
            <span class="tag" :class="r.ok ? 'tag-ok' : 'tag-danger'">{{ r.ok ? '成功' : '失败' }}</span>
            <span class="text-[12px] text-slate-200">{{ r.module.name }}</span>
          </div>
          <div v-if="r.error" class="mt-1 text-[11px] text-danger">{{ r.error }}</div>
          <pre v-else-if="r.output" class="mt-1 max-h-24 overflow-y-auto whitespace-pre-wrap text-[10.5px] text-muted">{{ r.output }}</pre>
        </div>
      </div>
    </div>
  </div>
</template>
