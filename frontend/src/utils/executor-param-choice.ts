import type { ExecutorParamDef } from '../types/pipeline'

export interface SelectOption {
  label: string
  value: string
  description?: string
  disabled?: boolean
}

export interface ChoiceMeta {
  options: SelectOption[]
  multiple: boolean
  delimiter: string
}

export const defaultChoiceMeta: ChoiceMeta = {
  options: [],
  multiple: false,
  delimiter: ',',
}

function readChoiceDelimiter(meta: Record<string, unknown>) {
  const raw = [
    meta.multiSelectDelimiter,
    meta.multi_select_delimiter,
    meta.valueDelimiter,
    meta.delimiter,
    meta.separator,
  ]
  for (const item of raw) {
    const value = String(item || '').trim()
    if (value) {
      return value
    }
  }
  return ','
}

export function splitChoiceText(value: string): string[] {
  const text = value.trim()
  if (!text) {
    return []
  }
  if (text.includes('\n') || text.includes('\r')) {
    return text
      .replace(/\r\n/g, '\n')
      .replace(/\r/g, '\n')
      .split('\n')
      .map((item) => item.trim())
      .filter(Boolean)
  }
  if (text.includes(',')) {
    return text
      .split(',')
      .map((item) => item.trim())
      .filter(Boolean)
  }
  return [text]
}

export function dedupe(values: string[]) {
  const result: string[] = []
  const seen = new Set<string>()
  values.forEach((item) => {
    if (!item || seen.has(item)) {
      return
    }
    seen.add(item)
    result.push(item)
  })
  return result
}

function dedupeChoiceOptions(options: SelectOption[]) {
  const result: SelectOption[] = []
  const seen = new Set<string>()
  options.forEach((item) => {
    const value = String(item.value ?? '').trim()
    const label = String(item.label ?? '').trim() || value
    if (!value || seen.has(value)) {
      return
    }
    seen.add(value)
    result.push({ label, value })
  })
  return result
}

function readChoiceOptionText(raw: unknown) {
  return String(raw ?? '').trim()
}

export function normalizeChoiceOptions(raw: unknown): SelectOption[] {
  if (Array.isArray(raw)) {
    const options: SelectOption[] = []
    raw.forEach((item) => {
      if (item && typeof item === 'object') {
        const objectRaw = item as Record<string, unknown>
        const value = readChoiceOptionText(
          objectRaw.value ?? objectRaw.id ?? objectRaw.key ?? objectRaw.name ?? objectRaw.label,
        )
        const label = readChoiceOptionText(
          objectRaw.label ?? objectRaw.name ?? objectRaw.text ?? objectRaw.description ?? value,
        )
        if (value) {
          options.push({ label: label || value, value })
        }
        return
      }
      splitChoiceText(readChoiceOptionText(item)).forEach((value) => {
        options.push({ label: value, value })
      })
    })
    return dedupeChoiceOptions(options)
  }
  if (typeof raw === 'string') {
    return dedupeChoiceOptions(splitChoiceText(raw).map((value) => ({ label: value, value })))
  }
  if (raw && typeof raw === 'object') {
    const objectRaw = raw as Record<string, unknown>
    for (const key of ['choiceOptions', 'options', 'choices', 'choiceList', 'values', 'items', 'list', 'value']) {
      const options = normalizeChoiceOptions(objectRaw[key])
      if (options.length > 0) {
        return options
      }
    }
  }
  return []
}

export function normalizeChoiceValues(raw: unknown): string[] {
  return dedupe(normalizeChoiceOptions(raw).map((item) => item.value))
}

/**
 * resolveChoiceMeta 从执行器参数的 raw_meta 里解析出可选项、是否多选与多值分隔符。
 * 发布建单页和自动化配置页都按同一套规则渲染参数控件，避免两边表现不一致。
 */
export function resolveChoiceMeta(item: ExecutorParamDef): ChoiceMeta {
  const raw = String(item.raw_meta || '').trim()
  if (!raw) {
    return {
      ...defaultChoiceMeta,
      multiple: false,
    }
  }
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>
    const options = normalizeChoiceOptions(
      parsed.choiceOptions ??
        parsed.options ??
        parsed.choices ??
        parsed.choiceList ??
        parsed.values ??
        parsed.value ??
        parsed.items ??
        null,
    )

    const className = String(parsed._class || '').toLowerCase()
    const typeName = String(parsed.type || parsed.choiceType || parsed.ptype || '').toLowerCase()
    const delimiter = readChoiceDelimiter(parsed)
    const inferredMulti =
      Boolean(parsed.multiSelect) ||
      Boolean(parsed.multi_select) ||
      Boolean(parsed.isMulti) ||
      typeName.includes('multi') ||
      typeName.includes('checkbox') ||
      className.includes('multi')
    const multiple =
      item.single_select
        ? false
        : inferredMulti ||
          (item.param_type === 'choice' &&
            Boolean(delimiter && String(item.default_value || '').includes(delimiter) && options.length > 1))

    return {
      options,
      multiple,
      delimiter,
    }
  } catch {
    return defaultChoiceMeta
  }
}
