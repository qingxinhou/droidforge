<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RefreshCw, Search, Download, HardDrive, Layers } from 'lucide-vue-next'
import { api } from '../api'
import { state, toast, fmtBytes, needAdb, run } from '../store'
import type { Partition } from '../types'

const list = ref<Partition[]>([])
const keyword = ref('')
const loading = ref(false)

const filtered = computed(() => {
  const k = keyword.value.trim().toLowerCase()
  if (!k) return list.value
  return list.value.filter(
    (p) => p.name.toLowerCase().includes(k) || p.block.toLowerCase().includes(k)
  )
})

const totalSize = computed(() => list.value.reduce((s, p) => s + p.sizeBytes, 0))

async function load() {
  if (!needAdb()) return
  loading.value = true
  try {
    list.value = await api.getPartitions(state.serial)
    toast('ok', `读取到 ${list.value.length} 个分区`)
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
  } finally {
    loading.value = false
  }
}

async function backup(p: Partition) {
  const out = await run('', () => api.extractBoot(state.serial, p.name, ''))
  if (out) {
    toast('ok', `已导出 ${p.name}`)
    await api.revealInFolder(out)
  }
}

onMounted(() => {
  if (state.serial && state.mode === 'adb') void load()
})
</script>

<template>
  <div class="p-5">
    <div class="mb-4 flex items-end justify-between">
      <div>
        <h1 class="text-[15px] font-medium text-slate-100">分区管理</h1>
        <p class="mt-1 text-[11.5px] text-muted">
          查看设备分区表与块设备映射。导出分区镜像需要设备已 root。
        </p>
      </div>
      <div class="flex items-center gap-2">
        <div class="relative">
          <Search class="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-500" />
          <input v-model="keyword" class="input w-[220px] pl-7" placeholder="搜索分区名 / 块设备" />
        </div>
        <button class="btn" :disabled="loading" @click="load">
          <RefreshCw class="h-3.5 w-3.5" :class="loading ? 'animate-spin' : ''" /> 读取分区表
        </button>
      </div>
    </div>

    <div v-if="state.mode !== 'adb'" class="panel py-16 text-center text-[12px] text-muted">
      读取分区表需要设备处于 ADB 模式
    </div>

    <div v-else-if="list.length === 0" class="panel flex flex-col items-center justify-center py-20">
      <HardDrive class="mb-3 h-9 w-9 text-slate-600" />
      <div class="text-[12.5px] text-slate-300">尚未读取分区表</div>
      <button class="btn btn-primary mt-4" @click="load">立即读取</button>
    </div>

    <div v-else class="panel overflow-hidden">
      <div class="flex items-center gap-3 border-b border-ink-700 px-4 py-2 text-[11px]">
        <span class="text-slate-300">{{ list.length }} 个分区</span>
        <span class="tag">{{ fmtBytes(totalSize) }}</span>
        <span class="tag tag-accent">{{ list.filter(p => p.dynamic).length }} 动态</span>
      </div>
      <div class="max-h-[540px] overflow-y-auto">
        <table class="w-full text-[11px]">
          <thead class="sticky top-0 bg-ink-850 text-muted">
            <tr class="border-b border-ink-700">
              <th class="px-4 py-2 text-left font-normal">分区名</th>
              <th class="px-3 py-2 text-left font-normal">块设备</th>
              <th class="px-3 py-2 text-right font-normal">大小</th>
              <th class="px-3 py-2 text-left font-normal">槽位</th>
              <th class="px-3 py-2 text-left font-normal">属性</th>
              <th class="px-3 py-2 text-left font-normal">挂载点</th>
              <th class="px-3 py-2 text-right font-normal">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="p in filtered" :key="p.name + p.block" class="border-b border-ink-800 hover:bg-ink-800/50">
              <td class="px-4 py-1.5 mono text-slate-200">{{ p.name }}</td>
              <td class="px-3 py-1.5 mono text-[10px] text-muted">{{ p.block }}</td>
              <td class="px-3 py-1.5 text-right mono text-slate-400">{{ p.sizeBytes ? fmtBytes(p.sizeBytes) : '—' }}</td>
              <td class="px-3 py-1.5">
                <span v-if="p.slot" class="tag">{{ p.slot.toUpperCase() }}</span>
                <span v-else class="text-slate-700">—</span>
              </td>
              <td class="px-3 py-1.5">
                <span v-if="p.dynamic" class="tag tag-accent"><Layers class="h-2.5 w-2.5" /> 动态</span>
                <span v-else class="text-slate-700">—</span>
              </td>
              <td class="px-3 py-1.5 mono text-[10px] text-muted">{{ p.mountPoint || '—' }}</td>
              <td class="px-3 py-1.5 text-right">
                <button class="btn btn-sm" @click="backup(p)">
                  <Download class="h-3 w-3" /> 导出
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
