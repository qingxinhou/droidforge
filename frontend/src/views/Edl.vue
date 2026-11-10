<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import {
  AlertTriangle,
  Cable,
  Download,
  Eraser,
  FileSearch,
  Loader2,
  Plug,
  PlugZap,
  Power,
  RefreshCw,
  Upload,
} from 'lucide-vue-next'
import { api } from '../api'
import { confirmDialog } from '../dialog'
import { RISK_POINTS } from '../legal'
import { run, toast, fmtBytes } from '../store'
import RiskBanner from '../components/RiskBanner.vue'
import type { EdlDevice, EdlGPT, EdlLoaderInfo, EdlPartition, EdlStatus } from '../types'

const devices = ref<EdlDevice[]>([])
const devicePath = ref('')
const loaderPath = ref('')
const loaderInfo = ref<EdlLoaderInfo | null>(null)
const scanning = ref(false)
const connecting = ref(false)
const busy = ref(false)

const status = ref<EdlStatus | null>(null)
const gpt = ref<EdlGPT | null>(null)
const lun = ref(0)
const selectedName = ref('')
const imgPath = ref('')

const parts = computed(() => gpt.value?.partitions ?? [])
const selectedPart = computed<EdlPartition | null>(
  () => parts.value.find((p) => p.name === selectedName.value) ?? null
)
const connected = computed(() => !!status.value?.connected)

function partBytes(p: EdlPartition) {
  return p.sectors * (gpt.value?.sectorSize ?? 4096)
}

async function scanDevices() {
  scanning.value = true
  try {
    devices.value = await api.edlScanDevices()
    if (devices.value.length === 0) {
      devicePath.value = ''
    } else if (!devices.value.some((d) => d.path === devicePath.value)) {
      devicePath.value = devices.value[0].path
    }
  } catch (e: any) {
    toast('err', e?.message ?? String(e), 8000)
  } finally {
    scanning.value = false
  }
}

async function pickLoader() {
  const p = await api.pickFile('选择 loader（prog_firehose_*.elf）', '*.elf;*.mbn;*.bin;*.*')
  if (!p) return
  loaderPath.value = p
  try {
    loaderInfo.value = await api.edlInspectLoader(p)
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
  }
}

async function connect() {
  if (!devicePath.value || !loaderPath.value) {
    toast('warn', '先选设备和 loader')
    return
  }
  const ok = await confirmDialog({
    title: '开始 EDL 会话',
    message:
      `将要向 ${devicePath.value.split('#')[1] ?? '设备'} 发送 loader 并建立 Firehose 会话。\n\n` +
      'loader 必须与机型严格匹配。发送错误的 loader 可能让设备停在 9008 无法自行恢复。',
    danger: true,
    risks: RISK_POINTS.edl,
    confirmText: '发送 loader',
  })
  if (!ok) return

  connecting.value = true
  try {
    const g = await api.edlConnect(devicePath.value, loaderPath.value)
    if (g) {
      gpt.value = g
      lun.value = g.lun
    }
    await refreshStatus()
    toast('ok', 'EDL 会话已建立')
  } catch (e: any) {
    toast('err', e?.message ?? String(e), 10000)
  } finally {
    connecting.value = false
  }
}

async function refreshStatus() {
  try {
    status.value = await api.edlState()
  } catch {
    status.value = null
  }
}

async function readGPT() {
  busy.value = true
  try {
    const g = await api.edlGPT(lun.value)
    gpt.value = g
    selectedName.value = ''
    toast('ok', `LUN ${g.lun}：${g.partitions.length} 个分区`)
  } catch (e: any) {
    toast('err', e?.message ?? String(e), 8000)
  } finally {
    busy.value = false
  }
}

async function readPart() {
  const p = selectedPart.value
  if (!p) return
  busy.value = true
  try {
    const out = await run('读取分区', () => api.edlReadPartition(p.name, '', lun.value))
    if (out) toast('ok', `已保存到 ${out}`, 5000)
  } catch (e: any) {
    toast('err', e?.message ?? String(e), 8000)
  } finally {
    busy.value = false
  }
}

async function pickImage() {
  const p = await api.pickFile('选择要写入的镜像', '*.img;*.bin;*.*')
  if (p) imgPath.value = p
}

async function writePart() {
  const p = selectedPart.value
  if (!p || !imgPath.value) {
    toast('warn', '先选分区和镜像')
    return
  }
  const ok = await confirmDialog({
    title: `写入 ${p.name}`,
    message:
      `即将把\n${imgPath.value}\n\n写入分区 ${p.name}（LUN ${lun.value}，起始扇区 ${p.firstLba}）。\n` +
      `分区容量 ${fmtBytes(partBytes(p))}，本次写入不可撤销。`,
    danger: true,
    risks: RISK_POINTS.edl,
    confirmText: '开始写入',
  })
  if (!ok) return

  busy.value = true
  try {
    await run('写入分区', () => api.edlWritePartition(p.name, imgPath.value, lun.value))
    toast('ok', `${p.name} 写入完成`, 5000)
  } catch (e: any) {
    toast('err', e?.message ?? String(e), 10000)
  } finally {
    busy.value = false
  }
}

async function erasePart() {
  const p = selectedPart.value
  if (!p) return
  const ok = await confirmDialog({
    title: `擦除 ${p.name}`,
    message: `即将擦除分区 ${p.name}（LUN ${lun.value}，${p.sectors} 个扇区）。\n擦除后内容无法通过任何工具恢复。`,
    danger: true,
    risks: RISK_POINTS.erase,
    confirmText: '擦除',
  })
  if (!ok) return

  busy.value = true
  try {
    await run('擦除分区', () => api.edlErasePartition(p.name, lun.value))
    toast('ok', `${p.name} 已擦除`)
  } catch (e: any) {
    toast('err', e?.message ?? String(e), 8000)
  } finally {
    busy.value = false
  }
}

async function reboot() {
  const ok = await confirmDialog({
    title: '重启设备',
    message: '发送 reset 让设备退出 EDL 并重新启动。会话会随之中断。',
    danger: false,
    confirmText: '重启',
  })
  if (!ok) return
  try {
    await api.edlReboot()
    gpt.value = null
    status.value = null
    toast('ok', '已发送重启')
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
  }
}

async function disconnect() {
  await api.edlDisconnect()
  gpt.value = null
  status.value = null
  toast('info', '会话已断开')
}

let timer: number | undefined
onMounted(() => {
  scanDevices()
  refreshStatus()
  timer = window.setInterval(refreshStatus, 4000)
})
onBeforeUnmount(() => {
  if (timer) window.clearInterval(timer)
})
</script>

<template>
  <div>
    <div class="mb-4">
      <h1 class="text-[17px] font-medium text-slate-100">EDL 9008</h1>
      <p class="mt-1 text-[11.5px] text-muted">
        通过 Sahara + Firehose 协议直接读写闪存分区，绕过 fastboot 与系统校验
      </p>
    </div>

    <RiskBanner
      title="EDL 是最后一级手段，也是风险最高的一级"
      desc="它直接操作闪存控制器，没有 fastboot 那层保护。分区写错、loader 不匹配或中途断开都可能让设备变成硬砖，且这类损坏通常只能靠同型号的官方工具修复。"
      :points="RISK_POINTS.edl"
    />

    <div class="panel mt-3">
      <div class="panel-head">
        <span class="panel-title">连接</span>
        <div class="flex gap-1.5">
          <button class="btn btn-sm" :disabled="scanning" @click="scanDevices">
            <RefreshCw class="h-3.5 w-3.5" :class="scanning ? 'animate-spin' : ''" />扫描设备
          </button>
          <button class="btn btn-sm" :disabled="connecting" @click="connect">
            <Loader2 v-if="connecting" class="h-3.5 w-3.5 animate-spin" />
            <Plug v-else class="h-3.5 w-3.5" />
            建立会话
          </button>
        </div>
      </div>

      <div class="grid grid-cols-2 gap-4 px-4 py-3">
        <div>
          <div class="mb-1 text-[11px] text-muted">9008 设备</div>
          <select v-model="devicePath" class="input w-full">
            <option value="">— 未找到 —</option>
            <option v-for="d in devices" :key="d.path" :value="d.path">{{ d.name }}</option>
          </select>
          <div class="mt-1 text-[10.5px] text-muted">
            {{ devices.length ? `找到 ${devices.length} 个` : '设备需进入 EDL 模式，且驱动换成 WinUSB' }}
          </div>
        </div>
        <div>
          <div class="mb-1 text-[11px] text-muted">loader</div>
          <div class="flex gap-2">
            <input class="input mono flex-1" :value="loaderPath" readonly placeholder="prog_firehose_*.elf" />
            <button class="btn btn-sm" @click="pickLoader"><FileSearch class="h-3.5 w-3.5" />选择</button>
          </div>
          <div v-if="loaderInfo" class="mt-1 text-[10.5px]" :class="loaderInfo.valid ? 'text-muted' : 'text-warn'">
            {{ fmtBytes(loaderInfo.size) }} · 按 {{ loaderInfo.memory }} 处理
            <span v-if="loaderInfo.note"> · {{ loaderInfo.note }}</span>
          </div>
        </div>
      </div>

      <div class="flex items-start gap-1.5 border-t border-ink-700 px-4 py-2.5 text-[11px] text-warn">
        <AlertTriangle class="mt-0.5 h-3 w-3 shrink-0" />
        <span>
          Windows 上 9008 默认挂的是 QDLoader 串口驱动，本工具需要 <span class="mono">WinUSB</span>。
          用 Zadig 选中设备、目标驱动选 WinUSB 安装后重新扫描即可。
        </span>
      </div>
    </div>

    <div v-if="status?.connected" class="panel mt-3">
      <div class="panel-head">
        <span class="panel-title">会话</span>
        <div class="flex items-center gap-1.5">
          <button class="btn btn-sm" @click="reboot"><Power class="h-3.5 w-3.5" />重启</button>
          <button class="btn btn-sm" @click="disconnect"><PlugZap class="h-3.5 w-3.5" />断开</button>
        </div>
      </div>
      <div class="grid grid-cols-4 gap-x-6 px-4 py-3">
        <div class="kv"><span class="kv-k">设备</span><span class="kv-v mono truncate">{{ status.device }}</span></div>
        <div class="kv"><span class="kv-k">loader</span><span class="kv-v mono truncate">{{ status.loader }}</span></div>
        <div class="kv"><span class="kv-k">扇区大小</span><span class="kv-v mono">{{ status.sectorSize || '—' }}</span></div>
        <div class="kv"><span class="kv-k">分区数</span><span class="kv-v mono">{{ status.partitions }}</span></div>
      </div>
    </div>

    <div v-if="status?.connected" class="panel mt-3">
      <div class="panel-head">
        <div class="flex items-center gap-2">
          <span class="panel-title">分区表</span>
          <select v-model.number="lun" class="input h-7 w-24 text-[11px]">
            <option v-for="n in 6" :key="n - 1" :value="n - 1">LUN {{ n - 1 }}</option>
          </select>
          <button class="btn btn-sm" :disabled="busy" @click="readGPT">
            <RefreshCw class="h-3.5 w-3.5" :class="busy ? 'animate-spin' : ''" />读取
          </button>
        </div>
        <span v-if="gpt" class="text-[11px] text-muted">
          {{ gpt.partitions.length }} 个分区 · 盘 {{ gpt.diskGuid.slice(0, 8) }}…
        </span>
      </div>

      <div v-if="!gpt" class="px-4 py-6 text-center text-[11.5px] text-muted">
        选一个 LUN 点「读取」。多数机型的系统分区在 LUN 0，boot 类分区在 LUN 1–4。
      </div>

      <template v-else>
        <div v-if="gpt.note" class="border-b border-ink-700 px-4 py-2 text-[11px] text-warn">{{ gpt.note }}</div>
        <div class="max-h-[38vh] overflow-y-auto">
          <table class="w-full">
            <thead class="sticky top-0 bg-ink-850">
              <tr class="text-[10.5px] text-muted">
                <th class="w-8 px-3 py-1.5"></th>
                <th class="px-2 py-1.5 text-left font-normal">分区</th>
                <th class="px-2 py-1.5 text-right font-normal">起始扇区</th>
                <th class="px-2 py-1.5 text-right font-normal">扇区数</th>
                <th class="px-3 py-1.5 text-right font-normal">大小</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-ink-700/50">
              <tr
                v-for="p in parts"
                :key="p.index"
                class="h-8 cursor-pointer text-[11.5px] hover:bg-ink-800/60"
                :class="selectedName === p.name ? 'bg-accent/10' : ''"
                @click="selectedName = p.name"
              >
                <td class="px-3">
                  <Cable class="h-3.5 w-3.5" :class="selectedName === p.name ? 'text-accent' : 'text-slate-700'" />
                </td>
                <td class="px-2 mono text-slate-200">{{ p.name || '（无名）' }}</td>
                <td class="px-2 text-right mono text-muted">{{ p.firstLba }}</td>
                <td class="px-2 text-right mono text-muted">{{ p.sectors }}</td>
                <td class="px-3 text-right mono text-slate-300">{{ fmtBytes(partBytes(p)) }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="border-t border-ink-700 px-4 py-3">
          <div class="mb-2 text-[11px] text-muted">
            已选：<span class="mono text-slate-300">{{ selectedName || '未选择' }}</span>
            <span v-if="selectedPart"> · {{ fmtBytes(partBytes(selectedPart)) }}</span>
          </div>
          <div class="flex items-center gap-2">
            <input class="input mono flex-1" :value="imgPath" readonly placeholder="写入用的本地镜像" />
            <button class="btn btn-sm" @click="pickImage"><FileSearch class="h-3.5 w-3.5" />选择镜像</button>
            <button class="btn btn-sm" :disabled="!selectedPart || busy" @click="readPart">
              <Download class="h-3.5 w-3.5" />读取
            </button>
            <button
              class="btn btn-danger btn-sm"
              :disabled="!selectedPart || !imgPath || busy"
              @click="writePart"
            >
              <Upload class="h-3.5 w-3.5" />写入
            </button>
            <button class="btn btn-sm" :disabled="!selectedPart || busy" @click="erasePart">
              <Eraser class="h-3.5 w-3.5" />擦除
            </button>
          </div>
        </div>
      </template>
    </div>

    <div class="mt-3 px-1 text-[10.5px] leading-relaxed text-muted">
      协议按公开的 Sahara / Firehose 规范实现。本工具不提供 loader、不内置固件、不绕过厂商授权：
      部分新机型的 EDL 需要签名的 loader 与授权账号，未经授权的写入会被设备拒绝。
    </div>
  </div>
</template>
