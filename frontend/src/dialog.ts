import { reactive } from 'vue'

export interface DialogOptions {
  title: string
  message: string
  danger?: boolean
  confirmText?: string
  cancelText?: string
  /** 风险点清单，仅 danger 时展示 */
  risks?: readonly string[]
  /** 是否要求勾选“我已了解风险”才能确认，默认 danger 时为 true */
  requireAck?: boolean
  /** 勾选框文案 */
  ackText?: string
}

export const dialogState = reactive({
  open: false,
  title: '',
  message: '',
  danger: false,
  confirmText: '确认',
  cancelText: '取消',
  risks: [] as string[],
  requireAck: false,
  ackText: '',
  ack: false,
  _resolve: null as null | ((v: boolean) => void),
})

const DEFAULT_ACK = '我已核对设备、目标与文件，理解该操作可能造成不可逆损坏，并自愿承担全部后果'

export function confirmDialog(opts: DialogOptions): Promise<boolean> {
  const danger = !!opts.danger
  dialogState.title = opts.title
  dialogState.message = opts.message
  dialogState.danger = danger
  dialogState.confirmText = opts.confirmText ?? '确认'
  dialogState.cancelText = opts.cancelText ?? '取消'
  dialogState.risks = [...(opts.risks ?? [])]
  dialogState.requireAck = opts.requireAck ?? (danger && dialogState.risks.length > 0)
  dialogState.ackText = opts.ackText ?? DEFAULT_ACK
  dialogState.ack = false
  dialogState.open = true
  return new Promise<boolean>((resolve) => {
    dialogState._resolve = resolve
  })
}

export function setAck(v: boolean) {
  dialogState.ack = v
}

export function resolveDialog(value: boolean) {
  // 未勾选风险确认时不允许确认（防止误触 Enter 或点穿）
  if (value && dialogState.requireAck && !dialogState.ack) return
  dialogState.open = false
  const r = dialogState._resolve
  dialogState._resolve = null
  r?.(value)
}
