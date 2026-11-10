<script setup lang="ts">
import { ShieldAlert, X } from 'lucide-vue-next'
import LegalDoc from './LegalDoc.vue'
import { legalState } from '../legal'
</script>

<template>
  <transition name="fade">
    <div v-if="legalState.docOpen" class="fixed inset-0 z-[70] flex items-center justify-center">
      <div class="absolute inset-0 bg-[var(--c-overlay)]" @click="legalState.docOpen = false" />
      <div class="relative flex h-[80%] w-[680px] flex-col overflow-hidden rounded border border-ink-600 bg-ink-850 shadow-2xl">
        <div class="flex items-center gap-2.5 border-b border-ink-700 px-4 py-3">
          <ShieldAlert class="h-4 w-4 text-danger" />
          <span class="flex-1 text-[13px] font-medium text-slate-100">风险提示与免责声明</span>
          <button class="btn btn-icon btn-sm" @click="legalState.docOpen = false">
            <X class="h-3.5 w-3.5" />
          </button>
        </div>
        <LegalDoc />
      </div>
    </div>
  </transition>
</template>
