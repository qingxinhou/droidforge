<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  FolderOpen,
  Package,
  FileImage,
  ScanLine,
  Download,
  CheckSquare,
  Square,
  Info,
  AlertTriangle,
  Layers,
  Link2,
  ChevronDown,
  Loader2,
  Globe,
  HardDriveDownload,
} from 'lucide-vue-next'
import { api } from '../api'
import { state, run, toast, fmtBytes } from '../store'
import type { BootImageInfo, PackageInfo, RemoteOtaInfo } from '../types'

const tab = ref<'package' | 'link' | 'image'>('package')

// ---------- 刷机包 ----------
const pkgPath = ref('')
const pkg = ref<PackageInfo | null>(null)
const selectedParts = ref<string[]>([])
const outDir = ref('')
const inspecting = ref(false)

const allSelected = computed(
  () => (pkg.value?.partitions?.length ?? 0) > 0 && selectedParts.value.length === pkg.value?.partitions?.length
)

async function pickPackage() {
  const p = await api.pickFile('选择刷机包', '*.zip;*.tar;*.tar.md5;*.tgz;*.tar.gz;*.*')
  if (!p) return
  pkgPath.value = p
  inspecting.value = true
  try {
    const info = await api.inspectPackage(p)
    pkg.value = info
    selectedParts.value = [...(info.partitions ?? [])]
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
    pkg.value = null
  } finally {
    inspecting.value = false
  }
}

async function pickFolderPackage() {
  const p = await api.pickDirectory('选择镜像文件夹')
  if (!p) return
  pkgPath.value = p
  inspecting.value = true
  try {
    const info = await api.inspectPackage(p)
    pkg.value = info
    selectedParts.value = [...(info.partitions ?? [])]
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
  } finally {
    inspecting.value = false
  }
}

function togglePart(name: string) {
  const i = selectedParts.value.indexOf(name)
  if (i >= 0) selectedParts.value.splice(i, 1)
  else selectedParts.value.push(name)
}

function toggleAllParts() {
  if (allSelected.value) selectedParts.value = []
  else selectedParts.value = [...(pkg.value?.partitions ?? [])]
}

async function pickOutDir() {
  const p = await api.pickDirectory('选择输出目录')
  if (p) outDir.value = p
}

async function doExtract() {
  if (!pkg.value) return
  const names = selectedParts.value.length > 0 ? selectedParts.value : []
  const res = await run('', () => api.extractPackage(pkgPath.value, names, outDir.value))
  if (res && res.length) {
    toast('ok', `已提取 ${res.length} 个镜像`)
    await api.revealInFolder(res[0])
  }
}

// ---------- 远程链接 ----------
const link = ref('')
const linkInfo = ref<RemoteOtaInfo | null>(null)
const linkParts = ref<string[]>([])
const linkOutDir = ref('')
const parsing = ref(false)
const advanced = ref(false)
const ua = ref('')
const referer = ref('')

const uaPresets = [
  { label: '浏览器 UA', value: '' },
  { label: 'Android DownloadManager', value: 'AndroidDownloadManager/14 (Linux; U; Android 14; Build/AP1A)' },
]

watch(
  () => state.env?.config,
  (cfg) => {
    if (!cfg) return
    if (!ua.value) ua.value = cfg.otaUserAgent ?? ''
    if (!referer.value) referer.value = cfg.otaReferer ?? ''
  },
  { immediate: true }
)

const linkAllSelected = computed(
  () => (linkInfo.value?.partitions?.length ?? 0) > 0 && linkParts.value.length === linkInfo.value?.partitions?.length
)

/** 选中分区实际需要下载的数据量 */
const linkDownloadSize = computed(() => {
  const list = linkInfo.value?.partitions ?? []
  return list.filter((p) => linkParts.value.includes(p.name)).reduce((sum, p) => sum + p.compressed, 0)
})

async function parseLink() {
  const url = link.value.trim()
  if (!url) {
    toast('warn', '请先粘贴 OTA 下载链接')
    return
  }
  parsing.value = true
  linkInfo.value = null
  linkParts.value = []
  try {
    await api.saveOtaRequest(ua.value, referer.value)
    const info = await api.inspectRemoteOta(url)
    linkInfo.value = info
    linkParts.value = (info.partitions ?? []).map((p) => p.name)
    linkOutDir.value = await api.otaOutputDir(info.fileName)
    if (!info.supportsRange) toast('warn', info.note || '该链接不支持远程分段解析', 5000)
  } catch (e: any) {
    toast('err', e?.message ?? String(e), 6000)
  } finally {
    parsing.value = false
  }
}

function toggleLinkPart(name: string) {
  const i = linkParts.value.indexOf(name)
  if (i >= 0) linkParts.value.splice(i, 1)
  else linkParts.value.push(name)
}

function toggleAllLinkParts() {
  if (linkAllSelected.value) linkParts.value = []
  else linkParts.value = [...(linkInfo.value?.partitions ?? []).map((p) => p.name)]
}

async function pickLinkOutDir() {
  const p = await api.pickDirectory('选择输出目录')
  if (p) linkOutDir.value = p
}

async function extractRemote() {
  const info = linkInfo.value
  if (!info) return
  const names = linkParts.value.length > 0 ? linkParts.value : []
  const res = await run('', () => api.extractRemoteOta(info.url, names, linkOutDir.value))
  if (res && res.length) {
    toast('ok', `已提取 ${res.length} 个镜像`)
    await api.revealInFolder(res[0])
  }
}

async function downloadWhole() {
  const url = link.value.trim()
  if (!url) return
  const path = await run('', () => api.downloadOta(url))
  if (path) {
    toast('ok', '整包下载完成')
    await api.revealInFolder(path)
  }
}

// ---------- 镜像分析 ----------
const imgPath = ref('')
const imgKind = ref('')
const boot = ref<BootImageInfo | null>(null)
const unpacked = ref<string[]>([])

async function pickImage() {
  const p = await api.pickFile('选择镜像文件', '*.img;*.bin;*.img.lz4;*.*')
  if (!p) return
  imgPath.value = p
  boot.value = null
  unpacked.value = []
  try {
    imgKind.value = await api.identifyImage(p)
  } catch {
    imgKind.value = 'unknown'
  }
  if (imgKind.value === 'boot' || imgKind.value === 'vendor_boot') {
    try {
      boot.value = await api.inspectBootImage(p)
    } catch (e: any) {
      toast('err', e?.message ?? String(e))
    }
  }
}

async function unpack() {
  const res = await run('', () => api.unpackBootImage(imgPath.value, ''))
  if (res) {
    unpacked.value = res
    toast('ok', `已拆分 ${res.length} 个文件`)
  }
}

async function toRaw() {
  const out = await run('', () => api.unsparseImage(imgPath.value, ''))
  if (out) {
    toast('ok', '已转换为 raw 镜像')
    await api.revealInFolder(out)
  }
}

const kindLabel = computed(() => {
  const map: Record<string, string> = {
    boot: 'boot 镜像',
    vendor_boot: 'vendor_boot 镜像',
    payload: 'payload.bin',
    sparse: 'Android sparse 镜像',
    zip: 'ZIP 压缩包',
    ext4: 'ext4 文件系统镜像',
    erofs: 'erofs 文件系统镜像',
    unknown: '未知格式',
  }
  return map[imgKind.value] ?? imgKind.value
})
</script>

<template>
  <div class="p-5">
    <div class="mb-4 flex items-end justify-between">
      <div>
        <h1 class="text-[15px] font-medium text-slate-100">刷机包提取</h1>
        <p class="mt-1 text-[11.5px] text-muted">
          从 OTA 包、线刷包中抽取分区镜像；也可直接粘贴下载链接按需下载，或分析单个镜像。
        </p>
      </div>
      <div class="flex rounded border border-ink-600 bg-ink-850 p-0.5">
        <button
          class="rounded-sm px-3 py-1 text-[11.5px]"
          :class="tab === 'package' ? 'bg-accent/20 text-accent' : 'text-muted'"
          @click="tab = 'package'"
        >刷机包</button>
        <button
          class="rounded-sm px-3 py-1 text-[11.5px]"
          :class="tab === 'link' ? 'bg-accent/20 text-accent' : 'text-muted'"
          @click="tab = 'link'"
        >链接提取</button>
        <button
          class="rounded-sm px-3 py-1 text-[11.5px]"
          :class="tab === 'image' ? 'bg-accent/20 text-accent' : 'text-muted'"
          @click="tab = 'image'"
        >镜像分析</button>
      </div>
    </div>

    <!-- ================= 刷机包 ================= -->
    <div v-if="tab === 'package'" class="grid grid-cols-12 gap-4">
      <div class="col-span-8 flex flex-col gap-4">
        <div class="panel">
          <div class="panel-head"><span class="panel-title">选择刷机包</span></div>
          <div class="flex gap-2 p-4">
            <input class="input mono flex-1" :value="pkgPath" readonly placeholder="支持 .zip / .tar / .tar.md5 / .tgz 或镜像文件夹" />
            <button class="btn" @click="pickPackage"><Package class="h-3.5 w-3.5" /> 选择文件</button>
            <button class="btn" @click="pickFolderPackage"><FolderOpen class="h-3.5 w-3.5" /> 文件夹</button>
          </div>
        </div>

        <div v-if="pkg" class="panel">
          <div class="flex items-center justify-between border-b border-ink-700 px-4 py-2">
            <div class="flex items-center gap-2">
              <span class="tag tag-accent">{{ pkg.kind }}</span>
              <span class="text-[11.5px] text-slate-300">{{ pkg.type }} · {{ fmtBytes(pkg.totalSize) }}</span>
            </div>
            <span class="text-[10.5px] text-muted">{{ pkg.note }}</span>
          </div>

          <!-- 分区选择 -->
          <div v-if="pkg.partitions?.length" class="p-4">
            <div class="mb-2 flex items-center justify-between">
              <span class="text-[11.5px] text-slate-300">可提取分区（{{ pkg.partitions.length }}）</span>
              <button class="btn btn-sm" @click="toggleAllParts">
                {{ allSelected ? '全不选' : '全选' }}
              </button>
            </div>
            <div class="grid grid-cols-4 gap-2">
              <button
                v-for="p in pkg.partitions"
                :key="p"
                class="flex items-center gap-1.5 rounded border px-2 py-1.5 text-left text-[11px] transition-colors"
                :class="selectedParts.includes(p) ? 'border-accent/40 bg-accent/10 text-accent' : 'border-ink-700 bg-ink-900 text-slate-400 hover:border-ink-600'"
                @click="togglePart(p)"
              >
                <CheckSquare v-if="selectedParts.includes(p)" class="h-3 w-3 shrink-0" />
                <Square v-else class="h-3 w-3 shrink-0" />
                <span class="mono truncate">{{ p }}</span>
              </button>
            </div>
          </div>

          <!-- 条目表 -->
          <div v-else class="max-h-[300px] overflow-y-auto">
            <table class="w-full text-[11px]">
              <thead class="sticky top-0 bg-ink-850 text-muted">
                <tr class="border-b border-ink-700">
                  <th class="px-4 py-2 text-left font-normal">文件</th>
                  <th class="px-3 py-2 text-left font-normal">类型</th>
                  <th class="px-3 py-2 text-right font-normal">大小</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="e in pkg.entries" :key="e.name" class="border-b border-ink-800">
                  <td class="px-4 py-1.5 mono text-slate-300">{{ e.name }}</td>
                  <td class="px-3 py-1.5">
                    <span class="tag" :class="e.kind === 'image' ? 'tag-ok' : ''">{{ e.kind }}</span>
                  </td>
                  <td class="px-3 py-1.5 text-right mono text-muted">{{ fmtBytes(e.size) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      <div class="col-span-4 flex flex-col gap-4">
        <div class="panel p-4">
          <div class="text-[11.5px] text-slate-300">输出设置</div>
          <div class="mt-3">
            <label class="label">输出目录</label>
            <div class="flex gap-2">
              <input class="input mono" :value="outDir" readonly placeholder="默认：我的文档/DroidForge" />
              <button class="btn btn-icon" @click="pickOutDir"><FolderOpen class="h-3.5 w-3.5" /></button>
            </div>
          </div>
          <button class="btn btn-primary mt-4 w-full" :disabled="!pkg || inspecting" @click="doExtract">
            <Download class="h-4 w-4" />
            提取{{ selectedParts.length ? ` ${selectedParts.length} ` : '全部' }}个分区
          </button>
        </div>

        <div class="panel flex items-start gap-2 border-accent/20 bg-accent/5 px-3 py-3">
          <Info class="mt-0.5 h-4 w-4 shrink-0 text-accent" />
          <div class="text-[10.5px] leading-relaxed text-muted">
            OTA 包会先解析 payload.bin 清单再按需提取，无需完整解压；
            小米 tgz / 三星 tar.md5 会直接抽取其中的镜像文件。
          </div>
        </div>
      </div>
    </div>

    <!-- ================= 链接提取 ================= -->
    <div v-else-if="tab === 'link'" class="grid grid-cols-12 gap-4">
      <div class="col-span-8 flex flex-col gap-4">
        <div class="panel">
          <div class="panel-head"><span class="panel-title">OTA 下载链接</span></div>
          <div class="p-4">
            <div class="flex gap-2">
              <input
                v-model="link"
                class="input mono flex-1"
                placeholder="粘贴 full OTA 直链，例如 https://…/ota-update.zip"
                @keyup.enter="parseLink"
              />
              <button class="btn btn-primary" :disabled="parsing" @click="parseLink">
                <Loader2 v-if="parsing" class="h-3.5 w-3.5 animate-spin" />
                <ScanLine v-else class="h-3.5 w-3.5" />
                {{ parsing ? '解析中' : '解析' }}
              </button>
            </div>

            <div class="mt-2 flex items-center gap-3">
              <button class="flex items-center gap-1 text-[10.5px] text-muted hover:text-slate-300" @click="advanced = !advanced">
                <ChevronDown class="h-3 w-3 transition-transform" :class="advanced ? 'rotate-180' : ''" />
                高级请求头
              </button>
              <span class="text-[10px] text-muted">部分厂商 CDN 需要特定 UA / Referer 才能访问</span>
            </div>

            <div v-if="advanced" class="mt-3 grid grid-cols-2 gap-3">
              <div>
                <label class="label">User-Agent</label>
                <input v-model="ua" class="input mono" placeholder="留空使用内置浏览器 UA" />
                <div class="mt-1.5 flex gap-1">
                  <button v-for="p in uaPresets" :key="p.label" class="btn btn-sm" @click="ua = p.value">
                    {{ p.label }}
                  </button>
                </div>
              </div>
              <div>
                <label class="label">Referer</label>
                <input v-model="referer" class="input mono" placeholder="可选" />
                <div class="mt-1.5 text-[10px] text-muted">设置会在下次解析时生效</div>
              </div>
            </div>
          </div>
        </div>

        <div v-if="linkInfo" class="panel">
          <div class="flex flex-wrap items-center justify-between gap-2 border-b border-ink-700 px-4 py-2">
            <div class="flex min-w-0 items-center gap-2">
              <span class="tag tag-accent">{{ linkInfo.container === 'payload' ? 'payload.bin' : 'zip' }}</span>
              <span class="mono truncate text-[11.5px] text-slate-300">{{ linkInfo.fileName }}</span>
            </div>
            <div class="flex items-center gap-2 text-[10.5px] text-muted">
              <span class="mono">{{ fmtBytes(linkInfo.totalSize) }}</span>
              <span v-if="linkInfo.partitions?.length" class="tag">{{ linkInfo.partitions.length }} 分区</span>
              <span class="tag" :class="linkInfo.supportsRange ? 'tag-ok' : 'tag-warn'">
                {{ linkInfo.supportsRange ? '支持 Range' : '不支持 Range' }}
              </span>
            </div>
          </div>

          <div class="flex items-start gap-2 px-4 py-2 text-[10.5px] text-muted">
            <Globe class="mt-0.5 h-3 w-3 shrink-0" />
            <span class="break-all">{{ linkInfo.url }}</span>
          </div>

          <div v-if="linkInfo.partitions?.length" class="p-4 pt-2">
            <div class="mb-2 flex items-center justify-between">
              <div class="flex items-center gap-2 text-[11.5px] text-slate-300">
                <span>选择分区</span>
                <span class="text-[10.5px] text-muted">
                  已选 {{ linkParts.length }} 个 · 需下载 {{ fmtBytes(linkDownloadSize) }}
                </span>
              </div>
              <button class="btn btn-sm" @click="toggleAllLinkParts">
                {{ linkAllSelected ? '全不选' : '全选' }}
              </button>
            </div>
            <div class="grid max-h-[320px] grid-cols-3 gap-2 overflow-y-auto pr-1">
              <button
                v-for="p in linkInfo.partitions"
                :key="p.name"
                class="flex flex-col gap-0.5 rounded border px-2 py-1.5 text-left transition-colors"
                :class="linkParts.includes(p.name) ? 'border-accent/40 bg-accent/10' : 'border-ink-700 bg-ink-900 hover:border-ink-600'"
                @click="toggleLinkPart(p.name)"
              >
                <span class="flex items-center gap-1.5">
                  <CheckSquare v-if="linkParts.includes(p.name)" class="h-3 w-3 shrink-0 text-accent" />
                  <Square v-else class="h-3 w-3 shrink-0 text-slate-500" />
                  <span class="mono truncate text-[11px]" :class="linkParts.includes(p.name) ? 'text-accent' : 'text-slate-300'">
                    {{ p.name }}
                  </span>
                  <AlertTriangle v-if="p.incremental" class="h-3 w-3 shrink-0 text-warn" />
                </span>
                <span class="pl-[18px] text-[10px] text-muted">
                  {{ fmtBytes(p.size) }} · 下载 {{ fmtBytes(p.compressed) }}
                </span>
              </button>
            </div>
          </div>
        </div>

        <div v-if="linkInfo && !linkInfo.supportsRange" class="panel flex items-start gap-2 border-warn/25 bg-warn/5 px-3 py-3">
          <AlertTriangle class="mt-0.5 h-4 w-4 shrink-0 text-warn" />
          <div class="text-[10.5px] leading-relaxed text-muted">
            该服务器未返回可用的分段响应，无法远程定位 payload（常见于重定向到镜像站后）。
            可以改用右下角的「下载整包」，下载完成后在「刷机包」页打开即可提取。
          </div>
        </div>
      </div>

      <div class="col-span-4 flex flex-col gap-4">
        <div class="panel p-4">
          <div class="text-[11.5px] text-slate-300">输出设置</div>
          <div class="mt-3">
            <label class="label">输出目录</label>
            <div class="flex gap-2">
              <input class="input mono" :value="linkOutDir" readonly placeholder="解析后自动填充" />
              <button class="btn btn-icon" @click="pickLinkOutDir"><FolderOpen class="h-3.5 w-3.5" /></button>
            </div>
          </div>
          <button
            class="btn btn-primary mt-4 w-full"
            :disabled="!linkInfo?.supportsRange || parsing || state.busy"
            @click="extractRemote"
          >
            <Download class="h-4 w-4" />
            提取{{ linkParts.length ? ` ${linkParts.length} ` : '全部' }}个分区
          </button>
          <button class="btn mt-2 w-full" :disabled="!link || state.busy" @click="downloadWhole">
            <HardDriveDownload class="h-4 w-4" />
            下载整包
          </button>
        </div>

        <div class="panel flex items-start gap-2 border-accent/20 bg-accent/5 px-3 py-3">
          <Link2 class="mt-0.5 h-4 w-4 shrink-0 text-accent" />
          <div class="text-[10.5px] leading-relaxed text-muted">
            直链模式下只用 HTTP Range 读取 zip 目录与 payload 清单，再按分区拉取真正需要的字节，
            提取一个 boot 分区通常只需十几 MB，而不是整包 1–2 GB。
          </div>
        </div>

        <div class="panel flex items-start gap-2 px-3 py-3">
          <Info class="mt-0.5 h-4 w-4 shrink-0 text-muted" />
          <div class="text-[10.5px] leading-relaxed text-muted">
            仅支持 <span class="mono">full OTA</span>（含完整 payload.bin）。增量包缺少底包数据，无法独立还原。
          </div>
        </div>
      </div>
    </div>

    <!-- ================= 镜像分析 ================= -->
    <div v-else class="grid grid-cols-12 gap-4">
      <div class="col-span-8 flex flex-col gap-4">
        <div class="panel">
          <div class="panel-head"><span class="panel-title">选择镜像</span></div>
          <div class="flex gap-2 p-4">
            <input class="input mono flex-1" :value="imgPath" readonly placeholder="boot.img / payload.bin / super.img …" />
            <button class="btn" @click="pickImage"><FileImage class="h-3.5 w-3.5" /> 浏览</button>
          </div>
        </div>

        <div v-if="imgPath" class="panel">
          <div class="flex items-center gap-2 border-b border-ink-700 px-4 py-2">
            <ScanLine class="h-3.5 w-3.5 text-accent" />
            <span class="text-[11.5px] text-slate-300">识别结果</span>
            <span class="tag tag-accent">{{ kindLabel }}</span>
          </div>
          <div class="p-4">
            <div v-if="boot" class="grid grid-cols-2 gap-x-6">
              <div class="kv"><span class="kv-k">类型</span><span class="kv-v mono">{{ boot.kind }}</span></div>
              <div class="kv"><span class="kv-k">头部版本</span><span class="kv-v mono">v{{ boot.headerVersion }}</span></div>
              <div class="kv"><span class="kv-k">文件大小</span><span class="kv-v">{{ fmtBytes(boot.fileSize) }}</span></div>
              <div class="kv"><span class="kv-k">页大小</span><span class="kv-v mono">{{ boot.pageSize }}</span></div>
              <div class="kv"><span class="kv-k">内核</span><span class="kv-v">{{ fmtBytes(boot.kernelSize) }}</span></div>
              <div class="kv"><span class="kv-k">Ramdisk</span><span class="kv-v">{{ fmtBytes(boot.ramdiskSize) }}</span></div>
              <div class="kv"><span class="kv-k">DTB</span><span class="kv-v">{{ fmtBytes(boot.dtbSize) }}</span></div>
              <div class="kv"><span class="kv-k">Recovery DTBO</span><span class="kv-v">{{ fmtBytes(boot.recoveryDtboSize) }}</span></div>
              <div class="kv col-span-2"><span class="kv-k">Cmdline</span><span class="kv-v mono text-[10.5px]">{{ boot.cmdline || '—' }}</span></div>
            </div>
            <div v-else class="text-[11.5px] text-muted">
              <template v-if="imgKind === 'payload'">该文件是 OTA payload，可在「刷机包」页作为 zip 的一部分提取。</template>
              <template v-else-if="imgKind === 'sparse'">检测到 Android sparse 格式，可转换为 raw 镜像后使用。</template>
              <template v-else>未识别到可解析的结构，可直接用于 fastboot 刷入。</template>
            </div>

            <div class="mt-4 flex gap-2">
              <button v-if="boot" class="btn btn-primary" @click="unpack">
                <Layers class="h-3.5 w-3.5" /> 拆分镜像
              </button>
              <button v-if="imgKind === 'sparse'" class="btn" @click="toRaw">
                转换为 raw
              </button>
            </div>

            <div v-if="unpacked.length" class="mt-3 rounded border border-ink-700 bg-ink-900 p-3">
              <div class="mb-1 text-[10.5px] text-muted">已生成文件</div>
              <div v-for="f in unpacked" :key="f" class="mono flex items-center justify-between py-0.5 text-[10.5px] text-slate-300">
                <span class="truncate">{{ f }}</span>
                <button class="btn btn-sm ml-2" @click="api.revealInFolder(f)">定位</button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="col-span-4">
        <div class="panel flex items-start gap-2 border-warn/20 bg-warn/5 p-3">
          <AlertTriangle class="mt-0.5 h-4 w-4 shrink-0 text-warn" />
          <div class="text-[10.5px] leading-relaxed text-muted">
            boot 镜像的拆分结果用于分析或手动修改；重新打包需要专用工具，
            本工具只做解析与提取，避免产出无法启动的镜像。
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
