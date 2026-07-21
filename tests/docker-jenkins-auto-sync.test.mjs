import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const entrypointSource = readFileSync(new URL('../docker/entrypoint.sh', import.meta.url), 'utf8')
const readJSON = (path) => JSON.parse(readFileSync(new URL(path, import.meta.url), 'utf8'))

test('single-container runtime enables Jenkins pipeline auto sync by default', () => {
  assert.match(
    entrypointSource,
    /"auto_sync_enabled": env_bool\("GOS_JENKINS_AUTO_SYNC_ENABLED", True\)/,
  )
})

test('shipped Docker configs enable Jenkins pipeline auto sync by default', () => {
  const configs = [
    readJSON('../configs/config.container.template.json'),
    readJSON('../configs/config.docker.json'),
    readJSON('../configs/config.docker.prod.json'),
  ]

  for (const config of configs) {
    assert.equal(config.jenkins.auto_sync_enabled, true)
    assert.ok(config.jenkins.auto_sync_interval_sec > 0)
  }
})
