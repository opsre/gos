import dayjs from 'dayjs'

// 后端只回传简短错误信息（如 credential not configured），这里补齐成用户可读的中文提示。
export function recentCommitErrorText(raw: string): string {
  const text = String(raw || '').trim()
  if (!text) {
    return '暂时无法获取最近提交'
  }
  const lowered = text.toLowerCase()
  if (lowered.includes('credential') || text.includes('凭证')) {
    return '未配置 Git 凭证'
  }
  return text
}

// 提交时间同时给出相对时间与绝对时间，便于快速判断新旧并核对具体时刻。
export function formatRecentCommitTime(value: string): string {
  if (!value) {
    return '-'
  }
  const parsed = dayjs(value)
  if (!parsed.isValid()) {
    return '-'
  }
  return `${formatRelativeCommitTime(parsed)} · ${parsed.format('YYYY-MM-DD HH:mm:ss')}`
}

function formatRelativeCommitTime(target: dayjs.Dayjs): string {
  const diffMs = dayjs().valueOf() - target.valueOf()
  const minute = 60_000
  if (diffMs < minute) {
    return '刚刚'
  }
  const hour = 60 * minute
  if (diffMs < hour) {
    return `${Math.floor(diffMs / minute)} 分钟前`
  }
  const day = 24 * hour
  if (diffMs < day) {
    return `${Math.floor(diffMs / hour)} 小时前`
  }
  if (diffMs < 30 * day) {
    return `${Math.floor(diffMs / day)} 天前`
  }
  return target.format('YYYY-MM-DD')
}
