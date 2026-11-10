<script setup lang="ts">
import { computed } from 'vue'
import { AlertTriangle, AlertOctagon, ShieldAlert } from 'lucide-vue-next'
import { dialogState, resolveDialog, setAck } from '../dialog'

const canConfirm = computed(() => !dialogState.requireAck || dialogState.ack)
</script>

<template>
  <transition name="fade">
    <div v-if="dialogState.open" class="fixed inset-0 z-[60] flex items-center justify-center">
      <div class="absolute inset-0 bg-[var(--c-overlay)]" @click="resolveDialog(false)" />
      <div class="relative w-[460px] rounded border border-ink-600 bg-ink-850 shadow-2xl">
        <div class="flex items-start gap-3 border-b border-ink-700 px-4 py-3">
          <div
            class="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded"
            :class="dialogState.danger ? 'bg-danger/15 text-danger' : 'bg-accent/15 text-accent'"
          >
            <AlertTriangle class="h-4 w-4" />
          </div>
          <div class="min-w-0 flex-1">
            <div class="text-[13px] font-medium text-slate-100">{{ dialogState.title }}</div>
            <div v-if="dialogState.danger" class="mt-0.5 text-[10.5px] text-danger/80">
              高危操作 · 可能造成设备损坏或数据丢失
            </div>
          </div>
        </div>

        <div class="px-4 py-4 text-[12px] leading-relaxed text-slate-300">
          <div class="whitespace-pre-wrap">{{ dialogState.message }}</div>

          <!-- 风险清单 -->
          <div
            v-if="dialogState.risks.length"
            class="mt-3 rounded border border-danger/25 bg-danger/[0.06] px-3 py-2.5"
          >
            <div class="mb-1.5 flex items-center gap-1.5 text-[11px] font-medium text-danger">
              <AlertOctagon class="h-3.5 w-3.5" /> 请先确认你了解以下风险
            </div>
            <ul class="space-y-1">
              <li
                v-for="(r, i) in dialogState.risks"
                :key="i"
                class="flex gap-1.5 text-[11.5px] leading-[1.6] text-slate-400"
              >
                <span class="mt-[6px] h-1 w-1 shrink-0 rounded-full bg-danger/70" />
                <span>{{ r }}</span>
              </li>
            </ul>
          </div>
        </div>

        <!-- 勾选确认 -->
        <label
          v-if="dialogState.requireAck"
          class="mx-4 mb-3 flex cursor-pointer items-start gap-2.5 rounded border border-ink-700 bg-ink-900/60 px-3 py-2.5"
        >
          <input
            :checked="dialogState.ack"
            type="checkbox"
            class="mt-0.5 h-3.5 w-3.5 shrink-0 accent-danger"
            @change="setAck(($event.target as HTMLInputElement).checked)"
          />
          <span class="text-[11.5px] leading-[1.5] text-slate-400">{{ dialogState.ackText }}</span>
        </label>

        <div class="flex items-center justify-between gap-2 border-t border-ink-700 px-4 py-3">
          <div class="flex items-center gap-1 text-[10px] text-muted">
            <ShieldAlert class="h-3 w-3" />
            本操作不可撤销
          </div>
          <div class="flex gap-2">
            <button class="btn" @click="resolveDialog(false)">{{ dialogState.cancelText }}</button>
            <button
              class="btn"
              :class="dialogState.danger ? 'btn-danger' : 'btn-primary'"
              :disabled="!canConfirm"
              @click="resolveDialog(true)"
            >
              {{ dialogState.confirmText }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </transition>
</template>
