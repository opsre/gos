import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const style = readFileSync(new URL('../src/style.css', import.meta.url), 'utf8')
const templateSource = readFileSync(new URL('../src/views/release/ReleaseTemplateView.vue', import.meta.url), 'utf8')

const OPAQUE_HEX = /^#[0-9a-f]{6}$/i

// 固定列底色必须是「行底色的不透明等价值」：
// 行底色是半透明玻璃色（-table-glass-row），固定列若用半透明白就会比行更白，
// 在每行右侧形成一条竖条色差；若用纯白同理。所以这里钉住三个不透明取值。
function cssVar(name) {
  const match = style.match(new RegExp(`--${name}:\\s*([^;]+);`))
  assert.ok(match, `expected :root variable --${name}`)
  return match[1].trim()
}

function backgroundOf(selectorFragment) {
  const start = style.indexOf(selectorFragment)
  assert.ok(start >= 0, `expected a rule containing "${selectorFragment}"`)
  const open = style.indexOf('{', start)
  const close = style.indexOf('}', open)
  const block = style.slice(open + 1, close)
  const match = block.match(/background:\s*([^;]+);/)
  assert.ok(match, `expected a background declaration in the rule for "${selectorFragment}"`)
  return match[1].replace(/!important/gi, '').trim()
}

test('固定操作列常规底色不透明，且与行底色同色', () => {
  const fixed = cssVar('table-glass-fixed-cell')
  assert.match(fixed, OPAQUE_HEX, `固定列底色必须是不透明色，当前为 ${fixed}`)
  assert.notEqual(
    fixed.toLowerCase(),
    '#ffffff',
    '固定列不能用纯白：行底色是玻璃白，纯白会让整列看起来是一个更白的色块',
  )
  assert.equal(
    fixed.toLowerCase(),
    '#f8f9fe',
    '固定列底色应等于行底色 rgba(255,255,255,0.18) 叠加后的不透明等价值',
  )
})

test('固定操作列悬停底色是行悬停的不透明等价值', () => {
  const hover = backgroundOf('.ant-table-cell-row-hover.ant-table-cell-fix-right')
  assert.match(hover, OPAQUE_HEX, `固定列悬停底色必须是不透明色，当前为 ${hover}`)
  assert.equal(
    hover.toLowerCase(),
    '#fbfcfe',
    '固定列悬停底色应等于行悬停 rgba(255,255,255,0.58) 叠加后的不透明等价值',
  )
})

test('固定操作列选中底色是行选中的不透明等价值', () => {
  const selected = backgroundOf('tr.ant-table-row-selected > td.ant-table-cell-fix-right')
  assert.match(selected, OPAQUE_HEX, `固定列选中底色必须是不透明色，当前为 ${selected}`)
  assert.equal(
    selected.toLowerCase(),
    '#f1f7ff',
    '固定列选中底色应等于行选中 rgba(239,246,255,0.72) 叠加后的不透明等价值',
  )
})

test('固定操作列的底色规则都不允许半透明（否则会透出横向滚动的内容）', () => {
  for (const fragment of [
    '.ant-table-cell-row-hover.ant-table-cell-fix-right',
    'tr.ant-table-row-selected > td.ant-table-cell-fix-right',
  ]) {
    const background = backgroundOf(fragment)
    assert.doesNotMatch(
      background,
      /rgba\([^)]*,\s*(0?\.\d+|0)\s*\)/,
      `固定列底色不能带透明度：${background}`,
    )
  }
})

test('模板页自己的固定列底色与全局保持一致', () => {
  const match = templateSource.match(/\.release-template-table :deep\(\.ant-table-cell-fix-right\)\s*\{([^}]*)\}/)
  assert.ok(match, 'expected the release template page fixed column rule')
  const background = (match[1].match(/background:\s*([^;]+);/) || [])[1]
  assert.ok(background, 'expected a background declaration on the template page fixed column rule')
  assert.equal(
    background.replace(/!important/gi, '').trim().toLowerCase(),
    '#f8f9fe',
    '模板页的固定列底色要和全局一致，避免它一旦生效就退回更白的色块',
  )
})
