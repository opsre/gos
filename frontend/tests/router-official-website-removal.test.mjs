import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const routerURL = new URL('../src/router/index.ts', import.meta.url)
const source = readFileSync(routerURL, 'utf8')

test('root route no longer serves the official website page', () => {
  const firstRoute = source.match(/routes:\s*\[\s*(\{[\s\S]*?\n\s*\},)/)?.[1] || ''

  assert.doesNotMatch(source, /OfficialWebsiteView|official-website|views\/marketing/, 'router should not reference the official website page')
  assert.match(firstRoute, /path:\s*'\/'/, 'first route should target root')
  assert.match(firstRoute, /redirect:\s*'\/applications'/, 'root route should redirect into the application workbench')
  assert.doesNotMatch(firstRoute, /meta:\s*\{[^}]*public:\s*true/, 'root route should not remain public')
})
