<script setup lang="ts">
import { computed, ref } from 'vue'
import { AlertTriangle, FileSearch, Loader2, ScanLine, ShieldCheck, ShieldOff } from 'lucide-vue-next'
import { api } from '../api'
import { run, toast, fmtBytes } from '../store'
import type { ARBReport, ARBVerdict, VBMetaReport } from '../types'

const tab = ref<'vbmeta' | 'arb'>('vbmeta')

// ---------------------------------------------------------------------------
// vbmeta
// ---------------------------------------------------------------------------

const vbPath = ref('')
const vbReport = ref<VBMetaReport | null>(null)
const vbBusy = ref(false)

const img = computed(() => vbReport.value?.image ?? null)
const descs = computed(() => img.value?.descriptors ?? [])

async function pickVBMeta() {
  const p = await api.pickFile('选择 vbmeta 或带 AVB 的镜像', '*.img;*.bin;*.*')
  if (!p) return
  vbPath.value = p
  await loadVBMeta()
}

async function loadVBMeta() {
  if (!vbPath.value) return
  vbBusy.value = true
  vbReport.value = null
  try {
    const r = await api.inspectVBMeta(vbPath.value)
    vbReport.value = r
    if (r.level === 'risk') toast('warn', r.summary, 6000)
  } catch (e: any) {
    toast('err', e?.message ?? String(e), 6000)
  } finally {
    vbBusy.value = false
  }
}

function levelClass(level: string) {
  if (level === 'risk') return 'tag-danger'
  if (level === 'warn') return 'tag-warn'
  return 'tag-ok'
}

function kindLabel(k: string) {
  const map: Record<string, string> = {
    hash: '分区哈希',
    hashtree: 'dm-verity 树',
    chain_partition: '链式分区',
    kernel_cmdline: '内核 cmdline',
    property: '属性',
  }
  return map[k] ?? k
}

// ---------------------------------------------------------------------------
// ARB
// ---------------------------------------------------------------------------

const arbPath = ref('')
const arbReport = ref<ARBReport | null>(null)
const arbBusy = ref(false)
const curVal = ref<number | null>(null)
const tgtVal = ref<number | null>(null)
const verdict = ref<ARBVerdict | null>(null)

async function pickARB() {
  const p = await api.pickFile('选择引导镜像（xbl / abl / emmc_appsboot）', '*.elf;*.mbn;*.bin;*.img;*.*')
  if (!p) return
  arbPath.value = p
  await loadARB()
}

async function loadARB() {
  if (!arbPath.value) return
  arbBusy.value = true
  arbReport.value = null
  verdict.value = null
  try {
    const r = await api.scanARB(arbPath.value)
    arbReport.value = r
    if (r.guestOk) tgtVal.value = r.guest
  } catch (e: any) {
    toast('err', e?.message ?? String(e), 6000)
  } finally {
    arbBusy.value = false
  }
}

async function doCompare() {
  if (curVal.value === null || tgtVal.value === null) {
    toast('warn', '两个值都要填，拿不到的那一侧填 -1 表示未知')
    return
  }
  try {
    verdict.value = await api.compareARB(curVal.value, tgtVal.value)
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
  }
}

const confLabel = computed(() => {
  const c = arbReport.value?.file.confidence
  return { high: '高', medium: '中', low: '低', none: '无' }[c ?? 'none'] ?? c
})
</script>

<template>
  <div>
    <div class="mb-4 flex items-start justify-between gap-4">
      <div>
        <h1 class="text-[17px] font-medium text-slate-100">引导校验</h1>
        <p class="mt-1 text-[11.5px] text-muted">
          解析 vbmeta 的签名、rollback index 与描述符；扫描引导镜像里的防回滚标记
        </p>
      </div>
      <div class="flex gap-1">
        <button class="btn btn-sm" :class="tab === 'vbmeta' ? 'btn-primary' : ''" @click="tab = 'vbmeta'">
          <ShieldCheck class="h-3.5 w-3.5" />vbmeta
        </button>
        <button class="btn btn-sm" :class="tab === 'arb' ? 'btn-primary' : ''" @click="tab = 'arb'">
          <ShieldOff class="h-3.5 w-3.5" />ARB
        </button>
      </div>
    </div>

    <!-- vbmeta -->
    <template v-if="tab === 'vbmeta'">
      <div class="panel">
        <div class="panel-head">
          <span class="panel-title">镜像</span>
          <button class="btn btn-sm" @click="pickVBMeta"><FileSearch class="h-3.5 w-3.5" />选择</button>
        </div>
        <div class="px-4 py-3">
          <input class="input mono w-full" :value="vbPath" readonly placeholder="vbmeta.img，或带 AVB footer 的 boot.img / system.img" />
          <div class="mt-2 text-[11px] text-muted">
            裸 vbmeta 直接从开头读；boot / system 这类镜像会读末尾的 <span class="mono">AVBf</span> footer 定位 vbmeta。
          </div>
        </div>
      </div>

      <div v-if="vbBusy" class="panel mt-3 flex items-center gap-2 px-4 py-6 text-[12px] text-muted">
        <Loader2 class="h-4 w-4 animate-spin" />正在解析…
      </div>

      <template v-if="img && vbReport">
        <div class="panel mt-3">
          <div class="panel-head">
            <div class="flex items-center gap-2">
              <span class="panel-title">{{ img.path.split(/[\\/]/).pop() }}</span>
              <span class="tag" :class="levelClass(vbReport.level)">
                {{ vbReport.level === 'risk' ? '未签名' : vbReport.level === 'warn' ? '校验已关' : '已签名' }}
              </span>
            </div>
            <span class="text-[11px] text-muted mono">{{ img.algorithm }}</span>
          </div>
          <div class="px-4 pt-3 text-[12px] leading-relaxed" :class="vbReport.level === 'ok' ? 'text-slate-300' : 'text-warn'">
            {{ vbReport.summary }}
          </div>
          <div class="grid grid-cols-4 gap-x-6 px-4 py-3">
            <div class="kv"><span class="kv-k">rollback index</span><span class="kv-v mono">{{ img.rollbackIndex }}</span></div>
            <div class="kv"><span class="kv-k">槽位</span><span class="kv-v mono">{{ img.rollbackIndexLocation }}</span></div>
            <div class="kv"><span class="kv-k">vbmeta 偏移</span><span class="kv-v mono">{{ img.vbmetaOffset }}</span></div>
            <div class="kv"><span class="kv-k">版本</span><span class="kv-v mono">v{{ img.versionMajor }}.{{ img.versionMinor }}</span></div>
            <div class="kv"><span class="kv-k">签名长度</span><span class="kv-v mono">{{ img.signatureSize || '—' }}</span></div>
            <div class="kv"><span class="kv-k">公钥长度</span><span class="kv-v mono">{{ img.publicKeySize || '—' }}</span></div>
            <div class="kv"><span class="kv-k">flags</span><span class="kv-v mono">0x{{ img.flags.toString(16) }}</span></div>
            <div class="kv"><span class="kv-k">工具标记</span><span class="kv-v mono truncate">{{ img.releaseString || '—' }}</span></div>
          </div>
          <div v-if="img.notes.length" class="border-t border-ink-700 px-4 py-2">
            <div v-for="(n, i) in img.notes" :key="i" class="flex items-start gap-1.5 py-0.5 text-[11px] text-muted">
              <AlertTriangle class="mt-0.5 h-3 w-3 shrink-0" />{{ n }}
            </div>
          </div>
        </div>

        <div class="panel mt-3">
          <div class="panel-head">
            <span class="panel-title">描述符</span>
            <span class="text-[11px] text-muted">{{ descs.length }} 个</span>
          </div>
          <div v-if="descs.length === 0" class="px-4 py-4 text-[11.5px] text-muted">
            没有描述符。这种 vbmeta 只用来下发 flags，super 分区的 vbmeta 常见此结构。
          </div>
          <div v-else class="max-h-[42vh] overflow-y-auto">
            <table class="w-full">
              <thead class="sticky top-0 bg-ink-850">
                <tr class="text-[10.5px] text-muted">
                  <th class="px-4 py-1.5 text-left font-normal">类型</th>
                  <th class="px-2 py-1.5 text-left font-normal">分区</th>
                  <th class="px-2 py-1.5 text-right font-normal">大小</th>
                  <th class="px-4 py-1.5 text-left font-normal">细节</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-ink-700/50">
                <tr v-for="(d, i) in descs" :key="i" class="text-[11.5px]">
                  <td class="px-4 py-1.5 text-slate-300">{{ kindLabel(d.kind) }}</td>
                  <td class="px-2 py-1.5 mono text-slate-400">{{ d.partitionName || d.key || '—' }}</td>
                  <td class="px-2 py-1.5 text-right mono text-muted">{{ d.imageSize ? fmtBytes(d.imageSize) : '—' }}</td>
                  <td class="px-4 py-1.5 text-[11px] text-muted">
                    <span v-if="d.hashAlgorithm" class="mono">{{ d.hashAlgorithm }}</span>
                    <span v-if="d.cmdline" class="mono break-all">{{ d.cmdline }}</span>
                    <span v-if="d.dataBlockSize">{{ d.dataBlockSize }} 字节块</span>
                    <span v-if="d.digest" class="mono"> · digest {{ d.digest.slice(0, 16) }}…</span>
                    <span v-if="d.value" class="mono">{{ d.value }}</span>
                    <span v-if="d.rollbackIndexLocation !== undefined"> · 槽 {{ d.rollbackIndexLocation }}</span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </template>
    </template>

    <!-- ARB -->
    <template v-else>
      <div class="panel">
        <div class="panel-head">
          <span class="panel-title">引导镜像</span>
          <button class="btn btn-sm" @click="pickARB"><ScanLine class="h-3.5 w-3.5" />扫描</button>
        </div>
        <div class="px-4 py-3">
          <input class="input mono w-full" :value="arbPath" readonly placeholder="xbl.elf / abl.elf / emmc_appsboot.mbn" />
          <div class="mt-2 flex items-start gap-1.5 text-[11px] leading-relaxed text-warn">
            <AlertTriangle class="mt-0.5 h-3 w-3 shrink-0" />
            <span>
              ARB 版本没有跨厂商的统一格式，这里按字符串标记做启发式扫描，
              结果仅供参考。确定性的判断请看 vbmeta 里的 rollback index。
            </span>
          </div>
        </div>
      </div>

      <div v-if="arbBusy" class="panel mt-3 flex items-center gap-2 px-4 py-6 text-[12px] text-muted">
        <Loader2 class="h-4 w-4 animate-spin" />正在扫描…
      </div>

      <template v-if="arbReport">
        <div class="panel mt-3">
          <div class="panel-head">
            <span class="panel-title">{{ arbPath.split(/[\\/]/).pop() }}</span>
            <span class="tag" :class="arbReport.file.confidence === 'none' ? 'tag-warn' : 'tag-ok'">
              置信度 {{ confLabel }}
            </span>
          </div>
          <div class="grid grid-cols-4 gap-x-6 px-4 py-3">
            <div class="kv"><span class="kv-k">格式</span><span class="kv-v mono">{{ arbReport.file.format }}</span></div>
            <div class="kv"><span class="kv-k">文件大小</span><span class="kv-v mono">{{ fmtBytes(arbReport.file.size) }}</span></div>
            <div class="kv"><span class="kv-k">已扫描</span><span class="kv-v mono">{{ fmtBytes(arbReport.file.scanned) }}</span></div>
            <div class="kv"><span class="kv-k">命中</span><span class="kv-v mono">{{ arbReport.file.hits.length }}</span></div>
          </div>
          <div class="px-4 pb-3 text-[12px] leading-relaxed text-slate-300">
            <span class="tag mr-2" :class="arbReport.verdict.level === 'ok' ? 'tag-ok' : 'tag-warn'">
              {{ arbReport.verdict.title }}
            </span>
            {{ arbReport.verdict.detail }}
          </div>
          <div v-if="arbReport.file.note" class="border-t border-ink-700 px-4 py-2 text-[11px] text-muted">
            {{ arbReport.file.note }}
          </div>
        </div>

        <div v-if="arbReport.file.hits.length" class="panel mt-3">
          <div class="panel-head">
            <span class="panel-title">命中明细</span>
            <span class="text-[11px] text-muted">偏移按文件字节计</span>
          </div>
          <div class="max-h-[32vh] overflow-y-auto">
            <table class="w-full">
              <thead class="sticky top-0 bg-ink-850">
                <tr class="text-[10.5px] text-muted">
                  <th class="px-4 py-1.5 text-left font-normal">标记</th>
                  <th class="px-2 py-1.5 text-left font-normal">编码</th>
                  <th class="px-2 py-1.5 text-right font-normal">偏移</th>
                  <th class="px-2 py-1.5 text-right font-normal">距离</th>
                  <th class="px-4 py-1.5 text-right font-normal">读出值</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-ink-700/50">
                <tr v-for="(h, i) in arbReport.file.hits" :key="i" class="text-[11.5px]">
                  <td class="px-4 py-1.5 mono text-slate-300">{{ h.marker }}</td>
                  <td class="px-2 py-1.5 text-muted">{{ h.encoding }}</td>
                  <td class="px-2 py-1.5 text-right mono text-muted">0x{{ h.offset.toString(16) }}</td>
                  <td class="px-2 py-1.5 text-right mono text-muted">{{ h.delta }}</td>
                  <td class="px-4 py-1.5 text-right mono text-slate-200">{{ h.value }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </template>

      <div class="panel mt-3">
        <div class="panel-head">
          <span class="panel-title">降级判断</span>
        </div>
        <div class="grid grid-cols-2 gap-3 px-4 py-3">
          <label class="block">
            <span class="mb-1 block text-[11px] text-muted">设备当前固件</span>
            <input v-model.number="curVal" type="number" class="input mono w-full" placeholder="例如 3" />
          </label>
          <label class="block">
            <span class="mb-1 block text-[11px] text-muted">目标固件</span>
            <input v-model.number="tgtVal" type="number" class="input mono w-full" placeholder="例如 4" />
          </label>
        </div>
        <div class="flex items-center justify-between border-t border-ink-700 px-4 py-3">
          <span class="text-[11px] text-muted">两侧任一未知就填 -1</span>
          <button class="btn btn-primary btn-sm" @click="doCompare">比较</button>
        </div>
        <div v-if="verdict" class="border-t border-ink-700 px-4 py-3">
          <div class="text-[12px] text-slate-200">{{ verdict.title }}</div>
          <div class="mt-1 text-[11.5px] leading-relaxed text-muted">{{ verdict.detail }}</div>
        </div>
      </div>
    </template>
  </div>
</template>
