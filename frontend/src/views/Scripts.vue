<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  Play, Square, FileUp, FileDown, Eraser, BookOpen, Terminal, Wand2,
} from 'lucide-vue-next'
import { api } from '../api'
import { state, toast, needAdb, run, pushLog } from '../store'
import { confirmDialog } from '../dialog'
import RiskBanner from '../components/RiskBanner.vue'
import { RISK_POINTS } from '../legal'

/** 会直接破坏设备的命令模式，命中就要求二次确认。 */
const DESTRUCTIVE = [
  /fastboot\s+(?:-\S+\s+)*(?:erase|flash|format)/i,
  /\berase\b/i,
  /\bflash\s+\w+/i,
  /adb\s+shell\s+rm\s+-r?f/i,
  /adb\s+shell\s+dd\b/i,
  /\bpm\s+clear\b/i,
  /\brecovery\s+--wipe/i,
  /adb\s+shell\s+reboot\s+(?:recovery|bootloader|edl)/i,
]

function hasDestructiveCommand(src: string): boolean {
  return DESTRUCTIVE.some((re) => re.test(src))
}

const KEY = 'droidforge.script'

const sample = `@echo off
echo 设备型号
adb shell getprop ro.product.model
echo 当前分辨率
adb shell wm size
echo 当前密度
adb shell wm density
echo 完成
`

const source = ref(sample)
const running = ref(false)
const showHelp = ref(false)

const samples = [
  { label: '读取设备信息', code: sample },
  {
    label: '关闭动画 + 隐藏状态栏图标',
    code: `@echo off
echo 关闭全部动画
adb shell settings put global window_animation_scale 0
adb shell settings put global transition_animation_scale 0
adb shell settings put global animator_duration_scale 0
echo 隐藏旋转锁定与蓝牙图标
adb shell settings put secure icon_blacklist rotate,bluetooth
echo 完成
`,
  },
  {
    label: '循环 + 变量示例',
    code: `@echo off
set /a n=3
echo 连续按三次音量减
for /l %%i in (1,1,%n%) do (
  echo 第 %%i 次
  adb shell input keyevent 25
  timeout /t 1
)
echo 完成
`,
  },
  {
    label: '条件判断示例',
    code: `@echo off
adb shell getprop ro.build.version.sdk
if errorlevel 1 (
  echo 读取失败
) else (
  echo 读取成功
)
`,
  },
]

function persist() {
  localStorage.setItem(KEY, source.value)
}

async function runScript() {
  if (!needAdb()) return
  if (!source.value.trim()) {
    toast('warn', '脚本为空')
    return
  }
  // 只有脚本里出现破坏性命令时才打断，避免正常脚本每次都被弹窗骚扰
  if (hasDestructiveCommand(source.value)) {
    const ok = await confirmDialog({
      title: '脚本包含破坏性命令',
      message: '检测到脚本中存在擦除、刷写或删除类命令。\n这些命令会直接作用在设备上，且无法撤销。',
      danger: true,
      risks: RISK_POINTS.scripts,
      confirmText: '仍然执行',
    })
    if (!ok) return
  }
  running.value = true
  try {
    const res = await run('', () => api.runScript(state.serial, source.value))
    if (res) {
      pushLog({ level: 'info', text: `脚本结束：${res.steps} 步 / ${res.failures} 个错误`, time: new Date().toLocaleTimeString('zh-CN', { hour12: false }) })
      toast(res.failures ? 'warn' : 'ok', `${res.steps} 步，${res.failures} 个错误`)
    }
  } finally {
    running.value = false
  }
}

async function stop() {
  await api.cancelOperation()
  running.value = false
  toast('warn', '已请求中止')
}

async function importScript() {
  const p = await api.pickFile('导入脚本', '*.bat;*.txt;*.sh')
  if (!p) return
  try {
    source.value = await api.readTextFile(p)
    persist()
    toast('ok', '已导入')
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
  }
}

async function exportScript() {
  const p = await api.saveTextFile('droidforge-script.bat', source.value)
  if (p) {
    toast('ok', '已导出')
    await api.revealInFolder(p)
  }
}

function useSample(code: string) {
  source.value = code
  persist()
}

onMounted(() => {
  const saved = localStorage.getItem(KEY)
  if (saved) source.value = saved
})
</script>

<template>
  <div class="p-5">
    <div class="mb-4 flex items-end justify-between">
      <div>
        <h1 class="text-[15px] font-medium text-slate-100">控机脚本</h1>
        <p class="mt-1 text-[11.5px] text-muted">
          用 BAT 语法批量执行 ADB 操作。只驱动 adb，不会调用本机 cmd 或执行 EXE。
        </p>
      </div>
      <div class="flex gap-2">
        <button class="btn" @click="importScript"><FileUp class="h-3.5 w-3.5" /> 导入</button>
        <button class="btn" @click="exportScript"><FileDown class="h-3.5 w-3.5" /> 导出</button>
        <button class="btn" @click="source = sample; persist()"><Eraser class="h-3.5 w-3.5" /> 重置</button>
        <button v-if="!running" class="btn btn-primary" @click="runScript"><Play class="h-3.5 w-3.5" /> 执行</button>
        <button v-else class="btn btn-danger" @click="stop"><Square class="h-3.5 w-3.5" /> 中止</button>
      </div>
    </div>

    <RiskBanner
      level="mid"
      title="脚本会以你的身份执行设备命令"
      desc="脚本里的命令和你手动敲进 adb 的效果完全一致，包括擦除与刷写。只运行你读得懂的脚本。"
      :points="RISK_POINTS.scripts"
    />

    <div v-if="state.mode !== 'adb'" class="panel py-16 text-center text-[12px] text-muted">
      脚本需要设备处于 ADB 模式
    </div>

    <div v-else class="grid grid-cols-[1fr_260px] gap-4">
      <div class="panel overflow-hidden">
        <div class="panel-header">
          <span class="panel-title"><Terminal class="h-3.5 w-3.5" /> 脚本编辑</span>
          <span class="text-[10px] text-muted">自动保存到本机</span>
        </div>
        <textarea
          v-model="source"
          class="h-[430px] w-full resize-none bg-ink-950 p-3 font-mono text-[11.5px] leading-[1.7] text-slate-200 outline-none"
          spellcheck="false"
          @input="persist"
        />
      </div>

      <div class="space-y-3">
        <div class="panel">
          <div class="panel-header"><span class="panel-title"><Wand2 class="h-3.5 w-3.5" /> 示例</span></div>
          <div class="p-2">
            <button
              v-for="s in samples"
              :key="s.label"
              class="block w-full rounded px-2 py-1.5 text-left text-[10.5px] text-slate-400 hover:bg-ink-800 hover:text-slate-200"
              @click="useSample(s.code)"
            >
              {{ s.label }}
            </button>
          </div>
        </div>

        <div class="panel">
          <div class="panel-header">
            <span class="panel-title"><BookOpen class="h-3.5 w-3.5" /> 语法范围</span>
            <button class="text-[10px] text-accent" @click="showHelp = !showHelp">{{ showHelp ? '收起' : '展开' }}</button>
          </div>
          <div v-if="showHelp" class="space-y-1 p-3 text-[10px] leading-relaxed text-muted">
            <div><span class="mono text-slate-300">echo</span> / <span class="mono text-slate-300">rem</span> / <span class="mono text-slate-300">::</span></div>
            <div><span class="mono text-slate-300">set</span> / <span class="mono text-slate-300">set /a</span> / <span class="mono text-slate-300">%var%</span></div>
            <div><span class="mono text-slate-300">goto</span> / 标签 <span class="mono text-slate-300">:name</span></div>
            <div><span class="mono text-slate-300">if errorlevel</span> / <span class="mono text-slate-300">if exist</span> / <span class="mono text-slate-300">if "a"=="b"</span></div>
            <div><span class="mono text-slate-300">for /l %%i in (1,1,5) do</span></div>
            <div><span class="mono text-slate-300">timeout /t</span> / <span class="mono text-slate-300">choice /t</span> / <span class="mono text-slate-300">pause</span></div>
            <div><span class="mono text-slate-300">adb shell / push / pull / install / reboot</span></div>
            <div class="pt-1 text-warn">不支持 for /f、管道、重定向、call 与外部程序。</div>
          </div>
          <div v-else class="px-3 py-2 text-[10px] text-muted">echo · set · if · for /l · goto · timeout · adb</div>
        </div>
      </div>
    </div>
  </div>
</template>
