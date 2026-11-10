<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  Monitor, Maximize2, Type, Timer, Keyboard, Camera, RotateCcw,
  ArrowLeft, Circle, Square, Power, Volume2, Volume1, Play, Pause, Music,
} from 'lucide-vue-next'
import { api } from '../api'
import { state, toast, needAdb, run } from '../store'
import type { DisplayState } from '../types'

const d = ref<DisplayState | null>(null)
const size = ref('')
const density = ref<number | null>(null)
const fontScale = ref(1)
const timeoutMs = ref(30000)
const anim = ref({ window: 1, transition: 1, animator: 1 })
const inputText = ref('')

const dirty = computed(() => {
  const cur = d.value
  if (!cur) return false
  return (
    Math.abs(fontScale.value - cur.fontScale) > 0.001 ||
    timeoutMs.value !== cur.screenTimeoutMs ||
    Math.abs(anim.value.window - cur.windowAnim) > 0.001 ||
    Math.abs(anim.value.transition - cur.transitionAnim) > 0.001 ||
    Math.abs(anim.value.animator - cur.animatorAnim) > 0.001
  )
})

const timeoutPresets = [
  { label: '15 秒', value: 15000 },
  { label: '30 秒', value: 30000 },
  { label: '1 分钟', value: 60000 },
  { label: '2 分钟', value: 120000 },
  { label: '5 分钟', value: 300000 },
  { label: '10 分钟', value: 600000 },
  { label: '30 分钟', value: 1800000 },
]

const keys = [
  { label: '返回', code: '4', icon: ArrowLeft },
  { label: '主页', code: '3', icon: Circle },
  { label: '多任务', code: '187', icon: Square },
  { label: '电源', code: '26', icon: Power },
  { label: '音量 +', code: '24', icon: Volume2 },
  { label: '音量 -', code: '25', icon: Volume1 },
  { label: '播放/暂停', code: '85', icon: Play },
  { label: '下一曲', code: '87', icon: Music },
  { label: '暂停', code: '127', icon: Pause },
]

async function load() {
  if (!state.serial || state.mode !== 'adb') return
  try {
    const t = await api.getTweaks(state.serial)
    d.value = t.display
    fontScale.value = t.display.fontScale || 1
    timeoutMs.value = nearestTimeout(t.display.screenTimeoutMs)
    anim.value = {
      window: t.display.windowAnim,
      transition: t.display.transitionAnim,
      animator: t.display.animatorAnim,
    }
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
  }
}

function nearestTimeout(ms: number): number {
  if (!ms) return 30000
  let best = timeoutPresets[0].value
  let diff = Infinity
  for (const p of timeoutPresets) {
    const dv = Math.abs(p.value - ms)
    if (dv < diff) {
      diff = dv
      best = p.value
    }
  }
  return best
}

async function applySize() {
  if (!needAdb()) return
  const v = size.value.trim()
  if (v && !/^\d{3,4}x\d{3,4}$/i.test(v)) {
    toast('warn', '格式应为 1080x2400')
    return
  }
  await run(v ? `分辨率已设为 ${v}` : '已恢复默认分辨率', () =>
    api.setResolution(state.serial, v),
  )
  await load()
}

async function applyDensity() {
  if (!needAdb()) return
  await run(density.value ? `密度已设为 ${density.value}` : '已恢复默认密度', () =>
    api.setDensity(state.serial, density.value ?? 0),
  )
  await load()
}

async function applyFont() {
  if (!needAdb()) return
  await run(`字体缩放 ${fontScale.value.toFixed(2)}x`, () =>
    api.setFontScale(state.serial, fontScale.value),
  )
  await load()
}

async function applyAnim() {
  if (!needAdb()) return
  await run('动画倍率已应用', () =>
    api.setAnimationScales(state.serial, anim.value.window, anim.value.transition, anim.value.animator),
  )
  await load()
}

async function killAnim() {
  if (!needAdb()) return
  anim.value = { window: 0, transition: 0, animator: 0 }
  await applyAnim()
}

async function resetAnim() {
  if (!needAdb()) return
  anim.value = { window: 1, transition: 1, animator: 1 }
  await applyAnim()
}

async function applyTimeout() {
  if (!needAdb()) return
  await run('锁屏时间已更新', () => api.setScreenTimeout(state.serial, timeoutMs.value))
  await load()
}

async function resetAll() {
  if (!needAdb()) return
  await run('已恢复显示默认值', async () => {
    await api.setResolution(state.serial, '')
    await api.setDensity(state.serial, 0)
    await api.setFontScale(state.serial, 1)
    await api.setScreenTimeout(state.serial, 30000)
    await api.setAnimationScales(state.serial, 1, 1, 1)
  })
  await load()
}

async function key(code: string) {
  if (!needAdb()) return
  await run('', () => api.sendKeyEvent(state.serial, code))
}

async function sendText() {
  if (!needAdb()) return
  if (!inputText.value.trim()) return
  await run('文本已发送', () => api.sendInputText(state.serial, inputText.value))
  inputText.value = ''
}

async function shot() {
  if (!needAdb()) return
  const p = await run('截图已保存', () => api.screenshot(state.serial))
  if (p) await api.revealInFolder(p)
}

watch(() => [state.serial, state.mode], () => void load())
onMounted(() => void load())
</script>

<template>
  <div class="p-5">
    <div class="mb-4 flex items-end justify-between">
      <div>
        <h1 class="text-[15px] font-medium text-slate-100">显示控制</h1>
        <p class="mt-1 text-[11.5px] text-muted">
          分辨率、密度、字体、动画、锁屏与按键模拟。全部走 ADB，无需 root。
        </p>
      </div>
      <div class="flex gap-2">
        <button class="btn" :disabled="!d" @click="load"><RotateCcw class="h-3.5 w-3.5" /> 重新读取</button>
        <button class="btn btn-danger" @click="resetAll">恢复显示默认值</button>
      </div>
    </div>

    <div v-if="state.mode !== 'adb'" class="panel py-16 text-center text-[12px] text-muted">
      该功能需要设备处于 ADB 模式
    </div>

    <div v-else class="grid grid-cols-2 gap-4">
      <!-- 分辨率与密度 -->
      <div class="panel">
        <div class="panel-header"><span class="panel-title"><Maximize2 class="h-3.5 w-3.5" /> 分辨率与密度</span></div>
        <div class="p-4">
          <div class="mb-3 rounded border border-ink-700 bg-ink-900 px-3 py-2">
            <div class="kv"><span class="kv-k">物理分辨率</span><span class="kv-v mono">{{ d?.physicalSize || '—' }}</span></div>
            <div class="kv"><span class="kv-k">当前覆盖</span><span class="kv-v mono">{{ d?.overrideSize || '未设置' }}</span></div>
            <div class="kv"><span class="kv-k">物理密度</span><span class="kv-v mono">{{ d?.physicalDensity || '—' }}</span></div>
            <div class="kv"><span class="kv-k">当前覆盖密度</span><span class="kv-v mono">{{ d?.overrideDensity || '未设置' }}</span></div>
          </div>

          <label class="label">分辨率</label>
          <div class="mb-3 flex gap-2">
            <input v-model="size" class="input mono flex-1" placeholder="1080x2400（留空恢复默认）" />
            <button class="btn btn-primary" @click="applySize">应用</button>
          </div>

          <label class="label">显示密度（DPI）</label>
          <div class="flex gap-2">
            <input v-model.number="density" type="number" class="input mono flex-1" placeholder="420（留空恢复默认）" />
            <button class="btn btn-primary" @click="applyDensity">应用</button>
          </div>
          <p class="mt-2 text-[10px] leading-relaxed text-muted">
            修改后若界面错乱，点右上「恢复显示默认值」即可。部分 ROM 会拒绝非标准分辨率。
          </p>
        </div>
      </div>

      <!-- 字体与动画 -->
      <div class="panel">
        <div class="panel-header"><span class="panel-title"><Type class="h-3.5 w-3.5" /> 字体与动画</span></div>
        <div class="p-4">
          <label class="label">字体缩放倍数</label>
          <div class="mb-1 flex items-center gap-2">
            <button class="btn btn-icon" @click="fontScale = Math.max(0.5, +(fontScale - 0.05).toFixed(2))">−</button>
            <input v-model.number="fontScale" type="number" step="0.05" min="0.5" max="2" class="input mono w-24 text-center" />
            <button class="btn btn-icon" @click="fontScale = Math.min(2, +(fontScale + 0.05).toFixed(2))">＋</button>
            <button class="btn btn-primary ml-auto" @click="applyFont">应用</button>
          </div>
          <div class="mb-4 text-[10px] text-muted">当前：{{ d?.fontScale ?? '—' }}x</div>

          <label class="label">动画倍率</label>
          <div class="grid grid-cols-3 gap-2">
            <div>
              <div class="mb-1 text-[10px] text-muted">窗口</div>
              <input v-model.number="anim.window" type="number" step="0.1" min="0" max="10" class="input mono" />
            </div>
            <div>
              <div class="mb-1 text-[10px] text-muted">过渡</div>
              <input v-model.number="anim.transition" type="number" step="0.1" min="0" max="10" class="input mono" />
            </div>
            <div>
              <div class="mb-1 text-[10px] text-muted">动画时长</div>
              <input v-model.number="anim.animator" type="number" step="0.1" min="0" max="10" class="input mono" />
            </div>
          </div>
          <div class="mt-2 flex gap-2">
            <button class="btn flex-1" @click="killAnim">关闭动画（0×）</button>
            <button class="btn flex-1" @click="resetAnim">恢复 1×</button>
            <button class="btn btn-primary" :class="dirty ? '' : 'opacity-50'" @click="applyAnim">应用</button>
          </div>
        </div>
      </div>

      <!-- 锁屏 -->
      <div class="panel">
        <div class="panel-header"><span class="panel-title"><Timer class="h-3.5 w-3.5" /> 自动锁屏</span></div>
        <div class="p-4">
          <div class="grid grid-cols-4 gap-2">
            <button
              v-for="p in timeoutPresets"
              :key="p.value"
              class="btn"
              :class="timeoutMs === p.value ? 'btn-primary' : ''"
              @click="timeoutMs = p.value"
            >
              {{ p.label }}
            </button>
          </div>
          <div class="mt-3 flex items-center justify-between">
            <span class="text-[10px] text-muted">当前生效：{{ d?.screenTimeoutMs ? d.screenTimeoutMs / 1000 + ' 秒' : '—' }}</span>
            <button class="btn btn-primary" @click="applyTimeout">应用</button>
          </div>
        </div>
      </div>

      <!-- 按键与截图 -->
      <div class="panel">
        <div class="panel-header"><span class="panel-title"><Keyboard class="h-3.5 w-3.5" /> 按键模拟与截图</span></div>
        <div class="p-4">
          <div class="grid grid-cols-3 gap-2">
            <button v-for="k in keys" :key="k.code" class="btn" @click="key(k.code)">
              <component :is="k.icon" class="h-3.5 w-3.5" /> {{ k.label }}
            </button>
          </div>
          <div class="mt-3 flex gap-2">
            <input v-model="inputText" class="input flex-1" placeholder="向当前输入框发送文本（仅 ASCII）" @keyup.enter="sendText" />
            <button class="btn" @click="sendText">发送</button>
            <button class="btn btn-primary" @click="shot"><Camera class="h-3.5 w-3.5" /> 截图</button>
          </div>
        </div>
      </div>
    </div>

    <div class="panel mt-4 flex items-start gap-2 border-warn/20 bg-warn/5 px-3 py-2.5">
      <Monitor class="mt-0.5 h-4 w-4 shrink-0 text-warn" />
      <div class="text-[10.5px] leading-relaxed text-muted">
        分辨率与密度是「覆盖值」，随时可恢复；动画倍率与字体缩放写的是系统设置，卸载应用不会自动还原。
      </div>
    </div>
  </div>
</template>
