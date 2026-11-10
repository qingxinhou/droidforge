/**
 * 导航的唯一数据源。
 *
 * 侧栏、命令面板（Ctrl K）、仪表盘的「功能总览」、设置页的「启动页」下拉
 * 全部从这里读，增删页面只改这一个文件。
 *
 * 分组维度刻意不用技术栈（旧版是 设备/刷机/系统/工具，边界互相打架），
 * 改用「这一步会动到什么」：
 *
 *   设备  —— 连着 ADB 就能做，随时可中止
 *   调校  —— 只改系统参数，能改回去
 *   刷机  —— 会写分区，做错不可逆
 *   进阶  —— Root 与批量化，风险最高
 *
 * 越往下越危险，侧栏顺序即风险顺序。
 */
import type { Component } from 'vue'
import {
  AppWindow,
  BatteryCharging,
  Boxes,
  Cpu,
  Download,
  Files,
  HardDrive,
  Layers,
  LayoutDashboard,
  Monitor,
  Package,
  Power,
  ScrollText,
  Settings,
  ShieldCheck,
  SlidersHorizontal,
  Terminal,
  Upload,
} from 'lucide-vue-next'

/** 该页面需要的设备状态。 */
export type NavNeeds = 'any' | 'adb' | 'fastboot'

export interface NavItem {
  path: string
  label: string
  /** 一句话说明这个页面能干什么，用于功能总览与搜索结果 */
  desc: string
  icon: Component
  needs: NavNeeds
  /** 会写分区或改动系统关键状态，侧栏与总览上给个提示标记 */
  risk?: boolean
  /** 搜索别名：英文名、缩写、常见叫法 */
  keywords?: string[]
}

export interface NavGroup {
  id: string
  label: string
  /** 分组依据，鼠标悬停分组标题时显示 */
  hint: string
  items: NavItem[]
}

export const HOME: NavItem = {
  path: '/',
  label: '仪表盘',
  desc: '设备状态、构建信息与常用入口',
  icon: LayoutDashboard,
  needs: 'any',
  keywords: ['home', 'overview', '概览'],
}

export const SETTINGS: NavItem = {
  path: '/settings',
  label: '设置',
  desc: '工具链路径、目录、主题、免责声明',
  icon: Settings,
  needs: 'any',
  keywords: ['config', '配置', '首选项'],
}

export const NAV_GROUPS: NavGroup[] = [
  {
    id: 'device',
    label: '设备',
    hint: '连着 ADB 就能做，随时可以中止',
    items: [
      {
        path: '/apps',
        label: '应用管理',
        desc: '安装、卸载、冻结、提取 APK',
        icon: AppWindow,
        needs: 'adb',
        keywords: ['apk', 'install', 'uninstall', 'pm', '冻结'],
      },
      {
        path: '/files',
        label: '文件传输',
        desc: '推送与拉取文件、浏览设备目录',
        icon: Files,
        needs: 'adb',
        keywords: ['push', 'pull', 'adb', '目录'],
      },
      {
        path: '/terminal',
        label: 'ADB 终端',
        desc: '直接执行 adb shell 与常用指令',
        icon: Terminal,
        needs: 'adb',
        keywords: ['shell', 'command', '命令', '命令行'],
      },
      {
        path: '/reboot',
        label: '重启与模式',
        desc: '进系统、Recovery、Bootloader、fastbootd',
        icon: Power,
        needs: 'adb',
        keywords: ['reboot', 'recovery', 'bootloader', '关机'],
      },
    ],
  },
  {
    id: 'tune',
    label: '调校',
    hint: '只改系统参数，改坏了可以改回去',
    items: [
      {
        path: '/display',
        label: '显示控制',
        desc: '分辨率、DPI、刷新率、旋转、锁屏',
        icon: Monitor,
        needs: 'adb',
        keywords: ['dpi', 'resolution', '刷新率', 'wm', 'density'],
      },
      {
        path: '/sysui',
        label: '界面定制',
        desc: '状态栏图标、快捷开关、沉浸模式',
        icon: SlidersHorizontal,
        needs: 'adb',
        keywords: ['statusbar', '状态栏', '导航栏', '沉浸'],
      },
      {
        path: '/battery',
        label: '电池管理',
        desc: '电量与温度曲线、充电参数模拟',
        icon: BatteryCharging,
        needs: 'adb',
        keywords: ['battery', '电量', '温度', '充电'],
      },
      {
        path: '/performance',
        label: '性能监控',
        desc: 'CPU 频率、调度器与实际负载',
        icon: Cpu,
        needs: 'adb',
        keywords: ['cpu', 'governor', '频率', '负载'],
      },
    ],
  },
  {
    id: 'flash',
    label: '刷机',
    hint: '会写分区，做错不可逆',
    items: [
      {
        path: '/download',
        label: '固件下载',
        desc: '多线程、断点续传下载 OTA 整包',
        icon: Download,
        needs: 'any',
        keywords: ['download', 'ota', '断点续传', '多线程', '直链'],
      },
      {
        path: '/extract',
        label: '刷机包提取',
        desc: '本地 OTA 包或直链，导出分区镜像',
        icon: Package,
        needs: 'any',
        keywords: ['payload', 'ota', 'rom', '解包', 'img'],
      },
      {
        path: '/super',
        label: '超级分区',
        desc: '拆分 super、散包合成 super.img、生成线刷脚本',
        icon: Boxes,
        needs: 'any',
        keywords: ['super', 'lp_metadata', '动态分区', '合并', '拆分', '逻辑分区'],
      },
      {
        path: '/flash',
        label: '刷入镜像',
        desc: 'fastboot 单分区刷写',
        icon: Upload,
        needs: 'fastboot',
        risk: true,
        keywords: ['fastboot', 'flash', 'boot', '刷写'],
      },
      {
        path: '/lineflash',
        label: '线刷整包',
        desc: '按脚本顺序刷完整套镜像',
        icon: Layers,
        needs: 'fastboot',
        risk: true,
        keywords: ['线刷', '整包', 'flash-all', '批量'],
      },
      {
        path: '/partitions',
        label: '分区管理',
        desc: '分区表、槽位、备份与擦除',
        icon: HardDrive,
        needs: 'any',
        risk: true,
        keywords: ['partition', 'slot', 'erase', '擦除', '槽位'],
      },
    ],
  },
  {
    id: 'advanced',
    label: '进阶',
    hint: 'Root 与批量化，风险最高',
    items: [
      {
        path: '/root',
        label: 'Root 方案',
        desc: 'Magisk / KernelSU / APatch 打补丁',
        icon: ShieldCheck,
        needs: 'adb',
        risk: true,
        keywords: ['magisk', 'kernelsu', 'apatch', 'su', '权限'],
      },
      {
        path: '/scripts',
        label: '控机脚本',
        desc: 'BAT 子集脚本批量执行 ADB 指令',
        icon: ScrollText,
        needs: 'adb',
        risk: true,
        keywords: ['bat', 'script', '脚本', '批量'],
      },
    ],
  },
]

/** 平铺后的全部页面，命令面板与启动页下拉用。 */
export const ALL_ITEMS: NavItem[] = [HOME, ...NAV_GROUPS.flatMap((g) => g.items), SETTINGS]

/** 按 path 反查，页面头部与面包屑可用。 */
export function findNav(path: string): NavItem | undefined {
  return ALL_ITEMS.find((i) => i.path === path)
}

/** needs 的可读文案。 */
export const NEEDS_LABEL: Record<NavNeeds, string> = {
  any: '无需设备',
  adb: '需 ADB',
  fastboot: '需 Fastboot',
}
