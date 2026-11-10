<script setup lang="ts">
import { CheckCircle2, XCircle, AlertTriangle, Info } from 'lucide-vue-next'
import { state } from '../store'
</script>

<template>
  <div class="pointer-events-none fixed bottom-4 right-4 z-50 flex w-[360px] flex-col gap-2">
    <transition-group name="fade">
      <div
        v-for="t in state.toasts"
        :key="t.id"
        class="pointer-events-auto flex items-start gap-2 rounded border px-3 py-2 shadow-2xl backdrop-blur"
        :class="{
          'border-ok/40 bg-ok/15 text-ok': t.type === 'ok',
          'border-danger/40 bg-danger/15 text-danger': t.type === 'err',
          'border-warn/40 bg-warn/15 text-warn': t.type === 'warn',
          'border-accent/40 bg-accent/15 text-accent': t.type === 'info',
        }"
      >
        <CheckCircle2 v-if="t.type === 'ok'" class="mt-0.5 h-4 w-4 shrink-0" />
        <XCircle v-else-if="t.type === 'err'" class="mt-0.5 h-4 w-4 shrink-0" />
        <AlertTriangle v-else-if="t.type === 'warn'" class="mt-0.5 h-4 w-4 shrink-0" />
        <Info v-else class="mt-0.5 h-4 w-4 shrink-0" />
        <div class="whitespace-pre-wrap break-words text-[12px] leading-relaxed">{{ t.text }}</div>
      </div>
    </transition-group>
  </div>
</template>
