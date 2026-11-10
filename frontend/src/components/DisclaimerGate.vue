<script setup lang="ts">
import { ref } from 'vue'
import { ShieldAlert, LogOut, FileText } from 'lucide-vue-next'
import LegalDoc from './LegalDoc.vue'
import { DISCLAIMER_VERSION, legalState } from '../legal'
import { api } from '../api'
import { refresh, toast } from '../store'

const read = ref(false)
const ack = ref(false)
const busy = ref(false)

const canAccept = () => read.value && ack.value && !busy.value

async function accept() {
  if (!canAccept()) return
  busy.value = true
  try {
    await api.acceptDisclaimer(DISCLAIMER_VERSION)
    await refresh()
    legalState.gateOpen = false
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
  } finally {
    busy.value = false
  }
}

function decline() {
  api.declineDisclaimer()
}
</script>

<template>
  <div v-if="legalState.gateOpen" class="fixed inset-0 z-[100] flex items-center justify-center bg-ink-950/95 p-6">
    <div class="flex h-full max-h-[640px] w-full max-w-[720px] flex-col overflow-hidden rounded border border-ink-600 bg-ink-850 shadow-2xl">
      <!-- 头部 -->
      <div class="flex items-start gap-3 border-b border-ink-700 px-5 py-4">
        <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded bg-danger/15 text-danger">
          <ShieldAlert class="h-[18px] w-[18px]" />
        </div>
        <div class="min-w-0 flex-1">
          <div class="text-[15px] font-medium text-slate-100">使用前必读 · 风险提示与免责声明</div>
          <div class="mt-1 text-[11.5px] leading-relaxed text-muted">
            这是固件级工具，不是普通应用。首次使用前请完整阅读以下条款。
          </div>
        </div>
      </div>

      <!-- 正文 -->
      <LegalDoc @read="read = true" />

      <!-- 底部 -->
      <div class="border-t border-ink-700 bg-ink-900/40 px-5 py-3.5">
        <label class="flex cursor-pointer items-start gap-2.5">
          <input
            v-model="ack"
            type="checkbox"
            :disabled="!read"
            class="mt-0.5 h-3.5 w-3.5 shrink-0 accent-danger disabled:opacity-40"
          />
          <span class="text-[11.5px] leading-[1.6]" :class="read ? 'text-slate-300' : 'text-muted'">
            我已完整阅读并理解上述全部条款，确认我对所操作的设备拥有处置权，理解操作可能导致设备损坏、
            数据丢失与保修失效，并自愿承担由此产生的全部后果。
          </span>
        </label>

        <div class="mt-3 flex items-center justify-between gap-3">
          <button class="btn" @click="decline">
            <LogOut class="h-3.5 w-3.5" /> 不同意并退出
          </button>
          <button class="btn btn-danger h-9 px-4" :disabled="!canAccept()" @click="accept">
            <FileText class="h-3.5 w-3.5" /> {{ busy ? '正在保存…' : '同意并继续' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
