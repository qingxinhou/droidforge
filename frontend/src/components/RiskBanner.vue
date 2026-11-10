<script setup lang="ts">
import { computed, ref } from 'vue'
import { AlertOctagon, ChevronDown } from 'lucide-vue-next'

const props = withDefaults(
  defineProps<{
    title: string
    desc?: string
    points?: readonly string[]
    level?: 'high' | 'mid'
  }>(),
  { level: 'high', points: () => [], desc: '' }
)

const open = ref(true)

const theme = computed(() =>
  props.level === 'mid'
    ? {
        box: 'border-warn/30 bg-warn/[0.05]',
        head: 'text-warn',
        bullet: 'bg-warn/70',
      }
    : {
        box: 'border-danger/30 bg-danger/[0.06]',
        head: 'text-danger',
        bullet: 'bg-danger/70',
      }
)
</script>

<template>
  <div class="mb-4 rounded border" :class="theme.box">
    <button class="flex w-full items-start gap-2.5 px-3.5 py-2.5 text-left" @click="open = !open">
      <AlertOctagon class="mt-0.5 h-4 w-4 shrink-0" :class="theme.head" />
      <div class="min-w-0 flex-1">
        <div class="text-[12px] font-medium" :class="theme.head">{{ title }}</div>
        <div v-if="desc" class="mt-0.5 text-[11px] leading-[1.6] text-slate-400">{{ desc }}</div>
      </div>
      <ChevronDown
        class="mt-0.5 h-3.5 w-3.5 shrink-0 text-slate-500 transition-transform duration-150"
        :class="open ? 'rotate-180' : ''"
      />
    </button>

    <ul v-if="open && points.length" class="space-y-1 px-3.5 pb-3 pl-[38px]">
      <li v-for="(p, i) in points" :key="i" class="flex gap-1.5 text-[11px] leading-[1.65] text-slate-400">
        <span class="mt-[6px] h-1 w-1 shrink-0 rounded-full" :class="theme.bullet" />
        <span>{{ p }}</span>
      </li>
    </ul>
  </div>
</template>
