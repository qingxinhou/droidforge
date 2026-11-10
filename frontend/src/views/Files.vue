<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  Upload, FileUp, FolderDown, Package, Layers, HardDriveDownload, FolderOpen,
} from 'lucide-vue-next'
import { api } from '../api'
import { state, toast, needAdb, run } from '../store'

const remotePath = ref('/sdcard/Download')
const pullRemote = ref('/sdcard/DCIM/Camera')
const pullLocal = ref(state.env?.config.pushDir ?? '')
const pkgInput = ref('')
const splitMode = ref(false)
const reinstall = ref(state.env?.config.installReinstall ?? true)
const lastOut = ref<string[]>([])

function ensure() {
  if (!needAdb()) return false
  if (!splitMode.value) return true
  return true
}

async function installSingle() {
  if (!ensure()) return
  const p = await api.pickFile('选择 APK', '*.apk')
  if (!p) return
  await run('安装完成', () => api.installApks(state.serial, [p], reinstall.value, false))
}

async function installMany() {
  if (!ensure()) return
  const files = await api.pickFiles('选择 APK（base + split 或批量）', '*.apk')
  if (!files || files.length === 0) return
  await run(
    splitMode.value ? `已安装 ${files.length} 个分片` : `已处理 ${files.length} 个 APK`,
    () => api.installApks(state.serial, files, reinstall.value, splitMode.value),
  )
}

async function pushFile() {
  if (!ensure()) return
  const p = await api.pickFile('选择要推送的文件', '*.*')
  if (!p) return
  const dest = remotePath.value.trim() || '/sdcard/Download'
  await run('推送完成', () => api.adbPush(state.serial, p, dest))
}

async function pullFile() {
  if (!ensure()) return
  const remote = pullRemote.value.trim()
  if (!remote) {
    toast('warn', '请输入设备路径')
    return
  }
  let local = pullLocal.value.trim()
  if (!local) {
    local = await api.pickDirectory('选择保存目录')
    if (!local) return
    pullLocal.value = local
  }
  await run('拉取完成', () => api.adbPull(state.serial, remote, local))
}

async function extractApps() {
  if (!ensure()) return
  const pkgs = pkgInput.value
    .split(/[\s,;]+/)
    .map((s) => s.trim())
    .filter(Boolean)
  if (pkgs.length === 0) {
    toast('warn', '请输入包名')
    return
  }
  const files = await run(`已提取 ${pkgs.length} 个安装包`, () => api.extractApps(state.serial, pkgs))
  if (files && files.length > 0) {
    lastOut.value = files
    await api.revealInFolder(files[0])
  }
}

async function pickPullDir() {
  const p = await api.pickDirectory('选择保存目录')
  if (p) pullLocal.value = p
}

onMounted(() => {
  if (!pullLocal.value) pullLocal.value = state.env?.config.pushDir ?? ''
})
</script>

<template>
  <div class="p-5">
    <div class="mb-4">
      <h1 class="text-[15px] font-medium text-slate-100">文件传输</h1>
      <p class="mt-1 text-[11.5px] text-muted">
        安装 APK（含拆分包与批量）、推送文件、从设备拉取、提取已安装应用的安装包。
      </p>
    </div>

    <div v-if="state.mode !== 'adb'" class="panel py-16 text-center text-[12px] text-muted">
      文件传输需要设备处于 ADB 模式
    </div>

    <div v-else class="grid grid-cols-2 gap-4">
      <!-- 安装 -->
      <div class="panel">
        <div class="panel-header"><span class="panel-title"><Package class="h-3.5 w-3.5" /> 安装应用</span></div>
        <div class="p-4">
          <div class="mb-3 flex items-center gap-4">
            <label class="flex cursor-pointer items-center gap-1.5 text-[11px] text-muted">
              <input v-model="reinstall" type="checkbox" class="accent-accent" /> 覆盖安装（-r）
            </label>
            <label class="flex cursor-pointer items-center gap-1.5 text-[11px] text-muted">
              <input v-model="splitMode" type="checkbox" class="accent-accent" /> 拆分 APK 模式
            </label>
          </div>
          <div class="flex gap-2">
            <button class="btn flex-1" @click="installSingle">
              <Upload class="h-3.5 w-3.5" /> 选择单个 APK
            </button>
            <button class="btn btn-primary flex-1" @click="installMany">
              <Layers class="h-3.5 w-3.5" /> {{ splitMode ? '选择 base + split' : '批量安装' }}
            </button>
          </div>
          <p class="mt-3 text-[10px] leading-relaxed text-muted">
            拆分包请一次选中 <span class="mono">base.apk</span> 与所有 <span class="mono">split_*.apk</span>，
            工具会用 <span class="mono">install-multiple</span> 安装。批量模式则逐个安装。
          </p>
        </div>
      </div>

      <!-- 推送 -->
      <div class="panel">
        <div class="panel-header"><span class="panel-title"><FileUp class="h-3.5 w-3.5" /> 推送文件到设备</span></div>
        <div class="p-4">
          <label class="label">设备目标目录</label>
          <input v-model="remotePath" class="input mono mb-3" placeholder="/sdcard/Download" />
          <button class="btn btn-primary w-full" @click="pushFile">
            <Upload class="h-3.5 w-3.5" /> 选择本地文件并推送
          </button>
          <p class="mt-3 text-[10px] leading-relaxed text-muted">
            目标可以是目录（保留原文件名）或完整文件路径。写入 /sdcard 之外的路径通常需要 root。
          </p>
        </div>
      </div>

      <!-- 拉取 -->
      <div class="panel">
        <div class="panel-header"><span class="panel-title"><HardDriveDownload class="h-3.5 w-3.5" /> 从设备拉取</span></div>
        <div class="p-4">
          <label class="label">设备路径</label>
          <input v-model="pullRemote" class="input mono mb-3" placeholder="/sdcard/DCIM/Camera" />
          <label class="label">保存到</label>
          <div class="mb-3 flex gap-2">
            <input v-model="pullLocal" class="input mono flex-1" placeholder="选择一个目录" />
            <button class="btn btn-icon" title="选择目录" @click="pickPullDir"><FolderOpen class="h-3.5 w-3.5" /></button>
          </div>
          <button class="btn btn-primary w-full" @click="pullFile">
            <FolderDown class="h-3.5 w-3.5" /> 拉取
          </button>
        </div>
      </div>

      <!-- 提取安装包 -->
      <div class="panel">
        <div class="panel-header"><span class="panel-title"><Package class="h-3.5 w-3.5" /> 提取安装包</span></div>
        <div class="p-4">
          <label class="label">包名（多个用逗号或换行分隔）</label>
          <textarea v-model="pkgInput" class="input mono h-[62px] resize-none" placeholder="com.example.app" />
          <button class="btn btn-primary mt-3 w-full" @click="extractApps">
            <FolderDown class="h-3.5 w-3.5" /> 提取到输出目录
          </button>
          <div v-if="lastOut.length" class="mt-3 max-h-[72px] space-y-0.5 overflow-y-auto">
            <div v-for="f in lastOut" :key="f" class="mono truncate text-[10px] text-muted">{{ f }}</div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
