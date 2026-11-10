<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  ShieldCheck,
  Download,
  FileImage,
  Smartphone,
  ArrowRight,
  Info,
  AlertTriangle,
  FolderOpen,
  Loader2,
  ExternalLink,
  CheckCircle2,
} from 'lucide-vue-next'
import { api } from '../api'
import { state, run, toast, needAdb } from '../store'
import { confirmDialog } from '../dialog'
import RiskBanner from '../components/RiskBanner.vue'
import { RISK_POINTS } from '../legal'
import type { RootStatus, Solution } from '../types'

const router = useRouter()

const status = ref<RootStatus | null>(null)
const solutions = ref<Solution[]>([])
const selected = ref('kernelsu')
const source = ref<'file' | 'device'>('file')
const bootImage = ref('')
const bootPartition = ref('boot')
const apkPath = ref('')
const patched = ref('')
const loading = ref(false)

const current = computed(() => solutions.value.find((s) => s.id === selected.value))

const familyLabel = (f: string) => (f === 'magisk' ? 'Magisk 系' : f === 'kernelsu' ? 'KernelSU 系' : 'APatch')

async function load() {
  loading.value = true
  try {
    solutions.value = await api.getRootSolutions()
    if (state.serial && state.mode === 'adb') {
      status.value = await api.getRootStatus(state.serial)
    }
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
  } finally {
    loading.value = false
  }
}

onMounted(load)

async function pickBoot() {
  const p = await api.pickFile('选择 boot / init_boot 镜像', '*.img;*.bin;*.*')
  if (p) {
    bootImage.value = p
    source.value = 'file'
  }
}

async function pickApk() {
  const p = await api.pickFile('选择已下载的安装包（可选）', '*.apk;*.*')
  if (p) apkPath.value = p
}

async function exportFromDevice() {
  if (!needAdb()) return
  if (!status.value?.rooted) {
    toast('warn', '从设备导出分区需要已有 root 权限')
    return
  }
  const out = await run('', () => api.extractBoot(state.serial, bootPartition.value, ''))
  if (out) {
    bootImage.value = out
    source.value = 'device'
    toast('ok', '已导出分区镜像')
  }
}

async function generate() {
  if (!needAdb()) return
  if (!bootImage.value) {
    toast('warn', '请选择或导出 boot 镜像')
    return
  }
  if (!selected.value) {
    toast('warn', '请选择 root 方案')
    return
  }
  const ok = await confirmDialog({
    title: '生成 Root 补丁镜像',
    message: `方案：${current.value?.name}\n镜像：${bootImage.value.split(/[\\/]/).pop()}\n\n流程会把补丁工具推送到手机 /data/local/tmp 执行，\n完成后自动拉回补丁镜像。过程中请勿断开 USB。`,
    danger: true,
    risks: RISK_POINTS.root,
    confirmText: '开始生成',
  })
  if (!ok) return

  const res = await run('', () =>
    api.installRoot(state.serial, selected.value, bootImage.value, apkPath.value)
  )
  if (res) {
    patched.value = res.patchedImage
    toast('ok', '补丁镜像已生成')
  }
}

function goFlash() {
  if (!patched.value) return
  state.pendingImage = patched.value
  router.push('/flash')
}
</script>

<template>
  <div class="p-5">
    <div class="mb-4">
      <h1 class="text-[15px] font-medium text-slate-100">Root 方案</h1>
      <p class="mt-1 text-[11.5px] text-muted">
        自动下载官方补丁工具，在手机上完成 boot 镜像打补丁，再通过 fastboot 刷入。
      </p>
    </div>

    <RiskBanner
      title="Root 会改变设备的安全边界"
      desc="获取 Root 需要解锁 Bootloader 并修改 boot 分区，多数厂商会因此终止保修。请自行确认设备归属与保修状态。"
      :points="RISK_POINTS.root"
    />

    <!-- 当前状态 -->
    <div class="panel mb-4 flex items-center gap-4 px-4 py-3">
      <div
        class="flex h-9 w-9 items-center justify-center rounded border"
        :class="status?.rooted ? 'border-ok/40 bg-ok/10 text-ok' : 'border-warn/30 bg-warn/10 text-warn'"
      >
        <ShieldCheck class="h-4 w-4" />
      </div>
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-2">
          <span class="text-[12.5px] text-slate-200">
            {{ status ? (status.rooted ? '设备已 Root' : '设备未 Root') : '选择设备后自动检测' }}
          </span>
          <span v-for="s in status?.solutions ?? []" :key="s" class="tag tag-ok">{{ s }}</span>
        </div>
        <div class="mt-0.5 text-[10.5px] text-muted">
          {{ status?.detail || '需要设备处于 ADB 模式' }}
        </div>
      </div>
      <button class="btn btn-sm" :disabled="loading" @click="load">
        <Loader2 v-if="loading" class="h-3.5 w-3.5 animate-spin" />
        重新检测
      </button>
    </div>

    <div class="grid grid-cols-12 gap-4">
      <!-- 方案选择 -->
      <div class="col-span-7">
        <div class="panel">
          <div class="panel-head"><span class="panel-title">选择 Root 方案</span></div>
          <div class="grid grid-cols-2 gap-2 p-3">
            <button
              v-for="s in solutions"
              :key="s.id"
              class="card-hover rounded border p-3 text-left"
              :class="selected === s.id ? 'border-accent/50 bg-accent/10' : 'border-ink-700 bg-ink-900'"
              @click="selected = s.id"
            >
              <div class="flex items-center justify-between">
                <span class="text-[12.5px] text-slate-200">{{ s.name }}</span>
                <CheckCircle2 v-if="selected === s.id" class="h-3.5 w-3.5 text-accent" />
              </div>
              <div class="mt-0.5 flex items-center gap-1.5">
                <span class="tag">{{ familyLabel(s.family) }}</span>
                <span class="text-[10px] text-slate-600">{{ s.vendor }}</span>
              </div>
              <div class="mt-1.5 text-[10.5px] leading-relaxed text-muted">{{ s.description }}</div>
              <div class="mt-1.5 flex items-center gap-1 text-[10px] text-accent/70" @click.stop="api.openUrl(s.website)">
                <ExternalLink class="h-2.5 w-2.5" /> {{ s.repo }}
              </div>
            </button>
          </div>
        </div>
      </div>

      <!-- 流程 -->
      <div class="col-span-5 flex flex-col gap-4">
        <div class="panel">
          <div class="panel-head"><span class="panel-title">1 · boot 镜像来源</span></div>
          <div class="p-4">
            <div class="flex gap-2">
              <button
                class="btn flex-1"
                :class="source === 'file' ? 'btn-primary' : ''"
                @click="source = 'file'"
              >本地文件</button>
              <button
                class="btn flex-1"
                :class="source === 'device' ? 'btn-primary' : ''"
                @click="source = 'device'"
              >从设备导出</button>
            </div>

            <div v-if="source === 'file'" class="mt-3">
              <div class="flex gap-2">
                <input class="input mono" :value="bootImage" readonly placeholder="选择 boot / init_boot 镜像" />
                <button class="btn btn-icon" @click="pickBoot"><FolderOpen class="h-3.5 w-3.5" /></button>
              </div>
            </div>

            <div v-else class="mt-3">
              <div class="flex gap-2">
                <select v-model="bootPartition" class="input flex-1">
                  <option value="boot">boot</option>
                  <option value="init_boot">init_boot</option>
                  <option value="vendor_boot">vendor_boot</option>
                </select>
                <button class="btn" @click="exportFromDevice">
                  <Smartphone class="h-3.5 w-3.5" /> 导出
                </button>
              </div>
              <div v-if="bootImage" class="mt-2 truncate mono text-[10.5px] text-ok">{{ bootImage }}</div>
            </div>
          </div>
        </div>

        <div class="panel">
          <div class="panel-head"><span class="panel-title">2 · 生成补丁</span></div>
          <div class="p-4">
            <label class="label">使用本地安装包（可选，留空则自动下载最新版）</label>
            <div class="flex gap-2">
              <input class="input mono" :value="apkPath" readonly placeholder="自动从 GitHub 下载" />
              <button class="btn btn-icon" @click="pickApk"><FileImage class="h-3.5 w-3.5" /></button>
            </div>

            <button class="btn btn-primary mt-3 w-full" :disabled="!bootImage || !selected" @click="generate">
              <Download class="h-4 w-4" />
              为 {{ current?.name || '…' }} 生成补丁镜像
            </button>

            <div v-if="patched" class="mt-3 rounded border border-ok/30 bg-ok/5 p-3">
              <div class="flex items-center gap-1.5 text-[11px] text-ok">
                <CheckCircle2 class="h-3.5 w-3.5" /> 补丁已生成
              </div>
              <div class="mt-1 truncate mono text-[10px] text-slate-400">{{ patched }}</div>
              <button class="btn btn-ok mt-2 w-full" @click="goFlash">
                去刷入 boot 分区 <ArrowRight class="h-3.5 w-3.5" />
              </button>
            </div>
          </div>
        </div>

        <div class="panel flex items-start gap-2 border-warn/20 bg-warn/5 px-3 py-3">
          <AlertTriangle class="mt-0.5 h-4 w-4 shrink-0 text-warn" />
          <div class="text-[10.5px] leading-relaxed text-muted">
            Root 的前提是 <span class="text-slate-300">bootloader 已解锁</span>，解锁需通过厂商官方渠道自行完成，本工具不提供解锁能力。
            部分 Magisk 分支需要对应版本的
            <span class="mono">boot_patch.sh</span>，若补丁失败请改用「本地安装包」指定。
          </div>
        </div>
      </div>
    </div>

    <!-- 步骤说明 -->
    <div class="panel mt-4 px-4 py-3">
      <div class="mb-2 text-[11px] text-slate-300">标准流程</div>
      <div class="flex items-center gap-2 text-[10.5px] text-muted">
        <span class="tag">1</span> 自行解锁 BL（本工具不提供）
        <ArrowRight class="h-3 w-3 text-slate-700" />
        <span class="tag">2</span> 获取与系统版本一致的 boot 镜像
        <ArrowRight class="h-3 w-3 text-slate-700" />
        <span class="tag">3</span> 生成补丁镜像
        <ArrowRight class="h-3 w-3 text-slate-700" />
        <span class="tag">4</span> fastboot 刷入 boot / init_boot
        <ArrowRight class="h-3 w-3 text-slate-700" />
        <span class="tag">5</span> 重启并安装对应的管理器 App
      </div>
    </div>
  </div>
</template>
