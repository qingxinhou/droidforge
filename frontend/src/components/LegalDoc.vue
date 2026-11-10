<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { CLAUSES, DISCLAIMER_DATE, DISCLAIMER_VERSION } from '../legal'
import { ChevronDown } from 'lucide-vue-next'

const emit = defineEmits<{ (e: 'read'): void }>()

const el = ref<HTMLElement | null>(null)
const atBottom = ref(false)

function check() {
  const e = el.value
  if (!e) return
  if (e.scrollHeight - e.scrollTop - e.clientHeight <= 32) {
    if (!atBottom.value) {
      atBottom.value = true
      emit('read')
    }
  }
}

function onResize() {
  check()
}

onMounted(async () => {
  await nextTick()
  check()
  window.addEventListener('resize', onResize)
})

onBeforeUnmount(() => window.removeEventListener('resize', onResize))
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <!-- 未读到底时的提示条，独立占位，不遮挡正文 -->
    <div
      v-if="!atBottom"
      class="flex shrink-0 items-center justify-center gap-1 border-b border-ink-800 bg-ink-900/50 py-1 text-[10.5px] text-muted"
    >
      <ChevronDown class="h-3 w-3" /> 请向下滚动阅读完整条款
    </div>

    <div
      ref="el"
      class="min-h-0 flex-1 overflow-y-auto px-5 py-4 text-[12px] leading-[1.75] text-slate-400"
      @scroll.passive="check"
    >
      <p class="mb-4 rounded border border-danger/25 bg-danger/[0.06] px-3 py-2.5 text-[11.5px] leading-[1.7] text-slate-300">
        本软件封装的是固件级操作。刷写分区、擦除数据、注入 Root 权限都可能让设备
        <span class="font-medium text-danger">无法开机、丢失全部数据</span>。请确认你已备份，
        并清楚如何在出问题时恢复设备。继续使用即表示你自行承担全部后果。
      </p>

      <section v-for="c in CLAUSES" :key="c.title" class="mb-4 last:mb-0">
        <h3 class="mb-1.5 text-[12px] font-medium text-slate-200">{{ c.title }}</h3>
        <p v-for="(p, i) in c.body" :key="i" class="mb-1.5 last:mb-0">{{ p }}</p>
      </section>

      <div class="mt-4 border-t border-ink-700 pt-3 text-[10.5px] text-muted">
        版本 v{{ DISCLAIMER_VERSION }} · 生效日期 {{ DISCLAIMER_DATE }} · 本软件以 MIT
        许可发布，按“现状”提供，不附带任何形式的担保
      </div>
    </div>
  </div>
</template>
