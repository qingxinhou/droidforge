<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { FileImage, FolderOpen, Upload, Info, AlertTriangle, Layers } from 'lucide-vue-next'
import { api } from '../api'
import { state, run, needFastboot, toast, fmtBytes } from '../store'
import { confirmDialog } from '../dialog'
import RiskBanner from '../components/RiskBanner.vue'
import { RISK_POINTS } from '../legal'
import type { BootImageInfo } from '../types'

const imagePath = ref('')
const partition = ref('boot')
const slot = ref('')
const bootInfo = ref<BootImageInfo | null>(null)
const devicePartitions = ref<string[]>([])
/** 分区列表来源：adb（系统内）/ bootloader（内核态 fastboot）/ fastbootd（用户态） */
const partitionSource = ref<'adb' | 'bootloader' | 'fastbootd' | ''>('')

const commonPartitions = [
  'boot', 'init_boot', 'vendor_boot', 'recovery', 'dtbo', 'vbmeta', 'vbmeta_system',
  'vbmeta_vendor', 'super', 'system', 'system_ext', 'vendor', 'product', 'odm',
  'modem', 'abl', 'xbl', 'xbl_config', 'tz', 'hyp', 'keymaster', 'keystore',
  'persist', 'misc', 'userdata', 'cache',
]

const partitionOptions = computed(() => {
  const set = new Set<string>(commonPartitions)
  for (const p of devicePartitions.value) set.add(p)
  return Array.from(set)
})

/** 分区名本身已带槽位（bootloader 读取到的都是 boot_a / boot_b） */
const hasSlotSuffix = computed(() => /_[ab]$/.test(partition.value))

const finalPartition = computed(() => {
  // 名字里已经有槽位时不能再叠一次，否则会刷成 boot_a_a 这种不存在分区
  if (hasSlotSuffix.value) return partition.value
  if (!slot.value) return partition.value
  return `${partition.value}_${slot.value}`
})

const imageName = computed(() => (imagePath.value ? imagePath.value.split(/[\\/]/).pop() : ''))

async function pickImage() {
  const p = await api.pickFile('选择镜像文件', '*.img;*.bin;*.img.lz4;*.*')
  if (!p) return
  imagePath.value = p
  bootInfo.value = null
  try {
    const info = await api.inspectBootImage(p)
    bootInfo.value = info
    if (info.kind === 'init_boot') partition.value = 'init_boot'
    else if (info.kind === 'vendor_boot') partition.value = 'vendor_boot'
    else partition.value = 'boot'
  } catch {
    // 非 boot 镜像，忽略
    bootInfo.value = null
  }
}

/**
 * 读取目标设备的分区名。
 *
 * 关键分叉：刷 boot 本来就是进 fastboot 做的事，而 fastboot 模式下 adb 完全
 * 不可见（`adb devices` 为空），走 GetPartitions 必然失败、下拉框空着，
 * 用户连着设备却选不出分区。所以这里按当前模式选数据源。
 */
async function loadDevicePartitions() {
  if (!state.serial) {
    toast('warn', '请先选择设备')
    return
  }
  try {
    const list =
      state.mode === 'fastboot'
        ? await api.fastbootPartitions(state.serial)
        : await api.getPartitions(state.serial)
    devicePartitions.value = list.map((p) => p.name)
    if (state.mode === 'fastboot') {
      // fastbootd 看到的是 super 里的逻辑分区，bootloader 只有物理分区
      const logical = list.length > 0 && list[0].kind === 'logical'
      partitionSource.value = logical ? 'fastbootd' : 'bootloader'
      toast('ok', `从 ${logical ? 'fastbootd' : 'bootloader'} 读取到 ${list.length} 个分区`)
    } else {
      partitionSource.value = 'adb'
      toast('ok', `读取到 ${list.length} 个分区`)
    }
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
  }
}

async function flash() {
  if (!imagePath.value) {
    toast('warn', '请先选择镜像文件')
    return
  }
  if (!needFastboot()) return
  const target = finalPartition.value
  const ok = await confirmDialog({
    title: '确认刷入',
    message: `即将把\n${imageName.value}\n刷入分区 ${target}。\n\n刷错分区可能导致设备无法开机，请确认分区名正确。`,
    danger: true,
    risks: RISK_POINTS.flash,
    confirmText: '确认刷入',
  })
  if (!ok) return
  await run(`${target} 刷入完成`, () =>
    api.flashPartition(state.serial, target, imagePath.value)
  )
}

async function unsparse() {
  if (!imagePath.value) return
  const out = await run('', () => api.unsparseImage(imagePath.value, ''))
  if (out) {
    toast('ok', '已转换为 raw 镜像')
    await api.revealInFolder(out)
  }
}

/** 接收来自 Root 页的待刷镜像 */
async function consumePending() {
  const p = state.pendingImage
  if (!p) return
  state.pendingImage = ''
  imagePath.value = p
  bootInfo.value = null
  try {
    const info = await api.inspectBootImage(p)
    bootInfo.value = info
    partition.value = info.kind === 'init_boot' ? 'init_boot' : info.kind === 'vendor_boot' ? 'vendor_boot' : 'boot'
  } catch {
    bootInfo.value = null
  }
}

onMounted(consumePending)
watch(() => state.pendingImage, consumePending)
// 设备接入、或从系统重启进 fastboot（反之亦然）时，分区来源会变，得重读。
// immediate 覆盖「进页面时设备已经连着」的情况，不必再单独在 onMounted 里读一次。
watch(
  () => [state.serial, state.mode] as const,
  () => {
    if (state.serial) void loadDevicePartitions()
    else devicePartitions.value = []
  },
  { immediate: true },
)
</script>

<template>
  <div class="p-5">
    <div class="mb-4">
      <h1 class="text-[15px] font-medium text-slate-100">刷入镜像</h1>
      <p class="mt-1 text-[11.5px] text-muted">
        通过 fastboot 刷入单个分区。请确保设备已处于 Bootloader / fastbootd 模式。
      </p>
    </div>

    <RiskBanner
      title="刷写分区会直接改动设备固件"
      desc="写错分区或刷入不匹配的镜像，设备可能当场变砖且无法通过本软件恢复。刷前请确认已备份、已知晓救砖方式。"
      :points="RISK_POINTS.flash"
    />

    <div class="grid grid-cols-12 gap-4">
      <div class="col-span-8 flex flex-col gap-4">
        <!-- 选择镜像 -->
        <div class="panel">
          <div class="panel-head"><span class="panel-title">1 · 选择镜像文件</span></div>
          <div class="p-4">
            <div class="flex gap-2">
              <input class="input mono flex-1" :value="imagePath" readonly placeholder="尚未选择" />
              <button class="btn" @click="pickImage"><FolderOpen class="h-3.5 w-3.5" /> 浏览</button>
            </div>

            <div v-if="bootInfo" class="mt-3 rounded border border-ink-700 bg-ink-900 p-3">
              <div class="mb-2 flex items-center gap-2">
                <FileImage class="h-3.5 w-3.5 text-accent" />
                <span class="text-[11.5px] text-slate-200">Android {{ bootInfo.kind }} 镜像</span>
                <span class="tag tag-accent">v{{ bootInfo.headerVersion }}</span>
              </div>
              <div class="grid grid-cols-2 gap-x-4">
                <div class="kv"><span class="kv-k">文件大小</span><span class="kv-v">{{ fmtBytes(bootInfo.fileSize) }}</span></div>
                <div class="kv"><span class="kv-k">页大小</span><span class="kv-v mono">{{ bootInfo.pageSize }}</span></div>
                <div class="kv"><span class="kv-k">内核</span><span class="kv-v">{{ fmtBytes(bootInfo.kernelSize) }}</span></div>
                <div class="kv"><span class="kv-k">Ramdisk</span><span class="kv-v">{{ fmtBytes(bootInfo.ramdiskSize) }}</span></div>
                <div class="kv"><span class="kv-k">DTB</span><span class="kv-v">{{ fmtBytes(bootInfo.dtbSize) }}</span></div>
                <div class="kv"><span class="kv-k">OS 版本</span><span class="kv-v mono">{{ bootInfo.osVersion || '—' }}</span></div>
              </div>
              <div v-if="bootInfo.sections?.length" class="mt-2 flex flex-wrap gap-1">
                <span v-for="s in bootInfo.sections" :key="s" class="tag">{{ s }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- 目标分区 -->
        <div class="panel">
          <div class="panel-head">
            <span class="panel-title">2 · 目标分区</span>
            <div class="flex items-center gap-2">
              <span v-if="partitionSource" class="tag">
                {{
                  partitionSource === 'adb'
                    ? '来自 ADB'
                    : partitionSource === 'fastbootd'
                      ? '来自 fastbootd'
                      : '来自 bootloader'
                }}
              </span>
              <button class="btn btn-sm" @click="loadDevicePartitions">读取设备分区表</button>
            </div>
          </div>
          <div class="grid grid-cols-2 gap-3 p-4">
            <div>
              <label class="label">分区名</label>
              <select v-model="partition" class="input">
                <option v-for="p in partitionOptions" :key="p" :value="p">{{ p }}</option>
              </select>
            </div>
            <div>
              <label class="label">槽位（A/B 设备）</label>
              <div class="flex gap-2">
                <button
                  v-for="s in ['', 'a', 'b']"
                  :key="s"
                  class="btn flex-1"
                  :class="slot === s ? 'btn-primary' : ''"
                  :disabled="hasSlotSuffix"
                  @click="slot = s"
                >{{ s === '' ? '自动' : s.toUpperCase() }}</button>
              </div>
              <p v-if="hasSlotSuffix" class="mt-1 text-[11px] text-muted">
                分区名已含槽位，此项不生效
              </p>
            </div>
            <div class="col-span-2 rounded border border-ink-700 bg-ink-900 px-3 py-2">
              <span class="text-[11px] text-muted">最终刷入：</span>
              <span class="mono ml-1 text-[12px] text-accent">{{ finalPartition }}</span>
            </div>
          </div>
        </div>

        <div class="flex items-center gap-2">
          <button class="btn btn-primary flex-1" @click="flash">
            <Upload class="h-4 w-4" /> 刷入 {{ finalPartition }}
          </button>
          <button class="btn" @click="unsparse" :disabled="!imagePath">
            <Layers class="h-3.5 w-3.5" /> sparse → raw
          </button>
        </div>
      </div>

      <!-- 说明 -->
      <div class="col-span-4">
        <div class="panel">
          <div class="panel-head"><span class="panel-title">刷入须知</span></div>
          <div class="space-y-3 p-4 text-[11.5px] leading-relaxed text-muted">
            <div class="flex gap-2">
              <Info class="mt-0.5 h-3.5 w-3.5 shrink-0 text-accent" />
              <div>
                <div class="text-slate-300">boot / init_boot</div>
                Android 13 起部分机型 boot 分区拆分为 <span class="mono">init_boot</span>，
                Root 补丁应刷入 <span class="mono">init_boot</span>，刷错会导致无法开机。
              </div>
            </div>
            <div class="flex gap-2">
              <Info class="mt-0.5 h-3.5 w-3.5 shrink-0 text-accent" />
              <div>
                <div class="text-slate-300">动态分区</div>
                system / vendor / product 等位于 super 内，
                需要在 <span class="mono">fastbootd</span> 下刷入（本工具线刷会自动切换）。
              </div>
            </div>
            <div class="flex gap-2">
              <AlertTriangle class="mt-0.5 h-3.5 w-3.5 shrink-0 text-warn" />
              <div>
                <div class="text-warn">风险提示</div>
                vbmeta 刷入错误的镜像可能触发校验失败；务必使用与当前系统版本匹配的镜像。
              </div>
            </div>
            <div class="divider" />
            <div class="text-[11px]">
              当前模式：
              <span :class="state.mode === 'fastboot' ? 'text-ok' : 'text-warn'">
                {{ state.mode === 'fastboot' ? 'Fastboot（可刷入）' : 'ADB（无法刷入）' }}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
