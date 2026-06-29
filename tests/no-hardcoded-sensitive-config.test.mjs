import test from 'node:test'
import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'

const blockedLiterals = [
  ['JSgc', '_w29Sd'].join(''),
  ['112aabb', '50308085b4c9fddb1cf7660a6f2'].join(''),
  ['192.168', '.49.227'].join(''),
  ['192.168', '.1.208'].join(''),
  ['192.168', '.30.189'].join(''),
  ['58.240', '.122.214'].join(''),
  ['admin', '123'].join(''),
  ['gos', '123'].join(''),
  ['root', '123'].join(''),
  ['gos-release-', 'local-2026'].join(''),
  ['gos-release-', 'docker-2026'].join(''),
  ['gos-release-', 'prod-2026'].join(''),
  ['gos-release-', 'production-2026'].join(''),
]

function gitGrepLiteral(literal) {
  return spawnSync('git', ['grep', '-n', '-I', '-F', '-e', literal, '--', '.'], {
    encoding: 'utf8',
  })
}

test('tracked files do not contain known leaked credentials or infrastructure addresses', () => {
  const hits = []

  for (const literal of blockedLiterals) {
    const result = gitGrepLiteral(literal)
    if (result.status === 0) {
      hits.push(result.stdout.trim())
      continue
    }
    assert.equal(result.status, 1, result.stderr || `git grep failed for ${literal}`)
  }

  assert.deepEqual(hits, [], `remove hardcoded sensitive literals:\n${hits.join('\n')}`)
})
