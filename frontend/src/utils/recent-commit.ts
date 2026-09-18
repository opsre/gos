import dayjs from 'dayjs'

// 提交信息在创建发布单时已落库（head_change_* 字段），前端只做展示格式化。
// 提交时间同时给出相对时间与绝对时间，便于快速判断新旧并核对具体时刻。
export function formatRecentCommitTime(value?: string | null): string {
  if (!value) {
    return '-'
  }
  const parsed = dayjs(value)
  if (!parsed.isValid()) {
    return '-'
  }
  return `${formatRelativeCommitTime(parsed)} · ${parsed.format('YYYY-MM-DD HH:mm:ss')}`
}

// 短 sha 统一截前 7 位，兼容后端直接回传完整 sha 或已截断的短 sha。
export function shortCommitSHA(value?: string | null): string {
  return String(value || '').trim().slice(0, 7)
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
