<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  AlertTriangle,
  Download,
  FolderOpen,
  Gauge,
  Link2,
  Loader2,
  Pause,
  ScanLine,
  Server,
} from 'lucide-vue-next'
import { api } from '../api'
import { state, run, toast, fmtBytes } from '../store'
import type { DownloadProbe, DownloadResult } from '../types'

const link = ref('')
const probe = ref<DownloadProbe | null>(null)
const probing = ref(false)
const threads = ref(4)
const dst = ref('')
const downloading = ref(false)
const result = ref<DownloadResult | null>(null)

const threadOptions = [1, 4, 8, 16]

const sizeText = computed(() => (probe.value?.size ? fmtBytes(probe.value.size) : '未知'))
const speedHint = computed(() => {
  if (!probe.value) return ''
  return probe.value.supportsRange
    ? `支持分段，将使用 ${threads.value} 个连接`
    : '服务端不支持 Range，只能单流下载，中断后需重下'
})

async function probeLink() {
  const url = link.value.trim()
  if (!url) return
  probing.value = true
  probe.value = null
  result.value = null
  try {
    const p = await api.probeDownload(url)
    probe.value = p
    if (p.size > 0) {
      toast('ok', `${p.fileName} · ${fmtBytes(p.size)}`)
    }
  } catch (e: any) {
    toast('err', e?.message ?? String(e), 6000)
  } finally {
    probing.value = false
  }
}

async function pickDst() {
  const p = await api.pickSaveFile('选择保存位置', probe.value?.fileName || 'ota.zip')
  if (p) dst.value = p
}

async function start() {
  const url = link.value.trim()
  if (!url) return
  downloading.value = true
  result.value = null
  try {
    const res = await run('', () => api.downloadFile(url, dst.value, threads.value))
    if (res) {
      result.value = res
      toast('ok', res.resumed ? '已续传完成' : '下载完成')
    }
  } catch (e: any) {
    const msg = e?.message ?? String(e)
    toast('err', msg, 7000)
  } finally {
    downloading.value = false
  }
}

function cancel() {
  api.cancelOperation()
}

function paste() {
  navigator.clipboard
    ?.readText()
    .then((t) => {
      if (t) link.value = t.trim()
    })
    .catch(() => toast('warn', '无法读取剪贴板，请手动粘贴'))
}

function openFolder(p: string) {
  api.revealInFolder(p)
}
</script>

<template>
  <div class="p-5">
    <div class="mb-4">
      <h1 class="text-[15px] font-medium text-slate-100">固件下载</h1>
      <p class="mt-1 text-[11.5px] text-muted">
        粘贴 OTA 或线刷包直链，多线程下载并支持断点续传。中断后再次点击会从已下载的位置继续。
      </p>
    </div>

    <div class="panel">
      <div class="panel-head">
        <span class="panel-title">下载地址</span>
        <button class="btn btn-sm" @click="paste"><Link2 class="h-3.5 w-3.5" />粘贴</button>
      </div>
      <div class="px-4 py-3">
        <div class="flex items-center gap-2">
          <input
            class="input mono flex-1"
            v-model="link"
            placeholder="https://…/full_ota.zip"
            @keyup.enter="probeLink"
          />
          <button class="btn" :disabled="!link.trim() || probing" @click="probeLink">
            <Loader2 v-if="probing" class="h-3.5 w-3.5 animate-spin" />
            <ScanLine v-else class="h-3.5 w-3.5" />
            探测
          </button>
        </div>
        <div class="mt-2 text-[11px] text-muted">
          复用「刷机包提取」里设置的 User-Agent 与 Referer，需要鉴权的链接先在那里填好。
        </div>
      </div>
    </div>

    <div v-if="probe" class="panel mt-3">
      <div class="panel-head">
        <span class="panel-title">远端信息</span>
        <span class="tag" :class="probe.supportsRange ? 'tag-ok' : 'tag-warn'">
          {{ probe.supportsRange ? '支持分段' : '不支持分段' }}
        </span>
      </div>
      <div class="grid grid-cols-3 gap-x-6 px-4 py-3">
        <div class="kv"><span class="kv-k">文件名</span><span class="kv-v mono">{{ probe.fileName }}</span></div>
        <div class="kv"><span class="kv-k">大小</span><span class="kv-v mono">{{ sizeText }}</span></div>
        <div class="kv">
          <span class="kv-k">Accept-Ranges</span>
          <span class="kv-v mono">{{ probe.acceptRanges || '—' }}</span>
        </div>
      </div>
    </div>

    <div class="panel mt-3">
      <div class="panel-head">
        <span class="panel-title">下载设置</span>
        <button class="btn btn-sm" @click="pickDst"><FolderOpen class="h-3.5 w-3.5" />保存到</button>
      </div>
      <div class="px-4 py-3">
        <div class="flex items-center gap-2">
          <input class="input mono flex-1" v-model="dst" placeholder="留空则保存到输出目录，文件名取自远端" />
        </div>

        <div class="mt-3 flex items-center gap-3">
          <span class="text-[11px] text-muted">并发连接</span>
          <div class="flex gap-1.5">
            <button
              v-for="n in threadOptions"
              :key="n"
              class="btn btn-sm"
              :class="threads === n ? 'btn-primary' : ''"
              @click="threads = n"
            >
              {{ n }}
            </button>
          </div>
          <span class="ml-2 text-[11px] text-muted">{{ speedHint }}</span>
        </div>

        <div class="mt-3 flex items-center gap-2">
          <button class="btn btn-primary" :disabled="!link.trim() || downloading" @click="start">
            <Loader2 v-if="downloading" class="h-3.5 w-3.5 animate-spin" />
            <Download v-else class="h-3.5 w-3.5" />
            开始下载
          </button>
          <button v-if="downloading" class="btn btn-danger" @click="cancel">
            <Pause class="h-3.5 w-3.5" />中止（保留进度）
          </button>
          <span v-if="state.busy && !downloading" class="text-[11px] text-muted">有其他操作正在进行</span>
        </div>
      </div>
    </div>

    <div v-if="result" class="panel mt-3">
      <div class="panel-head">
        <span class="panel-title">下载完成</span>
        <button class="btn btn-sm" @click="openFolder(result.path)">打开目录</button>
      </div>
      <div class="grid grid-cols-4 gap-x-6 px-4 py-3">
        <div class="kv"><span class="kv-k">大小</span><span class="kv-v mono">{{ fmtBytes(result.size) }}</span></div>
        <div class="kv"><span class="kv-k">线程</span><span class="kv-v mono">{{ result.threads }}</span></div>
        <div class="kv"><span class="kv-k">分段</span><span class="kv-v mono">{{ result.supportsRange ? '是' : '否' }}</span></div>
        <div class="kv"><span class="kv-k">续传</span><span class="kv-v mono">{{ result.resumed ? '是' : '否' }}</span></div>
      </div>
      <div class="px-4 pb-3">
        <div class="mono truncate text-[11px] text-muted">{{ result.path }}</div>
      </div>
    </div>

    <div class="panel mt-3 flex items-start gap-2 border-warn/20 bg-warn/5 px-4 py-3">
      <AlertTriangle class="mt-0.5 h-4 w-4 shrink-0 text-warn" />
      <div class="text-[11.5px] leading-relaxed text-muted">
        <span class="text-warn">提示</span>
        · 断点进度保存在同目录的 <span class="mono">.dpart</span> 文件里，删掉它就会从零开始；
        厂商 OTA 直链通常几小时就过期，出现 403 请重新获取链接。
      </div>
    </div>

    <div class="mt-3 flex items-center gap-2 text-[11px] text-muted">
      <Server class="h-3.5 w-3.5" />
      下载完成后可直接把整包拖进「刷机包提取」按分区取镜像，无需先解压。
      <Gauge class="ml-2 h-3.5 w-3.5" />
      进度在底部日志栏上方实时显示。
    </div>
  </div>
</template>
