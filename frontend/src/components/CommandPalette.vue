<script setup lang="ts">
/**
 * 命令面板：Ctrl / Cmd + K 呼出，直接跳到任意功能。
 *
 * 页面一多，侧栏就只能靠记位置。这里按同一个 nav.ts 做全文检索，
 * 支持名称、说明、路径与英文别名（比如输入 payload 能找到「刷机包提取」）。
 */
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Search, CornerDownLeft } from 'lucide-vue-next'
import { HOME, NAV_GROUPS, SETTINGS, NEEDS_LABEL, type NavItem } from '../nav'
import { paletteState, closePalette, togglePalette } from '../palette'
import { state } from '../store'

type Row =
  | { kind: 'group'; label: string }
  | { kind: 'item'; item: NavItem }

const router = useRouter()
const q = ref('')
const active = ref(0)
const inputEl = ref<HTMLInputElement | null>(null)

/** 命中逻辑：多个关键词按空格分开，全部命中才算匹配。 */
function matches(i: NavItem, kw: string): boolean {
  if (!kw) return true
  const hay = [i.label, i.desc, i.path, ...(i.keywords ?? [])].join(' ').toLowerCase()
  return kw.split(/\s+/).every((t) => hay.includes(t))
}

const rows = computed<Row[]>(() => {
  const kw = q.value.trim().toLowerCase()
  const out: Row[] = []

  if (matches(HOME, kw)) {
    out.push({ kind: 'group', label: '常用' })
    out.push({ kind: 'item', item: HOME })
  }

  for (const g of NAV_GROUPS) {
    const hit = g.items.filter((i) => matches(i, kw))
    if (!hit.length) continue
    out.push({ kind: 'group', label: g.label })
    for (const i of hit) out.push({ kind: 'item', item: i })
  }

  if (matches(SETTINGS, kw)) {
    out.push({ kind: 'group', label: '系统' })
    out.push({ kind: 'item', item: SETTINGS })
  }
  return out
})

const items = computed(() =>
  rows.value.filter((r): r is Extract<Row, { kind: 'item' }> => r.kind === 'item').map((r) => r.item)
)

/** 当前设备状态下访问该页面会不会卡住，卡住就在右侧标出来。 */
function needHint(i: NavItem): string {
  if (i.needs === 'any') return ''
  if (!state.serial) return NEEDS_LABEL[i.needs]
  return state.mode === i.needs ? '' : NEEDS_LABEL[i.needs]
}

function move(delta: number) {
  const n = items.value.length
  if (!n) return
  active.value = (active.value + delta + n) % n
  nextTick(() => {
    document.querySelector('[data-palette-active="1"]')?.scrollIntoView({ block: 'nearest' })
  })
}

function pick(item?: NavItem) {
  if (!item) return
  closePalette()
  if (router.currentRoute.value.path !== item.path) router.push(item.path)
}

function onKey(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
    e.preventDefault()
    togglePalette()
    return
  }

  if (paletteState.open) {
    if (e.key === 'Escape') {
      e.preventDefault()
      closePalette()
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      move(1)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      move(-1)
    } else if (e.key === 'Enter') {
      e.preventDefault()
      pick(items.value[active.value])
    }
    return
  }

  // 单按 / 也能呼出，但别抢输入框里的斜杠
  const t = e.target as HTMLElement | null
  const typing = !!t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)
  if (e.key === '/' && !typing) {
    e.preventDefault()
    paletteState.open = true
  }
}

watch(
  () => paletteState.open,
  async (open) => {
    if (open) {
      q.value = ''
      active.value = 0
      await nextTick()
      inputEl.value?.focus()
    }
  }
)

watch(q, () => {
  active.value = 0
})

onMounted(() => window.addEventListener('keydown', onKey))
onUnmounted(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <Teleport to="body">
    <div
      v-if="paletteState.open"
      class="fixed inset-0 z-[70] flex items-start justify-center bg-[var(--c-overlay)] px-4 pt-[12vh]"
      @click.self="closePalette"
    >
      <div
        class="w-full max-w-[560px] overflow-hidden rounded border border-ink-600 bg-ink-850 shadow-2xl"
      >
        <!-- 输入 -->
        <div class="flex items-center gap-2 border-b border-ink-700 px-3">
          <Search class="h-4 w-4 shrink-0 text-muted" />
          <input
            ref="inputEl"
            v-model="q"
            class="h-11 min-w-0 flex-1 bg-transparent text-[13px] text-slate-100 outline-none placeholder:text-muted"
            placeholder="搜索功能，例如 payload、状态栏、root"
            spellcheck="false"
          />
          <button class="tag shrink-0 hover:text-slate-300" @click="closePalette">Esc</button>
        </div>

        <!-- 结果 -->
        <div class="max-h-[46vh] overflow-y-auto py-1">
          <div v-if="!items.length" class="px-4 py-8 text-center text-[12px] text-muted">
            没有匹配的功能
          </div>

          <template v-for="(r, idx) in rows" :key="idx">
            <div
              v-if="r.kind === 'group'"
              class="px-4 pb-1 pt-2.5 text-[9.5px] tracking-[0.16em] text-muted"
            >
              {{ r.label }}
            </div>
            <button
              v-else
              class="flex w-full items-center gap-2.5 px-3 py-2 text-left"
              :class="items[active] === r.item ? 'bg-accent/10' : 'hover:bg-ink-800'"
              :data-palette-active="items[active] === r.item ? '1' : '0'"
              @click="pick(r.item)"
              @mouseenter="active = items.indexOf(r.item)"
            >
              <component
                :is="r.item.icon"
                class="h-4 w-4 shrink-0"
                :class="items[active] === r.item ? 'text-accent' : 'text-muted'"
              />
              <div class="min-w-0 flex-1">
                <div
                  class="text-[12.5px]"
                  :class="items[active] === r.item ? 'text-accent' : 'text-slate-200'"
                >
                  {{ r.item.label }}
                </div>
                <div class="truncate text-[10.5px] text-muted">{{ r.item.desc }}</div>
              </div>
              <span v-if="r.item.risk" class="tag tag-warn shrink-0">有风险</span>
              <span v-if="needHint(r.item)" class="tag shrink-0">{{ needHint(r.item) }}</span>
            </button>
          </template>
        </div>

        <!-- 底部说明 -->
        <div class="flex items-center gap-3 border-t border-ink-700 px-3 py-2 text-[10px] text-muted">
          <span class="flex items-center gap-1"><span class="tag mono">↑</span><span class="tag mono">↓</span> 选择</span>
          <span class="flex items-center gap-1"><CornerDownLeft class="h-3 w-3" /> 打开</span>
          <span class="flex-1" />
          <span>{{ items.length }} 个功能</span>
        </div>
      </div>
    </div>
  </Teleport>
</template>
