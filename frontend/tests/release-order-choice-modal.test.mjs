import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viewURL = new URL('../src/views/release/ReleaseOrderCreateView.vue', import.meta.url)
const choiceUtilURL = new URL('../src/utils/executor-param-choice.ts', import.meta.url)
const source = readFileSync(viewURL, 'utf8')
// 选项解析已抽成共享工具（建单页与自动化配置页共用），解析规则改读工具文件
const choiceUtil = readFileSync(choiceUtilURL, 'utf8')

test('release order multi choice dropdown exposes expanded checkbox modal', () => {
  assert.match(source, /FullscreenOutlined/, 'expanded choice action should use an icon')
  assert.match(
    source,
    /handleChoiceDeselectAll\(param\)[\s\S]*openExpandedChoiceModal\(item\.scope,\s*param\)/,
    'expanded action should sit next to the existing deselect-all action',
  )
  assert.match(
    source,
    /<a-modal[\s\S]*v-model:open="expandedChoiceState\.open"[\s\S]*class="expanded-choice-modal"/,
    'page should render a dedicated expanded choice modal',
  )
  assert.match(
    source,
    /v-model:value="expandedChoiceState\.keyword"[\s\S]*placeholder="搜索选项"/,
    'expanded choice modal should provide keyword search',
  )
  assert.match(
    source,
    /v-for="option in filteredExpandedChoiceOptions"[\s\S]*<a-checkbox[\s\S]*:checked="isExpandedChoiceChecked\(option\.value\)"[\s\S]*@change="handleExpandedChoiceToggle\(option\.value,\s*\$event\)"/,
    'expanded choice modal should render all filtered options as checkboxes',
  )
  assert.match(
    source,
    /expandedChoiceSelectedCount[\s\S]*expandedChoiceTotalCount/,
    'modal should show selected and total option counts for large lists',
  )
  assert.match(
    choiceUtil,
    /objectRaw\.value[\s\S]*objectRaw\.label[\s\S]*\{ label: label \|\| value, value \}/,
    'choice options should preserve a display label separately from the submitted value',
  )
  assert.match(
    choiceUtil,
    /parsed\.choiceOptions[\s\S]*parsed\.options[\s\S]*parsed\.choices/,
    'choice metadata should prefer labeled options before falling back to raw values',
  )
  assert.match(
    source,
    /import \{[\s\S]*resolveChoiceMeta[\s\S]*\} from '\.\.\/\.\.\/utils\/executor-param-choice'/,
    'the create page should reuse the shared choice parser instead of keeping a private copy',
  )
})
