import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const formURL = new URL('../src/views/application/ApplicationForm.vue', import.meta.url)

test('application language selector includes C#', async () => {
  const source = await readFile(formURL, 'utf8')

  assert.match(source, /\{\s*label:\s*'c#',\s*value:\s*'c#'\s*\}/)
})
