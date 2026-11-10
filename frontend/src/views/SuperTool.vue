<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  AlertTriangle,
  Boxes,
  CheckSquare,
  FileCode,
  FolderOpen,
  HardDrive,
  Loader2,
  Play,
  ScanLine,
  Square,
} from 'lucide-vue-next'
import { api } from '../api'
import { run, toast, fmtBytes } from '../store'
import type { SplitResult, SuperBuildPartition, SuperMetadata } from '../types'

const tab = ref<'split' | 'merge'>('split')

// ---------------------------------------------------------------------------
// 拆分
// ---------------------------------------------------------------------------

const superPath = ref('')
const meta = ref<SuperMetadata | null>(null)
const selected = ref<string[]>([])
const splitOut = ref('')
const splitting = ref(false)
const splitResults = ref<SplitResult[]>([])

const partitions = computed(() => meta.value?.partitions ?? [])
const allSelected = computed(
  () => partitions.value.length > 0 && selected.value.length === partitions.value.length
)
const selectedBytes = computed(() =>
  partitions.value.filter((p) => selected.value.includes(p.name)).reduce((s, p) => s + p.sizeBytes, 0)
)

async function pickSuper() {
  const p = await api.pickFile('选择 super 镜像', '*.img;*.bin;*.*')
  if (!p) return
  superPath.value = p
  await loadSuper()
}

async function loadSuper() {
  if (!superPath.value) return
  splitting.value = false
  meta.value = null
  selected.value = []
  try {
    const m = await api.inspectSuper(superPath.value)
    meta.value = m
    selected.value = (m.partitions ?? []).map((p) => p.name)
    if (!m.checksumOk) toast('warn', 'metadata 校验和不一致，镜像可能被第三方工具改过')
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
  }
}

function toggleAll() {
  selected.value = allSelected.value ? [] : partitions.value.map((p) => p.name)
}

function toggle(name: string) {
  const i = selected.value.indexOf(name)
  if (i >= 0) selected.value.splice(i, 1)
  else selected.value.push(name)
}

async function doSplit() {
  if (!superPath.value || selected.value.length === 0) return
  splitting.value = true
  splitResults.value = []
  try {
    const res = await run('', () => api.splitSuper(superPath.value, selected.value, splitOut.value))
    if (res) {
      splitResults.value = res
      toast('ok', `已导出 ${res.length} 个分区`)
    }
  } catch (e: any) {
    toast('err', e?.message ?? String(e), 6000)
  } finally {
    splitting.value = false
  }
}

// ---------------------------------------------------------------------------
// 合成
// ---------------------------------------------------------------------------

const srcDir = ref('')
const candidates = ref<SuperBuildPartition[]>([])
const picked = ref<string[]>([])
const outPath = ref('')
const scanning = ref(false)
const merging = ref(false)
const mergedMeta = ref<SuperMetadata | null>(null)

const advanced = ref(false)
const blockSize = ref(4096)
const alignmentMb = ref(1)
const slotCount = ref(2)
const outputSizeGb = ref(0)

const slot = ref('')
const scriptReboot = ref(true)
const scriptWipe = ref(false)
const scriptFiles = ref<string[]>([])

const pickedParts = computed(() => candidates.value.filter((c) => picked.value.includes(c.name)))
const pickedBytes = computed(() => pickedParts.value.reduce((s, c) => s + c.sizeBytes, 0))
const predictedSize = computed(() => {
  const raw = pickedBytes.value
  if (!raw) return 0
  const align = alignmentMb.value * 1024 * 1024
  return Math.ceil(raw / align) * align + align * pickedParts.value.length
})

async function pickDir() {
  const d = await api.pickDirectory('选择散包镜像目录')
  if (!d) return
  srcDir.value = d
  await scanParts()
}

async function scanParts() {
  if (!srcDir.value) return
  scanning.value = true
  candidates.value = []
  picked.value = []
  try {
    const list = await api.scanSuperParts(srcDir.value)
    candidates.value = list
    picked.value = list.map((c) => c.name)
    if (!outPath.value) outPath.value = joinPath(srcDir.value, 'super.img')
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
  } finally {
    scanning.value = false
  }
}

function joinPath(dir: string, name: string) {
  const sep = dir.includes('\\') ? '\\' : '/'
  return dir.replace(/[\\/]+$/, '') + sep + name
}

function togglePart(name: string) {
  const i = picked.value.indexOf(name)
  if (i >= 0) picked.value.splice(i, 1)
  else picked.value.push(name)
}

async function pickOut() {
  const p = await api.pickSaveFile('保存 super 镜像', 'super.img')
  if (p) outPath.value = p
}

async function doMerge() {
  if (pickedParts.value.length === 0) return
  merging.value = true
  mergedMeta.value = null
  try {
    const parts = pickedParts.value.map((c) => ({ name: c.name, path: c.path, group: c.group }))
    const res = await run('', () =>
      api.mergeSuper(parts, outPath.value, {
        label: 'super',
        defaultGroup: 'default',
        blockSize: blockSize.value,
        alignment: alignmentMb.value * 1024 * 1024,
        slotCount: slotCount.value,
        outputSize: Math.round(outputSizeGb.value * 1024 * 1024 * 1024),
      })
    )
    if (res) {
      mergedMeta.value = res
      toast('ok', `已生成 super.img，${res.partitions?.length ?? 0} 个分区`)
    }
  } catch (e: any) {
    toast('err', e?.message ?? String(e), 6000)
  } finally {
    merging.value = false
  }
}

async function makeScript() {
  if (!srcDir.value) return
  scriptFiles.value = []
  try {
    const res = await run('', () =>
      api.generateFlashScript(srcDir.value, srcDir.value, slot.value, scriptReboot.value, scriptWipe.value)
    )
    if (res) {
      scriptFiles.value = res
      toast('ok', `已生成 ${res.length} 个脚本`)
    }
  } catch (e: any) {
    toast('err', e?.message ?? String(e), 6000)
  }
}

function openFolder(p: string) {
  api.revealInFolder(p)
}
</script>

<template>
  <div class="p-5">
    <div class="mb-4 flex items-start justify-between">
      <div>
        <h1 class="text-[15px] font-medium text-slate-100">超级分区</h1>
        <p class="mt-1 text-[11.5px] text-muted">
          解析 <span class="mono text-slate-300">lp_metadata</span>，把 super 里的逻辑分区拆出来，
          或把散包分区合成为一个 super.img。全程只读写本地文件，不接触设备。
        </p>
      </div>
      <div class="flex gap-1.5">
        <button class="btn" :class="tab === 'split' ? 'btn-primary' : ''" @click="tab = 'split'">
          <Boxes class="h-3.5 w-3.5" />拆分
        </button>
        <button class="btn" :class="tab === 'merge' ? 'btn-primary' : ''" @click="tab = 'merge'">
          <HardDrive class="h-3.5 w-3.5" />合成
        </button>
      </div>
    </div>

    <!-- 拆分 -->
    <template v-if="tab === 'split'">
      <div class="panel">
        <div class="panel-head">
          <span class="panel-title">选择镜像</span>
          <div class="flex gap-1.5">
            <button class="btn btn-sm" @click="pickSuper"><FolderOpen class="h-3.5 w-3.5" />选择文件</button>
            <button class="btn btn-sm" :disabled="!superPath" @click="loadSuper">
              <ScanLine class="h-3.5 w-3.5" />重新解析
            </button>
          </div>
        </div>
        <div class="flex items-center gap-2 px-4 py-3">
          <input class="input mono flex-1" :value="superPath" readonly placeholder="super.img / super_empty.img" />
        </div>
      </div>

      <div v-if="meta" class="panel mt-3">
        <div class="panel-head">
          <span class="panel-title">元数据</span>
          <div class="flex items-center gap-1.5">
            <span class="tag" :class="meta.checksumOk ? 'tag-ok' : 'tag-warn'">
              {{ meta.checksumOk ? '校验通过' : '校验和不符' }}
            </span>
            <span v-if="meta.sparse" class="tag tag-accent">sparse</span>
            <span class="tag">v{{ meta.major }}.{{ meta.minor }}</span>
            <span class="tag">slot {{ meta.slot }}</span>
          </div>
        </div>
        <div class="grid grid-cols-4 gap-x-6 px-4 py-3">
          <div class="kv"><span class="kv-k">镜像大小</span><span class="kv-v mono">{{ fmtBytes(meta.totalSize) }}</span></div>
          <div class="kv"><span class="kv-k">逻辑分区</span><span class="kv-v mono">{{ partitions.length }}</span></div>
          <div class="kv"><span class="kv-k">分区组</span><span class="kv-v mono">{{ meta.groups?.length ?? 0 }}</span></div>
          <div class="kv"><span class="kv-k">块大小</span><span class="kv-v mono">{{ meta.geometry.logicalBlockSize }}</span></div>
        </div>
        <div v-if="meta.note" class="px-4 pb-3 text-[11px] text-muted">{{ meta.note }}</div>
      </div>

      <div v-if="meta" class="panel mt-3">
        <div class="panel-head">
          <div class="flex items-center gap-2">
            <span class="panel-title">逻辑分区</span>
            <button class="btn btn-sm" @click="toggleAll">
              <component :is="allSelected ? CheckSquare : Square" class="h-3.5 w-3.5" />
              {{ allSelected ? '取消全选' : '全选' }}
            </button>
          </div>
          <span class="text-[11px] text-muted">
            已选 {{ selected.length }} 项 · {{ fmtBytes(selectedBytes) }}
          </span>
        </div>

        <div class="max-h-[38vh] overflow-y-auto">
          <table class="w-full">
            <thead class="sticky top-0 bg-ink-850">
              <tr class="text-[10.5px] text-muted">
                <th class="w-8 px-3 py-1.5"></th>
                <th class="px-2 py-1.5 text-left font-normal">分区</th>
                <th class="px-2 py-1.5 text-right font-normal">大小</th>
                <th class="px-2 py-1.5 text-left font-normal">组</th>
                <th class="px-2 py-1.5 text-right font-normal">extent</th>
                <th class="px-3 py-1.5 text-right font-normal">属性</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-ink-700/50">
              <tr
                v-for="p in partitions"
                :key="p.name"
                class="h-8 cursor-pointer text-[11.5px] hover:bg-ink-800/60"
                @click="toggle(p.name)"
              >
                <td class="px-3">
                  <component
                    :is="selected.includes(p.name) ? CheckSquare : Square"
                    class="h-3.5 w-3.5"
                    :class="selected.includes(p.name) ? 'text-accent' : 'text-slate-600'"
                  />
                </td>
                <td class="px-2 mono text-slate-200">{{ p.name }}</td>
                <td class="px-2 text-right mono text-slate-300">{{ fmtBytes(p.sizeBytes) }}</td>
                <td class="px-2 text-muted">{{ p.group || 'default' }}</td>
                <td class="px-2 text-right mono text-muted">{{ p.extents?.length ?? 0 }}</td>
                <td class="px-3 text-right">
                  <span v-if="p.slotSuffixed" class="tag mr-1">带槽位</span>
                  <span v-if="p.readOnly" class="tag">只读</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="flex items-center gap-2 border-t border-ink-700 px-4 py-3">
          <input class="input mono flex-1" v-model="splitOut" placeholder="输出目录（留空自动建到输出目录）" />
          <button class="btn btn-primary" :disabled="selected.length === 0 || splitting" @click="doSplit">
            <Loader2 v-if="splitting" class="h-3.5 w-3.5 animate-spin" />
            <Play v-else class="h-3.5 w-3.5" />
            开始拆分
          </button>
        </div>
      </div>

      <div v-if="splitResults.length" class="panel mt-3">
        <div class="panel-head">
          <span class="panel-title">已导出 {{ splitResults.length }} 个镜像</span>
          <button class="btn btn-sm" @click="openFolder(splitResults[0].path)">打开目录</button>
        </div>
        <div class="max-h-[24vh] overflow-y-auto px-4 py-2">
          <div v-for="r in splitResults" :key="r.name" class="kv">
            <span class="kv-k mono">{{ r.name }}.img</span>
            <span class="kv-v mono">{{ fmtBytes(r.size) }}</span>
          </div>
        </div>
      </div>
    </template>

    <!-- 合成 -->
    <template v-else>
      <div class="panel">
        <div class="panel-head">
          <span class="panel-title">散包目录</span>
          <div class="flex gap-1.5">
            <button class="btn btn-sm" @click="pickDir"><FolderOpen class="h-3.5 w-3.5" />选择目录</button>
            <button class="btn btn-sm" :disabled="!srcDir" @click="scanParts">
              <ScanLine class="h-3.5 w-3.5" />重新扫描
            </button>
          </div>
        </div>
        <div class="px-4 py-3">
          <input class="input mono w-full" :value="srcDir" readonly placeholder="包含 system.img / vendor.img 等散包的目录" />
          <div class="mt-2 text-[11px] leading-relaxed text-muted">
            目录里若有 <span class="mono">super_def</span> 或
            <span class="mono">dynamic_partitions_info.txt</span>，会按其中的分组自动归类，否则统一放进 default 组。
          </div>
        </div>
      </div>

      <div v-if="candidates.length" class="panel mt-3">
        <div class="panel-head">
          <div class="flex items-center gap-2">
            <span class="panel-title">候选分区</span>
            <span class="tag">{{ picked.length }}/{{ candidates.length }}</span>
          </div>
          <span class="text-[11px] text-muted">原始数据 {{ fmtBytes(pickedBytes) }} · 预计需 {{ fmtBytes(predictedSize) }}</span>
        </div>
        <div class="max-h-[34vh] overflow-y-auto">
          <table class="w-full">
            <thead class="sticky top-0 bg-ink-850">
              <tr class="text-[10.5px] text-muted">
                <th class="w-8 px-3 py-1.5"></th>
                <th class="px-2 py-1.5 text-left font-normal">分区</th>
                <th class="px-2 py-1.5 text-right font-normal">大小</th>
                <th class="w-56 px-3 py-1.5 text-left font-normal">组</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-ink-700/50">
              <tr v-for="c in candidates" :key="c.name" class="h-8 text-[11.5px]">
                <td class="px-3 cursor-pointer" @click="togglePart(c.name)">
                  <component
                    :is="picked.includes(c.name) ? CheckSquare : Square"
                    class="h-3.5 w-3.5"
                    :class="picked.includes(c.name) ? 'text-accent' : 'text-slate-600'"
                  />
                </td>
                <td class="px-2 mono text-slate-200">{{ c.name }}</td>
                <td class="px-2 text-right mono text-slate-300">{{ fmtBytes(c.sizeBytes) }}</td>
                <td class="px-3 py-1">
                  <input class="input h-6 py-0 text-[11px]" v-model="c.group" placeholder="default" />
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="border-t border-ink-700 px-4 py-3">
          <div class="flex items-center gap-2">
            <input class="input mono flex-1" v-model="outPath" placeholder="输出路径" />
            <button class="btn btn-sm" @click="pickOut">另存为</button>
            <button class="btn btn-primary" :disabled="picked.length === 0 || merging" @click="doMerge">
              <Loader2 v-if="merging" class="h-3.5 w-3.5 animate-spin" />
              <Play v-else class="h-3.5 w-3.5" />
              合成 super
            </button>
          </div>

          <button class="mt-2 text-[11px] text-muted hover:text-slate-300" @click="advanced = !advanced">
            {{ advanced ? '收起' : '展开' }}高级选项
          </button>
          <div v-if="advanced" class="mt-2 grid grid-cols-4 gap-3">
            <div>
              <label class="label">逻辑块大小</label>
              <input class="input mono" type="number" v-model.number="blockSize" />
            </div>
            <div>
              <label class="label">对齐（MB）</label>
              <input class="input mono" type="number" v-model.number="alignmentMb" />
            </div>
            <div>
              <label class="label">metadata 槽位数</label>
              <input class="input mono" type="number" v-model.number="slotCount" />
            </div>
            <div>
              <label class="label">指定总大小（GB，0 自动）</label>
              <input class="input mono" type="number" step="0.5" v-model.number="outputSizeGb" />
            </div>
          </div>
        </div>
      </div>

      <div v-if="mergedMeta" class="panel mt-3">
        <div class="panel-head">
          <span class="panel-title">合成结果</span>
          <button class="btn btn-sm" @click="openFolder(mergedMeta!.path)">打开目录</button>
        </div>
        <div class="grid grid-cols-4 gap-x-6 px-4 py-3">
          <div class="kv"><span class="kv-k">分区数</span><span class="kv-v mono">{{ mergedMeta.partitions?.length ?? 0 }}</span></div>
          <div class="kv"><span class="kv-k">镜像大小</span><span class="kv-v mono">{{ fmtBytes(mergedMeta.totalSize) }}</span></div>
          <div class="kv"><span class="kv-k">metadata</span><span class="kv-v mono">v{{ mergedMeta.major }}.{{ mergedMeta.minor }}</span></div>
          <div class="kv"><span class="kv-k">校验</span><span class="kv-v mono">{{ mergedMeta.checksumOk ? '通过' : '不符' }}</span></div>
        </div>
      </div>

      <!-- 线刷脚本 -->
      <div class="panel mt-3">
        <div class="panel-head">
          <span class="panel-title">线刷脚本</span>
          <button class="btn btn-sm" :disabled="!srcDir" @click="makeScript">
            <FileCode class="h-3.5 w-3.5" />生成 bat / sh / txt
          </button>
        </div>
        <div class="flex items-center gap-4 px-4 py-3">
          <div class="w-32">
            <label class="label">槽位后缀</label>
            <select class="input" v-model="slot">
              <option value="">不添加</option>
              <option value="a">_a</option>
              <option value="b">_b</option>
            </select>
          </div>
          <label class="mt-4 flex items-center gap-1.5 text-[11.5px] text-slate-300">
            <input type="checkbox" v-model="scriptReboot" />末尾重启到系统
          </label>
          <label class="mt-4 flex items-center gap-1.5 text-[11.5px] text-slate-300">
            <input type="checkbox" v-model="scriptWipe" />刷写前 fastboot -w
          </label>
          <div class="flex-1"></div>
          <div v-if="scriptFiles.length" class="mt-4 text-[11px] text-muted">
            已生成 {{ scriptFiles.length }} 个文件
          </div>
        </div>
        <div v-if="scriptFiles.length" class="border-t border-ink-700 px-4 py-2">
          <button
            v-for="f in scriptFiles"
            :key="f"
            class="mono mr-3 text-[11px] text-accent hover:underline"
            @click="openFolder(f)"
          >
            {{ f.split(/[\\/]/).pop() }}
          </button>
        </div>
      </div>
    </template>

    <div class="panel mt-3 flex items-start gap-2 border-warn/20 bg-warn/5 px-4 py-3">
      <AlertTriangle class="mt-0.5 h-4 w-4 shrink-0 text-warn" />
      <div class="text-[11.5px] leading-relaxed text-muted">
        <span class="text-warn">注意</span>
        · 合成的 super 只保证 metadata 结构正确，不代表适配你的机型：分区名、槽位、组容量必须与目标设备一致，
        写错会导致无法开机。刷入前请先用原厂包比对分区表。
      </div>
    </div>
  </div>
</template>
