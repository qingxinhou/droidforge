<script setup lang="ts">
import { computed, ref } from 'vue'
import { FolderOpen, ScanLine, Play, Layers, AlertTriangle, CheckSquare, Square, Package } from 'lucide-vue-next'
import { api } from '../api'
import { state, run, needFastboot, toast, fmtBytes } from '../store'
import { confirmDialog } from '../dialog'
import RiskBanner from '../components/RiskBanner.vue'
import { RISK_POINTS } from '../legal'
import type { FlashStep } from '../types'

const dir = ref('')
const steps = ref<FlashStep[]>([])
const selected = ref<boolean[]>([])
const scanning = ref(false)

const totalSize = computed(() => steps.value.reduce((s, x) => s + x.sizeBytes, 0))
const selectedSteps = computed(() => steps.value.filter((_, i) => selected.value[i]))
const selectedSize = computed(() => selectedSteps.value.reduce((s, x) => s + x.sizeBytes, 0))

async function pickDir() {
  const p = await api.pickDirectory('选择镜像所在文件夹')
  if (!p) return
  dir.value = p
  await scan()
}

/** 直接从刷机包解包并生成刷机计划 */
async function importPackage() {
  const p = await api.pickFile('选择刷机包', '*.zip;*.tgz;*.tar;*.tar.md5;*.tar.gz')
  if (!p) return
  const base = state.env?.config.outputDir || ''
  const outDir = `${base ? base + '/' : ''}rom_${Date.now()}`
  const files = await run('', () => api.extractPackage(p, [], outDir))
  if (!files || files.length === 0) return
  const first = files[0].replace(/[\\/][^\\/]+$/, '')
  dir.value = first
  await scan()
  toast('ok', `已解包 ${files.length} 个镜像`)
}

async function scan() {
  if (!dir.value) return
  scanning.value = true
  try {
    const list = await api.buildFlashPlan(dir.value)
    steps.value = list
    selected.value = list.map(() => true)
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
    steps.value = []
  } finally {
    scanning.value = false
  }
}

function toggle(i: number) {
  selected.value[i] = !selected.value[i]
}

function toggleAll(v: boolean) {
  selected.value = steps.value.map(() => v)
}

async function start() {
  if (selectedSteps.value.length === 0) {
    toast('warn', '请至少选择一个分区')
    return
  }
  if (!needFastboot()) return
  const ok = await confirmDialog({
    title: '开始线刷',
    message: `即将刷入 ${selectedSteps.value.length} 个分区，共 ${fmtBytes(selectedSize.value)}。\n\n刷机过程中请勿断开 USB 或关机。确认开始？`,
    danger: true,
    risks: RISK_POINTS.lineflash,
    confirmText: '开始刷入',
  })
  if (!ok) return
  await run('线刷完成', () => api.flashPlan(state.serial, selectedSteps.value))
}
</script>

<template>
  <div class="p-5">
    <div class="mb-4 flex items-end justify-between">
      <div>
        <h1 class="text-[15px] font-medium text-slate-100">线刷整包</h1>
        <p class="mt-1 text-[11.5px] text-muted">
          选择解压后的镜像目录，自动生成刷机顺序并批量执行。动态分区会自动切换 fastbootd。
        </p>
      </div>
      <div class="flex gap-2">
        <input class="input mono w-[280px]" :value="dir" readonly placeholder="尚未选择目录" />
        <button class="btn" @click="pickDir"><FolderOpen class="h-3.5 w-3.5" /> 选择目录</button>
        <button class="btn btn-primary" @click="importPackage"><Package class="h-3.5 w-3.5" /> 从刷机包导入</button>
        <button class="btn" :disabled="!dir || scanning" @click="scan">
          <ScanLine class="h-3.5 w-3.5" /> 重新扫描
        </button>
      </div>
    </div>

    <RiskBanner
      title="整包线刷是风险最高的操作"
      desc="它会重写全部分区，通常清空用户数据；中途断开或固件不匹配都可能让设备变砖。执行前请确认固件与机型、区域完全对应。"
      :points="RISK_POINTS.lineflash"
    />

    <div v-if="!dir" class="panel grid-bg flex flex-col items-center justify-center py-24">
      <Layers class="mb-4 h-10 w-10 text-slate-600" />
      <div class="text-[13px] text-slate-300">选择一个包含 .img 镜像的文件夹</div>
      <div class="mt-2 max-w-lg text-center text-[11.5px] leading-relaxed text-muted">
        支持小米 tgz、三星 tar.md5 解压后的目录，或任意 fastboot 刷机包解压结果。
        也可以先用「刷机包提取」把 OTA 包中的镜像导出再回到这里。
      </div>
    </div>

    <div v-else class="grid grid-cols-12 gap-4">
      <div class="panel col-span-9 overflow-hidden">
        <div class="flex items-center justify-between border-b border-ink-700 px-4 py-2">
          <div class="flex items-center gap-3">
            <span class="text-[11.5px] text-slate-300">共 {{ steps.length }} 个分区</span>
            <span class="tag">{{ fmtBytes(totalSize) }}</span>
            <span class="tag tag-accent">已选 {{ selectedSteps.length }}</span>
          </div>
          <div class="flex gap-1">
            <button class="btn btn-sm" @click="toggleAll(true)">全选</button>
            <button class="btn btn-sm" @click="toggleAll(false)">全不选</button>
          </div>
        </div>

        <div class="max-h-[420px] overflow-y-auto">
          <div v-if="steps.length === 0" class="py-16 text-center text-[12px] text-muted">
            目录中未找到可识别的镜像文件
          </div>
          <table v-else class="w-full text-[11.5px]">
            <thead class="sticky top-0 bg-ink-850 text-muted">
              <tr class="border-b border-ink-700">
                <th class="w-10 px-3 py-2 text-left"></th>
                <th class="px-3 py-2 text-left font-normal">分区</th>
                <th class="px-3 py-2 text-left font-normal">镜像文件</th>
                <th class="px-3 py-2 text-right font-normal">大小</th>
                <th class="px-3 py-2 text-left font-normal">备注</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="(s, i) in steps"
                :key="s.partition"
                class="border-b border-ink-800 hover:bg-ink-800/60"
                @click="toggle(i)"
              >
                <td class="px-3 py-1.5">
                  <CheckSquare v-if="selected[i]" class="h-3.5 w-3.5 text-accent" />
                  <Square v-else class="h-3.5 w-3.5 text-slate-600" />
                </td>
                <td class="px-3 py-1.5">
                  <span class="mono text-slate-200">{{ s.partition }}</span>
                </td>
                <td class="px-3 py-1.5 text-slate-400">{{ s.image.split(/[\\/]/).pop() }}</td>
                <td class="px-3 py-1.5 text-right mono text-slate-400">{{ fmtBytes(s.sizeBytes) }}</td>
                <td class="px-3 py-1.5">
                  <span v-if="s.needsFastbootd" class="tag tag-warn">fastbootd</span>
                  <span v-else class="text-slate-600">—</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div class="col-span-3 flex flex-col gap-4">
        <div class="panel p-4">
          <div class="text-[11.5px] text-slate-300">执行摘要</div>
          <div class="mt-3 space-y-2">
            <div class="kv"><span class="kv-k">分区数</span><span class="kv-v">{{ selectedSteps.length }}</span></div>
            <div class="kv"><span class="kv-k">总大小</span><span class="kv-v">{{ fmtBytes(selectedSize) }}</span></div>
            <div class="kv">
              <span class="kv-k">动态分区</span>
              <span class="kv-v">{{ selectedSteps.filter(s => s.needsFastbootd).length }}</span>
            </div>
          </div>
          <button class="btn btn-primary mt-4 w-full" @click="start">
            <Play class="h-4 w-4" /> 开始线刷
          </button>
        </div>

        <div class="panel flex items-start gap-2 border-warn/20 bg-warn/5 px-3 py-3">
          <AlertTriangle class="mt-0.5 h-4 w-4 shrink-0 text-warn" />
          <div class="text-[10.5px] leading-relaxed text-muted">
            线刷前请确认镜像与机型、系统版本匹配。刷入顺序按引导分区优先排列，动态分区会在进入
            fastbootd 后执行。
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
