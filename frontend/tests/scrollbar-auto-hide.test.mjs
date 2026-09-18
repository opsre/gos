import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const styleURL = new URL('../src/style.css', import.meta.url)
const utilURL = new URL('../src/utils/scrollbar-auto-hide.ts', import.meta.url)
const appURL = new URL('../src/App.vue', import.meta.url)
const layoutURL = new URL('../src/layouts/AppLayout.vue', import.meta.url)

const style = readFileSync(styleURL, 'utf8')
const util = readFileSync(utilURL, 'utf8')
const app = readFileSync(appURL, 'utf8')
const layout = readFileSync(layoutURL, 'utf8')

test('global scrollbars use a transparent track and a translucent thumb', () => {
  assert.match(
    style,
    /\*::-webkit-scrollbar-track,\s*\*::-webkit-scrollbar-corner \{\s*background: transparent;/,
    'the groove/track must disappear so tables stop showing a grey rail',
  )
  assert.match(
    style,
    /\*::-webkit-scrollbar-thumb \{[\s\S]*background-color: rgba\(100, 116, 139, 0\.16\)[\s\S]*background-clip: content-box/,
    'the idle thumb should be semi-transparent and slim',
  )
})

test('scrollbars fade in while scrolling and fade back when idle', () => {
  assert.match(
    style,
    /\*:hover::-webkit-scrollbar-thumb,\s*\*::-webkit-scrollbar-thumb:hover,\s*\.is-scrolling::-webkit-scrollbar-thumb \{\s*background-color: rgba\(100, 116, 139, 0\.46\);/,
    'hovering the rail or scrolling should reveal the thumb',
  )
  assert.match(
    util,
    /const SCROLLING_CLASS = 'is-scrolling'[\s\S]*const IDLE_DELAY_MS = 900/,
    'the reveal state must expire after a short idle delay',
  )
  assert.match(
    util,
    /document\.addEventListener\('scroll', markScrolling, \{ capture: true, passive: true \}\)/,
    'scroll events do not bubble, so the listener has to be installed in the capture phase',
  )
  assert.match(
    util,
    /scroller\.classList\.add\(SCROLLING_CLASS\)[\s\S]*window\.setTimeout\(\(\) => \{\s*scroller\.classList\.remove\(SCROLLING_CLASS\)/,
    'the class should be removed once scrolling stops',
  )
  assert.match(
    app,
    /import \{ installScrollbarAutoHide \} from '\.\/utils\/scrollbar-auto-hide'[\s\S]*onMounted\(\(\) => \{\s*installScrollbarAutoHide\(\)/,
    'the root component should install the watcher once for the whole app',
  )
})

test('the dark sider keeps its own translucent rail', () => {
  assert.doesNotMatch(
    layout,
    /linear-gradient\(180deg, rgba\(56, 189, 248, 0\.7\)/,
    'the old bright gradient thumb should be replaced by the shared translucent style',
  )
  assert.match(
    layout,
    /\.sider-menu::-webkit-scrollbar-thumb \{[\s\S]*background-color: rgba\(226, 232, 240, 0\.16\)/,
    'the sider thumb should be translucent against the dark background',
  )
  assert.match(
    layout,
    /\.sider-menu:hover::-webkit-scrollbar-thumb,\s*\.sider-menu\.is-scrolling::-webkit-scrollbar-thumb \{\s*background-color: rgba\(226, 232, 240, 0\.42\);/,
    'the sider thumb should follow the same reveal behaviour',
  )
})
