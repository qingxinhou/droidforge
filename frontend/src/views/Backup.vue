<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Archive, CheckSquare, FolderOpen, Loader2, Play, RefreshCw, Square } from 'lucide-vue-next'
import { api } from '../api'
import { currentDevice, needAdb, run, toast, fmtBytes } from '../store'
import type { BackupPlan, BackupResult } from '../types'

const plan = ref<BackupPlan | null>(null)
const selected = ref<string[]>([])
const outDir = ref('')
const skipExisting = ref(true)
const scanning = ref(false)
const backing = ref(false)
const result = ref<BackupResult | null>(null)

const serial = computed(() => currentDevice.value?.serial ?? '')
const cats = computed(() => plan.value?.categories ?? [])
const usable = computed(() => cats.value.filter((c) => c.exists))
const selectedCats = computed(() => cats.value.filter((c) => selected.value.includes(c.id)))
const selectedBytes = computed(() => selectedCats.value.reduce((s, c) => s + c.size, 0))

async function scan() {
  if (!needAdb()) return
  scanning.value = true
  result.value = null
  try {
    const p = await api.scanBackup(serial.value)
    plan.value = p
    selected.value = (p.categories ?? []).filter((c) => c.exists).map((c) => c.id)
    if (!p.categories.some((c) => c.exists)) {
      toast('warn', '设备上没有可读的数据目录，可能被系统限制', 6000)
    }
  } catch (e: any) {
    toast('err', e?.message ?? String(e), 6000)
  } finally {
    scanning.value = false
  }
}

function toggle(id: string) {
  const i = selected.value.indexOf(id)
  if (i >= 0) selected.value.splice(i, 1)
  else selected.value.push(id)
}

function toggleAll() {
  selected.value =
    selected.value.length === usable.value.length ? [] : usable.value.map((c) => c.id)
}

async function pickOut() {
  const p = await api.pickDirectory('选择备份保存目录')
  if (p) outDir.value = p
}

async function start() {
  if (!needAdb() || selected.value.length === 0) return
  backing.value = true
  result.value = null
  try {
    const r = await run('备份', () =>
      api.runBackup(serial.value, {
        outDir: outDir.value,
        categoryIds: selected.value,
        skipExisting: skipExisting.value,
      })
    )
    if (r) {
      result.value = r
      toast('ok', `已备份 ${r.files} 个文件，共 ${fmtBytes(r.bytes)}`)
    }
  } catch (e: any) {
    toast('err', e?.message ?? String(e), 8000)
  } finally {
    backing.value = false
  }
}

function openOut(p: string) {
  api.openPath(p).catch(() => toast('err', '打不开这个目录'))
}

onMounted(() => {
  if (currentDevice.value) scan()
})
</script>

<template>
  <div>
    <div class="mb-4 flex items-start justify-between gap-4">
      <div>
        <h1 class="text-[17px] font-medium text-slate-100">备份助手</h1>
        <p class="mt-1 text-[11.5px] text-muted">
          按类别把照片、文档、音乐、通讯录、短信拉到本机，已存在且大小一致的文件自动跳过
        </p>
      </div>
      <button class="btn btn-sm" :disabled="scanning" @click="scan">
        <RefreshCw class="h-3.5 w-3.5" :class="scanning ? 'animate-spin' : ''" />重新扫描
      </button>
    </div>

    <div v-if="!currentDevice" class="panel px-4 py-6 text-[12px] text-muted">
      先连接一台已开启 USB 调试的设备。
    </div>

    <template v-else>
      <div class="panel">
        <div class="panel-head">
          <div class="flex items-center gap-2">
            <span class="panel-title">可备份内容</span>
            <button class="btn btn-sm" :disabled="!plan" @click="toggleAll">
              <component :is="selected.length === usable.length && usable.length > 0 ? CheckSquare : Square" class="h-3.5 w-3.5" />
              {{ selected.length === usable.length && usable.length > 0 ? '取消全选' : '全选' }}
            </button>
          </div>
          <span class="text-[11px] text-muted">
            {{ selected.length }} 项 · 预估 {{ fmtBytes(selectedBytes) }}
          </span>
        </div>

        <div v-if="scanning" class="flex items-center gap-2 px-4 py-6 text-[12px] text-muted">
          <Loader2 class="h-4 w-4 animate-spin" />正在统计设备上的数据…
        </div>

        <div v-else class="divide-y divide-ink-700/50">
          <div
            v-for="c in cats"
            :key="c.id"
            class="flex items-center gap-3 px-4 py-2.5"
            :class="c.exists ? 'cursor-pointer hover:bg-ink-800/50' : 'opacity-55'"
            @click="c.exists && toggle(c.id)"
          >
            <component
              :is="selected.includes(c.id) ? CheckSquare : Square"
              class="h-3.5 w-3.5 shrink-0"
              :class="selected.includes(c.id) ? 'text-accent' : 'text-slate-600'"
            />
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span class="text-[12px] text-slate-200">{{ c.label }}</span>
                <span v-if="!c.exists" class="tag tag-warn">不可用</span>
              </div>
              <div class="mt-0.5 truncate text-[11px] text-muted">
                {{ c.exists ? c.desc : c.note }}
              </div>
            </div>
            <div class="shrink-0 text-right">
              <div class="mono text-[11.5px] text-slate-300">{{ c.size > 0 ? fmtBytes(c.size) : '—' }}</div>
              <div class="mono text-[10.5px] text-muted">{{ c.files > 0 ? c.files + ' 项' : '' }}</div>
            </div>
          </div>
        </div>
      </div>

      <div class="panel mt-3">
        <div class="panel-head">
          <span class="panel-title">保存位置</span>
          <button class="btn btn-sm" @click="pickOut"><FolderOpen class="h-3.5 w-3.5" />选择</button>
        </div>
        <div class="px-4 py-3">
          <input class="input mono w-full" :value="outDir" readonly placeholder="留空则保存到输出目录下的 backup_时间戳" />
          <label class="mt-2.5 flex items-center gap-2 text-[11.5px] text-muted">
            <input v-model="skipExisting" type="checkbox" class="h-3.5 w-3.5 accent-accent" />
            跳过本地已存在且大小一致的文件（适合反复增量备份）
          </label>
        </div>
        <div class="flex items-center justify-between border-t border-ink-700 px-4 py-3">
          <span class="text-[11px] text-muted">
            备份过程走 tar 流，一个连接传完整棵目录；中途断开可直接重来
          </span>
          <button class="btn btn-primary" :disabled="backing || selected.length === 0" @click="start">
            <Loader2 v-if="backing" class="h-3.5 w-3.5 animate-spin" />
            <Play v-else class="h-3.5 w-3.5" />
            开始备份
          </button>
        </div>
      </div>

      <div v-if="result" class="panel mt-3">
        <div class="panel-head">
          <span class="panel-title">备份完成</span>
          <button class="btn btn-sm" @click="openOut(result.outDir)">打开目录</button>
        </div>
        <div class="grid grid-cols-4 gap-x-6 px-4 py-3">
          <div class="kv"><span class="kv-k">文件</span><span class="kv-v mono">{{ result.files }}</span></div>
          <div class="kv"><span class="kv-k">大小</span><span class="kv-v mono">{{ fmtBytes(result.bytes) }}</span></div>
          <div class="kv"><span class="kv-k">跳过</span><span class="kv-v mono">{{ result.skipped }}</span></div>
          <div class="kv"><span class="kv-k">清单</span><span class="kv-v mono truncate">{{ result.manifest ? 'manifest.json' : '—' }}</span></div>
        </div>
        <div v-if="result.categories.some((c) => c.error)" class="border-t border-ink-700 px-4 py-2">
          <div
            v-for="c in result.categories.filter((x) => x.error)"
            :key="c.id"
            class="py-0.5 text-[11px] text-warn"
          >
            {{ c.label }}：{{ c.error }}
          </div>
        </div>
        <div class="border-t border-ink-700 px-4 py-2 text-[11px] text-muted">
          <Archive class="mr-1 inline h-3 w-3" />通讯录与短信导出为 CSV（带 BOM，Excel 可直接打开）
        </div>
      </div>
    </template>
  </div>
</template>
