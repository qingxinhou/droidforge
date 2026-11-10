<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import {
  Download,
  FolderOpen,
  Save,
  RefreshCw,
  CheckCircle2,
  AlertTriangle,
  Info,
  ExternalLink,
  FileUp,
  FileDown,
  Monitor,
  FileText,
  ShieldAlert,
  Moon,
  Sun,
} from 'lucide-vue-next'
import { api } from '../api'
import { state, refresh, run, toast, applyUIScale, setTheme } from '../store'
import { confirmDialog } from '../dialog'
import { DISCLAIMER_VERSION, legalState } from '../legal'
import { ALL_ITEMS } from '../nav'
import type { ThemeMode } from '../theme'
import type { Config } from '../types'

const cfg = ref<Config | null>(null)
const downloading = ref(false)

const pages = ALL_ITEMS.map((i) => ({ path: i.path, label: i.label }))

const themes = [
  { value: 'dark' as ThemeMode, label: '深色', icon: Moon },
  { value: 'light' as ThemeMode, label: '浅色', icon: Sun },
  { value: 'system' as ThemeMode, label: '跟随系统', icon: Monitor },
]

onMounted(() => {
  if (state.env?.config) cfg.value = { ...state.env.config }
})

// 顶栏的主题按钮改的是全局配置，这里要跟着同步，否则选中态会停在旧值
watch(
  () => state.env?.config.theme,
  (v) => {
    if (cfg.value && v) cfg.value.theme = v
  }
)

/** 主题贴换立刻生效并落盘，不用等「保存」。 */
function setThemeMode(mode: ThemeMode) {
  if (!cfg.value) return
  cfg.value.theme = mode
  setTheme(mode)
}

/** 把 RFC3339 转成本地可读时间。 */
function fmtAccepted(s?: string): string {
  if (!s) return '未同意'
  const d = new Date(s)
  if (Number.isNaN(d.getTime())) return s
  return d.toLocaleString('zh-CN', { hour12: false })
}

async function resetDisclaimer() {
  const ok = await confirmDialog({
    title: '撤销免责声明同意',
    message:
      '撤销后本程序会立即退出，下次启动需要重新阅读并同意全部条款。\n\n这不会卸载软件，也不会影响已保存的配置。',
    confirmText: '撤销并退出',
  })
  if (!ok) return
  try {
    await api.resetDisclaimer()
  } catch (e: any) {
    toast('err', e?.message ?? String(e))
    return
  }
  window.setTimeout(() => (window as any).runtime?.Quit?.(), 250)
}

async function pickExe(which: 'adb' | 'fastboot') {
  if (!cfg.value) return
  const p = await api.pickFile(`选择 ${which} 可执行文件`, which === 'adb' ? 'adb.exe;adb;*.*' : 'fastboot.exe;fastboot;*.*')
  if (!p) return
  if (which === 'adb') cfg.value.customAdb = p
  else cfg.value.customFastboot = p
}

async function autoDetect() {
  if (!cfg.value) return
  cfg.value.customAdb = ''
  cfg.value.customFastboot = ''
  await api.setToolsPaths('', '')
  await refresh()
  if (state.env?.config) cfg.value = { ...state.env.config }
  toast('ok', state.env?.tools.hint ?? '已重新检测')
}

async function pickDir(field: 'cacheDir' | 'outputDir' | 'pushDir') {
  if (!cfg.value) return
  const p = await api.pickDirectory('选择目录')
  if (p) cfg.value[field] = p
}

async function save() {
  if (!cfg.value) return
  await run('配置已保存', () => api.saveConfig(cfg.value!))
  if (state.env?.config) cfg.value = { ...state.env.config }
  applyUIScale(cfg.value?.uiScale ?? 1)
}

async function setScale(v: number) {
  if (!cfg.value) return
  cfg.value.uiScale = v
  applyUIScale(v)
  await api.saveConfig(cfg.value)
  toast('ok', `界面缩放 ${Math.round(v * 100)}%`)
}

async function exportConfig() {
  if (!cfg.value) return
  const p = await api.saveTextFile('droidforge-config.json', JSON.stringify(cfg.value, null, 2))
  if (p) {
    toast('ok', '配置已导出')
    await api.revealInFolder(p)
  }
}

async function importConfig() {
  if (!cfg.value) return
  const p = await api.pickFile('导入配置', '*.json')
  if (!p) return
  try {
    const raw = await api.readTextFile(p)
    const parsed = JSON.parse(raw) as Partial<Config>
    cfg.value = { ...cfg.value, ...parsed }
    await api.saveConfig(cfg.value)
    applyUIScale(cfg.value.uiScale ?? 1)
    toast('ok', '配置已导入')
    await refresh()
    if (state.env?.config) cfg.value = { ...state.env.config }
  } catch (e: any) {
    toast('err', '导入失败：' + (e?.message ?? String(e)))
  }
}

async function downloadTools() {
  downloading.value = true
  try {
    await run('platform-tools 安装完成', () => api.downloadPlatformTools())
    await refresh()
  } finally {
    downloading.value = false
  }
}

const tools = () => state.env?.tools
</script>

<template>
  <div v-if="cfg" class="p-5">
    <div class="mb-4 flex items-end justify-between">
      <div>
        <h1 class="text-[15px] font-medium text-slate-100">设置</h1>
        <p class="mt-1 text-[11.5px] text-muted">平台工具路径、下载加速与输出目录。</p>
      </div>
      <button class="btn btn-primary" @click="save">
        <Save class="h-3.5 w-3.5" /> 保存配置
      </button>
    </div>

    <div class="grid grid-cols-12 gap-4">
      <div class="col-span-8 flex flex-col gap-4">
        <!-- 平台工具 -->
        <div class="panel">
          <div class="panel-head">
            <span class="panel-title">平台工具</span>
            <div class="flex items-center gap-1.5 text-[10.5px]">
              <CheckCircle2 v-if="tools()?.ready" class="h-3 w-3 text-ok" />
              <AlertTriangle v-else class="h-3 w-3 text-warn" />
              <span :class="tools()?.ready ? 'text-ok' : 'text-warn'">{{ tools()?.hint }}</span>
            </div>
          </div>
          <div class="space-y-3 p-4">
            <div>
              <label class="label">adb 路径</label>
              <div class="flex gap-2">
                <input v-model="cfg.customAdb" class="input mono" placeholder="留空自动检测" />
                <button class="btn" @click="pickExe('adb')"><FolderOpen class="h-3.5 w-3.5" /> 选择</button>
              </div>
              <div class="mt-1 text-[10px] text-muted">{{ tools()?.adbVersion || '未检测到' }}</div>
            </div>
            <div>
              <label class="label">fastboot 路径</label>
              <div class="flex gap-2">
                <input v-model="cfg.customFastboot" class="input mono" placeholder="留空自动检测" />
                <button class="btn" @click="pickExe('fastboot')"><FolderOpen class="h-3.5 w-3.5" /> 选择</button>
              </div>
              <div class="mt-1 text-[10px] text-muted">{{ tools()?.fastbootVersion || '未检测到' }}</div>
            </div>
            <div class="flex gap-2">
              <button class="btn" @click="autoDetect">
                <RefreshCw class="h-3.5 w-3.5" /> 重新自动检测
              </button>
              <button class="btn btn-primary" :disabled="downloading" @click="downloadTools">
                <Download class="h-3.5 w-3.5" /> {{ downloading ? '下载中…' : '下载官方 platform-tools' }}
              </button>
            </div>
            <div class="text-[10px] leading-relaxed text-muted">
              下载来源为 Google 官方仓库。也可以把 <span class="mono">platform-tools</span> 文件夹放到程序同目录，实现绿色版免配置。
            </div>
          </div>
        </div>

        <!-- 下载与目录 -->
        <div class="panel">
          <div class="panel-head"><span class="panel-title">下载与目录</span></div>
          <div class="space-y-3 p-4">
            <div>
              <label class="label">GitHub 加速前缀（可选）</label>
              <input v-model="cfg.mirror" class="input mono" placeholder="例如 https://ghfast.top" />
              <div class="mt-1 text-[10px] text-muted">
                仅用于下载 Root 方案的官方安装包；留空则直连 GitHub。
              </div>
            </div>
            <div>
              <label class="label">缓存目录（补丁工具 / 解包临时文件）</label>
              <div class="flex gap-2">
                <input v-model="cfg.cacheDir" class="input mono" />
                <button class="btn btn-icon" @click="pickDir('cacheDir')"><FolderOpen class="h-3.5 w-3.5" /></button>
              </div>
            </div>
            <div>
              <label class="label">默认输出目录（提取的镜像 / 截图）</label>
              <div class="flex gap-2">
                <input v-model="cfg.outputDir" class="input mono" />
                <button class="btn btn-icon" @click="pickDir('outputDir')"><FolderOpen class="h-3.5 w-3.5" /></button>
                <button class="btn" @click="api.openPath(cfg.outputDir)">打开</button>
              </div>
            </div>
          </div>
        </div>

        <!-- 行为 -->
        <div class="panel">
          <div class="panel-head"><span class="panel-title">行为</span></div>
          <div class="divide-y divide-ink-800">
            <label class="flex cursor-pointer items-center justify-between px-4 py-3">
              <div>
                <div class="text-[12px] text-slate-200">自动刷新设备列表</div>
                <div class="text-[10px] text-muted">每 4 秒扫描一次 adb / fastboot 设备</div>
              </div>
              <input v-model="cfg.autoRefresh" type="checkbox" class="h-4 w-4 accent-accent" />
            </label>
            <label class="flex cursor-pointer items-center justify-between px-4 py-3">
              <div>
                <div class="text-[12px] text-slate-200">危险操作二次确认</div>
                <div class="text-[10px] text-muted">刷机、擦除前弹出确认框</div>
              </div>
              <input v-model="cfg.confirmDanger" type="checkbox" class="h-4 w-4 accent-accent" />
            </label>
            <label class="flex cursor-pointer items-center justify-between px-4 py-3">
              <div>
                <div class="text-[12px] text-slate-200">安装时默认覆盖</div>
                <div class="text-[10px] text-muted">安装 APK 默认带 -r 参数</div>
              </div>
              <input v-model="cfg.installReinstall" type="checkbox" class="h-4 w-4 accent-accent" />
            </label>
            <label class="flex cursor-pointer items-center justify-between px-4 py-3">
              <div>
                <div class="text-[12px] text-slate-200">隐藏设备标识</div>
                <div class="text-[10px] text-muted">顶栏与信息卡中的序列号显示为掩码（截图分享用）</div>
              </div>
              <input v-model="cfg.hideIds" type="checkbox" class="h-4 w-4 accent-accent" />
            </label>
          </div>
        </div>

        <!-- 界面 -->
        <div class="panel">
          <div class="panel-head">
            <span class="panel-title"><Monitor class="h-3.5 w-3.5" /> 界面</span>
          </div>
          <div class="space-y-4 p-4">
            <div>
              <label class="label">主题</label>
              <div class="flex rounded border border-ink-600 bg-ink-900 p-0.5">
                <button
                  v-for="t in themes"
                  :key="t.value"
                  class="flex flex-1 items-center justify-center gap-1.5 rounded-sm px-2 py-1.5 text-[11.5px] transition-colors"
                  :class="
                    cfg.theme === t.value
                      ? 'bg-accent/15 text-accent'
                      : 'text-muted hover:text-slate-300'
                  "
                  @click="setThemeMode(t.value)"
                >
                  <component :is="t.icon" class="h-3.5 w-3.5" />
                  {{ t.label }}
                </button>
              </div>
              <div class="mt-1 text-[10px] text-muted">
                顶栏右侧的按钮可以在深色 / 浅色之间快速切换。
              </div>
            </div>
            <div class="grid grid-cols-2 gap-4">
              <div>
                <label class="label">默认启动页</label>
                <select v-model="cfg.startPage" class="input">
                  <option v-for="p in pages" :key="p.path" :value="p.path">{{ p.label }}</option>
                </select>
              </div>
              <div>
                <label class="label">CPU 刷新间隔</label>
                <select v-model.number="cfg.cpuIntervalMs" class="input">
                  <option :value="500">500 ms</option>
                  <option :value="1000">1 秒</option>
                  <option :value="1500">1.5 秒</option>
                  <option :value="2000">2 秒</option>
                  <option :value="5000">5 秒</option>
                </select>
              </div>
            </div>
            <div>
              <label class="label">界面缩放</label>
              <div class="flex gap-2">
                <button
                  v-for="v in [0.9, 1, 1.1, 1.25]"
                  :key="v"
                  class="btn flex-1"
                  :class="Math.abs((cfg.uiScale || 1) - v) < 0.001 ? 'btn-primary' : ''"
                  @click="setScale(v)"
                >
                  {{ Math.round(v * 100) }}%
                </button>
              </div>
              <div class="mt-1 text-[10px] text-muted">按比例缩放整个界面，适合高分屏或小窗口。</div>
            </div>
            <div>
              <label class="label">拉取 / 截图默认保存目录</label>
              <div class="flex gap-2">
                <input v-model="cfg.pushDir" class="input mono" />
                <button class="btn btn-icon" @click="pickDir('pushDir')"><FolderOpen class="h-3.5 w-3.5" /></button>
                <button class="btn" @click="api.openPath(cfg.pushDir)">打开</button>
              </div>
            </div>
          </div>
        </div>

        <!-- 配置备份 -->
        <div class="panel">
          <div class="panel-head"><span class="panel-title">配置备份</span></div>
          <div class="flex items-center gap-2 p-4">
            <button class="btn flex-1" @click="exportConfig"><FileDown class="h-3.5 w-3.5" /> 导出配置</button>
            <button class="btn flex-1" @click="importConfig"><FileUp class="h-3.5 w-3.5" /> 导入配置</button>
          </div>
          <div class="px-4 pb-3 text-[10px] leading-relaxed text-muted">
            导出的 JSON 含目录、快捷键与显示偏好，不含设备数据。换机器时导入即可恢复。
          </div>
        </div>
      </div>

      <!-- 关于 -->
      <div class="col-span-4 flex flex-col gap-4">
        <div class="panel">
          <div class="panel-head"><span class="panel-title">运行环境</span></div>
          <div class="px-4 py-3">
            <div class="kv"><span class="kv-k">操作系统</span><span class="kv-v">{{ state.env?.os }} / {{ state.env?.arch }}</span></div>
            <div class="kv"><span class="kv-k">应用版本</span><span class="kv-v mono">v{{ state.env?.appVersion }}</span></div>
            <div class="kv"><span class="kv-k">工具来源</span><span class="kv-v">{{ tools()?.source || '—' }}</span></div>
            <div class="kv"><span class="kv-k">adb</span><span class="kv-v mono text-[10px]">{{ tools()?.adbPath || '未找到' }}</span></div>
            <div class="kv"><span class="kv-k">fastboot</span><span class="kv-v mono text-[10px]">{{ tools()?.fastbootPath || '未找到' }}</span></div>
          </div>
        </div>

        <div class="panel p-4">
          <div class="mb-2 flex items-center gap-1.5 text-[11.5px] text-slate-300">
            <Info class="h-3.5 w-3.5 text-accent" /> 关于搞机工坊
          </div>
          <div class="text-[10.5px] leading-relaxed text-muted">
            开源 Android 搞机工具箱，聚合 ADB / Fastboot 常用操作、刷机包解析与 Root 补丁流程。
            所有命令在本机执行，不上传任何设备数据。
          </div>
          <button class="btn mt-3 w-full" @click="api.openUrl('https://github.com')">
            <ExternalLink class="h-3.5 w-3.5" /> 项目主页
          </button>
        </div>

        <div class="panel">
          <div class="panel-head"><span class="panel-title">风险与合规</span></div>
          <div class="px-4 py-3">
            <div class="kv">
              <span class="kv-k">免责声明版本</span>
              <span class="kv-v mono">
                v{{ cfg?.disclaimerVersion || 0 }} / v{{ DISCLAIMER_VERSION }}
              </span>
            </div>
            <div class="kv">
              <span class="kv-k">同意时间</span>
              <span class="kv-v text-[10.5px]">{{ fmtAccepted(cfg?.disclaimerAcceptedAt) }}</span>
            </div>
          </div>
          <div class="space-y-2 px-4 pb-3">
            <button class="btn w-full" @click="legalState.docOpen = true">
              <FileText class="h-3.5 w-3.5" /> 查看完整声明
            </button>
            <button class="btn btn-danger w-full" @click="resetDisclaimer">
              <ShieldAlert class="h-3.5 w-3.5" /> 撤销同意并退出
            </button>
          </div>
          <div class="border-t border-ink-800 px-4 py-3 text-[10px] leading-relaxed text-muted">
            本软件按“现状”提供，不对设备损坏、数据丢失或保修失效承担责任。
            撤销同意会立即退出程序，下次启动需重新阅读协议。
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
