<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  Smartphone,
  Cpu,
  HardDrive,
  Battery,
  ShieldCheck,
  Lock,
  Unlock,
  Layers,
  Terminal,
  Power,
  Camera,
  RefreshCw,
  ChevronRight,
  Info,
  Monitor,
  BatteryCharging,
  Search,
} from 'lucide-vue-next'
import { api } from '../api'
import { state, fmtBytes, toast, needAdb, run, maskSerial } from '../store'
import { NAV_GROUPS } from '../nav'
import { openPalette } from '../palette'
import type { DeviceInfo } from '../types'

const router = useRouter()
const info = ref<DeviceInfo | null>(null)
const loading = ref(false)

const hasDevice = computed(() => !!state.serial)
const isAdb = computed(() => state.mode === 'adb')

async function load() {
  if (!state.serial || state.mode !== 'adb') {
    info.value = null
    return
  }
  loading.value = true
  try {
    info.value = await api.getDeviceInfo(state.serial)
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
  } finally {
    loading.value = false
  }
}

onMounted(load)
watch(() => [state.serial, state.mode], load)

const storagePercent = computed(() => {
  const i = info.value
  if (!i || !i.storageTotalKb) return 0
  return Math.round(((i.storageTotalKb - i.storageFreeKb) / i.storageTotalKb) * 100)
})

const memGB = computed(() => {
  const i = info.value
  return i ? i.memoryTotalKb / 1024 / 1024 : 0
})

async function quickReboot(target: string) {
  if (!needAdb()) return
  await run('重启指令已发送', () => api.adbReboot(state.serial, target))
}

async function shot() {
  if (!needAdb()) return
  const p = await run('', () => api.screenshot(state.serial))
  if (p) {
    toast('ok', '截图已保存')
    await api.revealInFolder(p)
  }
}

const rootText = computed(() => {
  const r = info.value?.root
  if (!r) return '—'
  if (!r.rooted) return '未 Root'
  const map: Record<string, string> = { magisk: 'Magisk', kernelsu: 'KernelSU', apatch: 'APatch', supersu: 'SuperSU' }
  return (r.solutions ?? []).map((s) => map[s] ?? s).join(' / ') || '已 Root'
})

const blText = computed(() => {
  const i = info.value
  if (!i) return '—'
  if (i.flashLocked === '1') return '已锁定'
  if (i.flashLocked === '0') return '已解锁'
  return i.verifiedBoot || '未知'
})
</script>

<template>
  <div class="p-5">
    <!-- 无设备 -->
    <div v-if="!hasDevice" class="panel grid-bg flex flex-col items-center justify-center py-24">
      <Smartphone class="mb-4 h-10 w-10 text-slate-600" />
      <div class="text-[14px] text-slate-300">未检测到设备</div>
      <div class="mt-2 max-w-md text-center text-[12px] leading-relaxed text-muted">
        请用 USB 连接手机，在「开发者选项」中开启 <span class="text-slate-300">USB 调试</span>，
        并在手机上允许本机的调试授权。
      </div>
      <button class="btn btn-primary mt-5" @click="router.push('/settings')">
        前往设置检查工具链
        <ChevronRight class="h-3.5 w-3.5" />
      </button>
    </div>

    <div v-else class="grid grid-cols-12 gap-4">
      <!-- 设备主卡 -->
      <div class="panel col-span-8 overflow-hidden">
        <div class="relative grid-bg border-b border-ink-700 px-5 py-4">
          <div class="flex items-start justify-between">
            <div class="flex items-center gap-3">
              <div class="flex h-11 w-11 items-center justify-center rounded border border-accent/30 bg-accent/10">
                <Smartphone class="h-5 w-5 text-accent" />
              </div>
              <div>
                <div class="flex items-center gap-2">
                  <span class="text-[16px] font-medium text-slate-100">
                    {{ info?.model || (loading ? '读取中…' : '未知机型') }}
                  </span>
                  <span v-if="info?.productName" class="tag">{{ info.productName }}</span>
                </div>
                <div class="mt-1 flex items-center gap-2 text-[11px] text-muted">
                  <span>{{ info?.brand }}</span>
                  <span class="text-slate-700">/</span>
                  <span class="mono">{{ info?.device }}</span>
                  <span class="text-slate-700">/</span>
                  <span class="mono">{{ maskSerial(state.serial) }}</span>
                </div>
              </div>
            </div>
            <div class="flex items-center gap-2">
              <span class="tag" :class="info?.root.rooted ? 'tag-ok' : 'tag-warn'">
                <ShieldCheck class="h-3 w-3" />
                {{ rootText }}
              </span>
              <span class="tag" :class="blText === '已解锁' ? 'tag-accent' : 'tag'">
                <Unlock v-if="blText === '已解锁'" class="h-3 w-3" />
                <Lock v-else class="h-3 w-3" />
                BL {{ blText }}
              </span>
            </div>
          </div>
        </div>

        <!-- 关键指标 -->
        <div class="grid grid-cols-4 divide-x divide-ink-700 border-b border-ink-700">
          <div class="px-4 py-3">
            <div class="flex items-center gap-1.5 text-[10.5px] text-muted">
              <Cpu class="h-3 w-3" /> 平台
            </div>
            <div class="mt-1 truncate text-[12.5px] text-slate-200">{{ info?.soc || '—' }}</div>
          </div>
          <div class="px-4 py-3">
            <div class="flex items-center gap-1.5 text-[10.5px] text-muted">Android</div>
            <div class="mt-1 text-[12.5px] text-slate-200">
              {{ info?.androidVersion || '—' }}
              <span class="ml-1 text-[10px] text-muted">API {{ info?.sdk || '—' }}</span>
            </div>
          </div>
          <div class="px-4 py-3">
            <div class="flex items-center gap-1.5 text-[10.5px] text-muted">内存</div>
            <div class="mt-1 text-[12.5px] text-slate-200">{{ memGB ? memGB.toFixed(0) + ' GB' : '—' }}</div>
          </div>
          <div class="px-4 py-3">
            <div class="flex items-center gap-1.5 text-[10.5px] text-muted">
              <Battery class="h-3 w-3" /> 电量
            </div>
            <div class="mt-1 text-[12.5px]" :class="(info?.batteryLevel ?? 100) < 20 ? 'text-danger' : 'text-slate-200'">
              {{ info ? info.batteryLevel + '%' : '—' }}
            </div>
          </div>
        </div>

        <!-- 存储条 -->
        <div class="px-5 py-4">
          <div class="mb-1.5 flex items-center justify-between text-[11px]">
            <span class="flex items-center gap-1.5 text-muted"><HardDrive class="h-3 w-3" /> 存储空间</span>
            <span class="mono text-slate-400">
              {{ info ? fmtBytes((info.storageTotalKb - info.storageFreeKb) * 1024) + ' / ' + fmtBytes(info.storageTotalKb * 1024) : '—' }}
            </span>
          </div>
          <div class="h-1.5 overflow-hidden rounded-sm bg-ink-700">
            <div class="h-full bg-accent transition-all" :style="{ width: storagePercent + '%' }" />
          </div>
          <div class="mt-4 grid grid-cols-2 gap-3">
            <div class="kv">
              <span class="kv-k">内核</span>
              <span class="kv-v mono">{{ info?.kernel || '—' }}</span>
            </div>
            <div class="kv">
              <span class="kv-k">架构</span>
              <span class="kv-v mono">{{ info?.arch || '—' }}</span>
            </div>
            <div class="kv">
              <span class="kv-k">槽位</span>
              <span class="kv-v mono">
                {{ info?.isAb ? (info?.slot ? info.slot.toUpperCase() : 'A/B') : '单分区' }}
              </span>
            </div>
            <div class="kv">
              <span class="kv-k">动态分区</span>
              <span class="kv-v">{{ info?.hasDynamicPartitions ? '支持' : '不支持' }}</span>
            </div>
            <div class="kv">
              <span class="kv-k">SELinux</span>
              <span class="kv-v mono">{{ info?.selinux || '—' }}</span>
            </div>
            <div class="kv">
              <span class="kv-k">运行时长</span>
              <span class="kv-v">{{ info?.uptime || '—' }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- 右侧快捷操作 -->
      <div class="col-span-4 flex flex-col gap-4">
        <div class="panel">
          <div class="panel-head">
            <span class="panel-title">快捷操作</span>
            <RefreshCw class="h-3.5 w-3.5 cursor-pointer text-muted hover:text-slate-300" @click="load" />
          </div>
          <div class="grid grid-cols-2 gap-2 p-3">
            <button class="btn card-hover h-16 flex-col gap-1" @click="quickReboot('bootloader')">
              <Power class="h-4 w-4 text-warn" />
              <span>进 Bootloader</span>
            </button>
            <button class="btn card-hover h-16 flex-col gap-1" @click="quickReboot('recovery')">
              <Layers class="h-4 w-4 text-accent" />
              <span>进 Recovery</span>
            </button>
            <button class="btn card-hover h-16 flex-col gap-1" @click="router.push('/terminal')">
              <Terminal class="h-4 w-4 text-slate-300" />
              <span>ADB 终端</span>
            </button>
            <button class="btn card-hover h-16 flex-col gap-1" @click="shot">
              <Camera class="h-4 w-4 text-ok" />
              <span>截屏</span>
            </button>
            <button class="btn card-hover h-16 flex-col gap-1" @click="router.push('/display')">
              <Monitor class="h-4 w-4 text-accent" />
              <span>显示控制</span>
            </button>
            <button class="btn card-hover h-16 flex-col gap-1" @click="router.push('/battery')">
              <BatteryCharging class="h-4 w-4 text-warn" />
              <span>电池管理</span>
            </button>
          </div>
        </div>

        <div class="panel flex-1">
          <div class="panel-head">
            <span class="panel-title">构建信息</span>
            <Info class="h-3.5 w-3.5 text-muted" />
          </div>
          <div class="px-4 py-3">
            <div class="kv">
              <span class="kv-k">版本号</span>
              <span class="kv-v mono text-[11px]">{{ info?.buildId || '—' }}</span>
            </div>
            <div class="kv">
              <span class="kv-k">构建类型</span>
              <span class="kv-v mono">{{ info?.buildType || '—' }}</span>
            </div>
            <div class="kv">
              <span class="kv-k">补丁级别</span>
              <span class="kv-v mono">{{ info?.securityPatch || '—' }}</span>
            </div>
            <div class="kv">
              <span class="kv-k">调试模式</span>
              <span class="kv-v">{{ info?.debuggable ? '是 (userdebug)' : '否 (user)' }}</span>
            </div>
            <div class="kv">
              <span class="kv-k">ABI</span>
              <span class="kv-v mono text-[11px]">{{ info?.abi || '—' }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- 指纹 -->
      <div v-if="info?.buildFingerprint" class="panel col-span-12 px-4 py-2.5">
        <div class="flex items-center gap-2">
          <span class="shrink-0 text-[10.5px] text-muted">Fingerprint</span>
          <span class="mono min-w-0 flex-1 truncate text-[10.5px] text-slate-500">{{ info.buildFingerprint }}</span>
          <button class="btn btn-sm" @click="api.copyText(info!.buildFingerprint)">复制</button>
        </div>
      </div>
    </div>

    <!-- 功能总览：与侧栏共用 nav.ts 的分组，越往右越危险 -->
    <div class="panel mt-4">
      <div class="panel-head">
        <div class="flex items-baseline gap-2">
          <span class="panel-title">功能总览</span>
          <span class="text-[10px] text-muted">按操作性质分组，从左到右风险递增</span>
        </div>
        <button class="btn btn-sm h-6" @click="openPalette">
          <Search class="h-3 w-3" />
          搜索
          <span class="mono ml-0.5 text-[9.5px] text-muted">Ctrl K</span>
        </button>
      </div>
      <div class="grid grid-cols-4 divide-x divide-ink-700">
        <div v-for="g in NAV_GROUPS" :key="g.id" class="px-3 py-3">
          <div class="mb-2 flex items-center gap-2" :title="g.hint">
            <span class="shrink-0 text-[10.5px] font-medium tracking-[0.12em] text-slate-400">
              {{ g.label }}
            </span>
            <span class="h-px flex-1 bg-ink-700" />
          </div>
          <button
            v-for="item in g.items"
            :key="item.path"
            class="group flex w-full items-start gap-2 rounded px-1.5 py-1.5 text-left transition-colors hover:bg-ink-800"
            :title="item.desc"
            @click="router.push(item.path)"
          >
            <component
              :is="item.icon"
              class="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted transition-colors group-hover:text-accent"
            />
            <div class="min-w-0">
              <div class="text-[12px] text-slate-300 transition-colors group-hover:text-slate-100">
                {{ item.label }}
              </div>
              <div class="truncate text-[10.5px] text-muted">{{ item.desc }}</div>
            </div>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
