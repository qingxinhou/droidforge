<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  Search, RefreshCw, Trash2, Upload, Play, Power, Square,
  Eraser, Info, FolderDown, Undo2, History, Layers,
} from 'lucide-vue-next'
import { api } from '../api'
import { state, toast, needAdb, run } from '../store'
import { confirmDialog } from '../dialog'
import type { AppDetail, AppInfo } from '../types'

const list = ref<AppInfo[]>([])
const keyword = ref('')
const scope = ref<'third' | 'system' | 'all' | 'disabled' | 'google'>('third')
const loading = ref(false)
const detail = ref<AppDetail | null>(null)
const detailPkg = ref('')
const ops = ref<{ time: string; text: string }[]>([])
const selected = ref<string[]>([])

const LOG_KEY = 'droidforge.applog'

const scopes = [
  { key: 'third', label: '第三方' },
  { key: 'system', label: '系统' },
  { key: 'all', label: '全部' },
  { key: 'disabled', label: '已禁用' },
  { key: 'google', label: '谷歌套件' },
] as const

function isGoogle(pkg: string): boolean {
  return /^(com\.google\.|com\.android\.vending|com\.android\.gsf)/.test(pkg)
}

const filtered = computed(() => {
  let src = list.value
  if (scope.value === 'third') src = src.filter((a) => !a.system)
  else if (scope.value === 'system') src = src.filter((a) => a.system)
  else if (scope.value === 'disabled') src = src.filter((a) => !a.enabled)
  else if (scope.value === 'google') src = src.filter((a) => isGoogle(a.package))
  const k = keyword.value.trim().toLowerCase()
  if (!k) return src
  return src.filter((a) => a.package.toLowerCase().includes(k))
})

const googleCount = computed(() => list.value.filter((a) => isGoogle(a.package)).length)

function loadLog() {
  try {
    const raw = localStorage.getItem(LOG_KEY)
    ops.value = raw ? JSON.parse(raw) : []
  } catch {
    ops.value = []
  }
}

function pushLog(text: string) {
  ops.value.unshift({ time: new Date().toLocaleTimeString('zh-CN', { hour12: false }), text })
  if (ops.value.length > 300) ops.value.length = 300
  localStorage.setItem(LOG_KEY, JSON.stringify(ops.value))
}

function clearLog() {
  ops.value = []
  localStorage.removeItem(LOG_KEY)
}

async function load() {
  if (!needAdb()) return
  loading.value = true
  try {
    list.value = await api.listAppsDetail(state.serial, true)
    selected.value = []
    if (detailPkg.value) {
      const hit = list.value.find((a) => a.package === detailPkg.value)
      if (!hit) {
        detail.value = null
        detailPkg.value = ''
      }
    }
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
  } finally {
    loading.value = false
  }
}

async function install() {
  if (!needAdb()) return
  const files = await api.pickFiles('选择 APK（可多选拆分包）', '*.apk')
  if (!files || files.length === 0) return
  const reinstall = state.env?.config.installReinstall ?? true
  await run('安装完成', () => api.installApks(state.serial, files, reinstall, files.length > 1))
  await load()
}

async function act(a: AppInfo, action: string, label: string) {
  if (!needAdb()) return
  if (action === 'clear') {
    const ok = await confirmDialog({
      title: '清除应用数据',
      message: `确认清除 ${a.package} 的全部数据？\n\n应用会回到刚安装的状态，登录信息与本地文件都会丢失。`,
      danger: true,
      confirmText: '清除',
    })
    if (!ok) return
  }
  const done = await run('', () => api.appAction(state.serial, a.package, action))
  if (done !== null) {
    pushLog(`${label} ${a.package}`)
    toast('ok', `${label}：${a.package}`)
    await load()
    if (detailPkg.value === a.package) await showDetail(a.package)
  }
}

async function uninstall(a: AppInfo, keepData: boolean) {
  if (!needAdb()) return
  const ok = await confirmDialog({
    title: keepData ? '卸载但保留数据' : '卸载应用',
    message: `确认卸载 ${a.package}？\n\n${
      a.system ? '这是系统应用，误删可能导致系统异常；之后可用「恢复」重新安装。' : '应用数据会一并删除。'
    }`,
    danger: a.system,
    confirmText: '卸载',
  })
  if (!ok) return
  const done = await run('', () => api.uninstallApp(state.serial, a.package, keepData))
  if (done !== null) {
    pushLog(`${keepData ? '保留数据卸载' : '卸载'} ${a.package}`)
    toast('ok', `已卸载 ${a.package}`)
    await load()
  }
}

async function showDetail(pkg: string) {
  if (detailPkg.value === pkg && detail.value) {
    detailPkg.value = ''
    detail.value = null
    return
  }
  detailPkg.value = pkg
  try {
    detail.value = await api.getAppDetail(state.serial, pkg)
  } catch (e: any) {
    detail.value = null
    toast('err', e?.message ?? String(e))
  }
}

async function extractOne(a: AppInfo) {
  const files = await run('', () => api.extractApps(state.serial, [a.package]))
  if (files && files.length) {
    pushLog(`提取 ${a.package}`)
    toast('ok', `已提取到 ${files[0]}`)
    await api.revealInFolder(files[0])
  }
}

async function batchGoogle(action: 'disable' | 'enable') {
  if (!needAdb()) return
  const targets = list.value.filter((a) => isGoogle(a.package) && (action === 'disable' ? a.enabled : !a.enabled))
  if (targets.length === 0) {
    toast('warn', action === 'disable' ? '没有可禁用的谷歌组件' : '没有可启用的谷歌组件')
    return
  }
  const ok = await confirmDialog({
    title: action === 'disable' ? '批量禁用谷歌套件' : '批量启用谷歌套件',
    message: `将${action === 'disable' ? '禁用' : '启用'} ${targets.length} 个组件。\n\n禁用后相关应用（Play 商店、地图等）可能无法使用，可随时用同一按钮恢复。`,
    danger: action === 'disable',
    confirmText: action === 'disable' ? '禁用' : '启用',
  })
  if (!ok) return
  let failed = 0
  for (const a of targets) {
    try {
      await api.appAction(state.serial, a.package, action)
    } catch {
      failed++
    }
  }
  pushLog(`批量${action === 'disable' ? '禁用' : '启用'}谷歌组件 ${targets.length - failed} 个`)
  toast(failed ? 'warn' : 'ok', `完成 ${targets.length - failed} 个，失败 ${failed} 个`)
  await load()
}

function toggleSelect(pkg: string) {
  const i = selected.value.indexOf(pkg)
  if (i >= 0) selected.value.splice(i, 1)
  else selected.value.push(pkg)
}

async function batchSelected(action: 'disable' | 'enable' | 'extract') {
  if (!needAdb() || selected.value.length === 0) return
  if (action === 'extract') {
    const files = await run('', () => api.extractApps(state.serial, selected.value))
    if (files && files.length) {
      toast('ok', `已提取 ${selected.value.length} 个应用`)
      await api.revealInFolder(files[0])
    }
    return
  }
  let failed = 0
  for (const pkg of selected.value) {
    try {
      await api.appAction(state.serial, pkg, action)
    } catch {
      failed++
    }
  }
  toast(failed ? 'warn' : 'ok', `完成 ${selected.value.length - failed} 个，失败 ${failed} 个`)
  await load()
}

onMounted(() => {
  loadLog()
  if (state.serial && state.mode === 'adb') void load()
})
</script>

<template>
  <div class="p-5">
    <div class="mb-4 flex items-end justify-between">
      <div>
        <h1 class="text-[15px] font-medium text-slate-100">应用管理</h1>
        <p class="mt-1 text-[11.5px] text-muted">
          启用 / 禁用、强行停止、清除数据、卸载（可保留数据）、恢复系统应用与提取安装包。
        </p>
      </div>
      <div class="flex items-center gap-2">
        <div class="relative">
          <Search class="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-500" />
          <input v-model="keyword" class="input w-[200px] pl-7" placeholder="搜索包名" />
        </div>
        <button class="btn" :disabled="loading" @click="load">
          <RefreshCw class="h-3.5 w-3.5" :class="loading ? 'animate-spin' : ''" /> 刷新
        </button>
        <button class="btn btn-primary" @click="install"><Upload class="h-3.5 w-3.5" /> 安装 APK</button>
      </div>
    </div>

    <div v-if="state.mode !== 'adb'" class="panel py-16 text-center text-[12px] text-muted">
      应用管理需要设备处于 ADB 模式
    </div>

    <template v-else>
      <!-- 筛选与批量 -->
      <div class="mb-3 flex items-center gap-2">
        <div class="flex overflow-hidden rounded border border-ink-600">
          <button
            v-for="sc in scopes"
            :key="sc.key"
            class="px-3 py-1.5 text-[11px] transition-colors"
            :class="scope === sc.key ? 'bg-accent/15 text-accent' : 'text-slate-400 hover:bg-ink-800'"
            @click="scope = sc.key"
          >
            {{ sc.label }}
            <span v-if="sc.key === 'google' && googleCount" class="ml-1 text-[9px] text-muted">{{ googleCount }}</span>
          </button>
        </div>
        <div class="ml-auto flex items-center gap-2">
          <span v-if="selected.length" class="text-[10px] text-muted">已选 {{ selected.length }}</span>
          <button v-if="selected.length" class="btn btn-sm" @click="batchSelected('enable')">启用</button>
          <button v-if="selected.length" class="btn btn-sm" @click="batchSelected('disable')">禁用</button>
          <button v-if="selected.length" class="btn btn-sm" @click="batchSelected('extract')">
            <FolderDown class="h-3 w-3" /> 提取
          </button>
          <button class="btn btn-sm" @click="batchGoogle('enable')">谷歌套件全部启用</button>
          <button class="btn btn-sm btn-danger" @click="batchGoogle('disable')">谷歌套件全部禁用</button>
        </div>
      </div>

      <!-- 详情 -->
      <div v-if="detail" class="panel mb-3 p-4">
        <div class="mb-2 flex items-center justify-between">
          <div class="mono text-[12px] text-slate-100">{{ detail.package }}</div>
          <div class="flex gap-1.5">
            <span v-if="detail.system" class="tag tag-warn">系统</span>
            <span class="tag" :class="detail.enabled ? 'tag-ok' : 'tag-danger'">{{ detail.enabled ? '已启用' : '已禁用' }}</span>
            <button class="btn btn-sm" @click="detail = null">关闭</button>
          </div>
        </div>
        <div class="grid grid-cols-3 gap-x-6">
          <div class="kv"><span class="kv-k">版本</span><span class="kv-v mono">{{ detail.versionName || '—' }} ({{ detail.versionCode }})</span></div>
          <div class="kv"><span class="kv-k">SDK</span><span class="kv-v mono">min {{ detail.minSdk || '—' }} / target {{ detail.targetSdk || '—' }}</span></div>
          <div class="kv"><span class="kv-k">UID</span><span class="kv-v mono">{{ detail.uid || '—' }}</span></div>
          <div class="kv"><span class="kv-k">首次安装</span><span class="kv-v">{{ detail.firstInstall || '—' }}</span></div>
          <div class="kv"><span class="kv-k">最近更新</span><span class="kv-v">{{ detail.lastUpdate || '—' }}</span></div>
          <div class="kv"><span class="kv-k">APK 数量</span><span class="kv-v mono">{{ detail.splitPaths?.length ?? 0 }}</span></div>
          <div class="kv col-span-2"><span class="kv-k">数据目录</span><span class="kv-v mono truncate">{{ detail.dataDir || '—' }}</span></div>
          <div class="kv"><span class="kv-k">标记</span><span class="kv-v mono truncate">{{ (detail.flags ?? []).join(' ') || '—' }}</span></div>
          <div class="kv col-span-3"><span class="kv-k">APK 路径</span><span class="kv-v mono truncate">{{ (detail.splitPaths ?? []).join('  |  ') || detail.codePath || '—' }}</span></div>
        </div>
      </div>

      <!-- 列表 -->
      <div class="panel overflow-hidden">
        <div class="flex items-center justify-between border-b border-ink-700 px-4 py-2 text-[11px] text-slate-300">
          <span>共 {{ filtered.length }} 个应用</span>
          <span class="text-[10px] text-muted">点包名查看详情 · 勾选可批量操作</span>
        </div>
        <div class="max-h-[520px] overflow-y-auto">
          <table class="w-full text-[11px]">
            <thead class="sticky top-0 z-10 bg-ink-850 text-muted">
              <tr class="border-b border-ink-700">
                <th class="w-8 px-3 py-2"></th>
                <th class="px-2 py-2 text-left font-normal">包名</th>
                <th class="px-3 py-2 text-left font-normal">状态</th>
                <th class="px-3 py-2 text-left font-normal">类型</th>
                <th class="px-3 py-2 text-right font-normal">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="a in filtered" :key="a.package" class="border-b border-ink-800 hover:bg-ink-800/50">
                <td class="px-3 py-1.5">
                  <input
                    type="checkbox"
                    class="accent-accent"
                    :checked="selected.includes(a.package)"
                    @change="toggleSelect(a.package)"
                  />
                </td>
                <td class="px-2 py-1.5">
                  <button class="mono text-left text-slate-200 hover:text-accent" @click="showDetail(a.package)">
                    {{ a.package }}
                  </button>
                </td>
                <td class="px-3 py-1.5">
                  <span class="tag" :class="a.enabled ? 'tag-ok' : 'tag-danger'">{{ a.enabled ? '启用' : '禁用' }}</span>
                </td>
                <td class="px-3 py-1.5">
                  <span class="tag" :class="a.system ? 'tag-warn' : ''">{{ a.system ? '系统' : '第三方' }}</span>
                </td>
                <td class="px-3 py-1.5">
                  <div class="flex items-center justify-end gap-1">
                    <button class="btn btn-sm btn-icon" title="启动" @click="act(a, 'launch', '启动')"><Play class="h-3 w-3" /></button>
                    <button class="btn btn-sm btn-icon" title="强行停止" @click="act(a, 'force-stop', '停止')"><Square class="h-3 w-3" /></button>
                    <button class="btn btn-sm btn-icon" title="清除数据" @click="act(a, 'clear', '清除数据')"><Eraser class="h-3 w-3" /></button>
                    <button
                      class="btn btn-sm btn-icon"
                      :title="a.enabled ? '禁用' : '启用'"
                      @click="act(a, a.enabled ? 'disable' : 'enable', a.enabled ? '禁用' : '启用')"
                    >
                      <Power class="h-3 w-3" />
                    </button>
                    <button class="btn btn-sm btn-icon" title="提取安装包" @click="extractOne(a)"><FolderDown class="h-3 w-3" /></button>
                    <button class="btn btn-sm btn-icon" title="详情" @click="showDetail(a.package)"><Info class="h-3 w-3" /></button>
                    <button
                      v-if="a.system"
                      class="btn btn-sm btn-icon"
                      title="恢复系统应用"
                      @click="act(a, 'restore', '恢复')"
                    >
                      <Undo2 class="h-3 w-3" />
                    </button>
                    <button class="btn btn-sm btn-icon" title="保留数据卸载" @click="uninstall(a, true)"><Layers class="h-3 w-3" /></button>
                    <button class="btn btn-sm btn-icon btn-danger" title="卸载" @click="uninstall(a, false)">
                      <Trash2 class="h-3 w-3" />
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- 操作记录 -->
      <div class="panel mt-3">
        <div class="panel-header">
          <span class="panel-title"><History class="h-3.5 w-3.5" /> 操作记录</span>
          <button class="btn btn-sm" @click="clearLog">清空</button>
        </div>
        <div v-if="ops.length === 0" class="px-4 py-6 text-center text-[10.5px] text-muted">
          还没有操作记录（保存在本机浏览器存储，不上传）
        </div>
        <div v-else class="max-h-[140px] overflow-y-auto px-4 py-2">
          <div v-for="(o, i) in ops" :key="i" class="flex gap-3 border-b border-ink-700/50 py-1 text-[10.5px] last:border-0">
            <span class="mono shrink-0 text-muted">{{ o.time }}</span>
            <span class="text-slate-300">{{ o.text }}</span>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
