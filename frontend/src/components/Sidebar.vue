<script setup lang="ts">
/**
 * 侧栏：分组与条目全部来自 nav.ts，这里只负责画。
 *
 * 视觉上做了三件事来降噪：
 *   1. 分组标题带一条细线，分组之间自然断开，不再是一列连续的 16 行；
 *   2. 条目高度收到 34px，15 个入口在 860 高的窗口里不用滚动；
 *   3. 设置沉到底部单独一区，不和功能抢视线。
 */
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Github, Search } from 'lucide-vue-next'
import { HOME, NAV_GROUPS, SETTINGS } from '../nav'
import { openPalette } from '../palette'
import { api } from '../api'
import { state } from '../store'

const route = useRoute()
const router = useRouter()

const version = computed(() => (state.env?.appVersion ? `v${state.env.appVersion}` : 'dev'))

function isActive(path: string) {
  return route.path === path
}

function go(path: string) {
  if (route.path !== path) router.push(path)
}
</script>

<template>
  <aside class="flex w-[208px] shrink-0 flex-col border-r border-ink-700 bg-ink-900">
    <!-- 品牌 -->
    <div class="flex h-14 shrink-0 items-center gap-2.5 border-b border-ink-700 px-4">
      <div class="flex h-8 w-8 items-center justify-center rounded border border-accent/40 bg-accent/10">
        <svg viewBox="0 0 24 24" class="h-4 w-4 text-accent" fill="none" stroke="currentColor" stroke-width="2">
          <rect x="6" y="6" width="12" height="12" rx="2" />
          <path d="M9 2v4M15 2v4M9 18v4M15 18v4M2 9h4M2 15h4M18 9h4M18 15h4" stroke-linecap="round" />
        </svg>
      </div>
      <div class="leading-tight">
        <div class="text-[13px] font-semibold tracking-wide text-slate-100">搞机工坊</div>
        <div class="text-[9.5px] tracking-[0.18em] text-muted">DROIDFORGE</div>
      </div>
    </div>

    <!-- 搜索 -->
    <div class="shrink-0 px-2 pb-1 pt-2">
      <button
        class="flex h-8 w-full items-center gap-2 rounded border border-ink-700 bg-ink-850 px-2.5
          text-[11.5px] text-muted transition-colors hover:border-ink-600 hover:text-slate-300"
        @click="openPalette"
      >
        <Search class="h-3.5 w-3.5" />
        <span>搜索功能</span>
        <span class="flex-1" />
        <span class="rounded-sm border border-ink-600 px-1 py-px text-[9px] tracking-wide">Ctrl K</span>
      </button>
    </div>

    <!-- 导航 -->
    <nav class="min-h-0 flex-1 overflow-y-auto pb-2">
      <div
        class="nav-item mt-1 h-[34px]"
        :class="{ 'nav-item-active': isActive(HOME.path) }"
        @click="go(HOME.path)"
      >
        <component :is="HOME.icon" class="h-[15px] w-[15px]" />
        <span>{{ HOME.label }}</span>
      </div>

      <div v-for="g in NAV_GROUPS" :key="g.id">
        <div class="mb-1 mt-3 flex items-center gap-2 px-4" :title="g.hint">
          <span class="shrink-0 text-[9.5px] tracking-[0.16em] text-muted">{{ g.label }}</span>
          <span class="h-px flex-1 bg-ink-700" />
        </div>
        <div
          v-for="item in g.items"
          :key="item.path"
          class="nav-item h-[34px]"
          :class="{ 'nav-item-active': isActive(item.path) }"
          :title="item.desc"
          @click="go(item.path)"
        >
          <component :is="item.icon" class="h-[15px] w-[15px] shrink-0" />
          <span class="truncate">{{ item.label }}</span>
        </div>
      </div>
    </nav>

    <!-- 底部：设置与元信息 -->
    <div class="shrink-0 border-t border-ink-700 px-2 py-2">
      <div
        class="nav-item h-[34px]"
        :class="{ 'nav-item-active': isActive(SETTINGS.path) }"
        :title="SETTINGS.desc"
        @click="go(SETTINGS.path)"
      >
        <component :is="SETTINGS.icon" class="h-[15px] w-[15px]" />
        <span>{{ SETTINGS.label }}</span>
      </div>
      <div class="mt-1 flex items-center gap-2 px-2">
        <button class="btn btn-sm h-7 flex-1" @click="api.openUrl('https://github.com')">
          <Github class="h-3.5 w-3.5" />
          <span>GitHub</span>
        </button>
        <span class="mono shrink-0 text-[10px] text-muted">{{ version }}</span>
      </div>
    </div>
  </aside>
</template>
