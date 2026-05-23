import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const serverMain = readFileSync(new URL('../cmd/server/main.go', import.meta.url), 'utf8')

test('server binds the TCP port before external dependency checks and router setup', () => {
  const listenIndex = serverMain.indexOf('net.Listen("tcp", cfg.Server.Addr)')
  const jenkinsCheckIndex = serverMain.indexOf('bootstrap.CheckJenkinsConnection')
  const routerSetupIndex = serverMain.indexOf('httpapi.NewRouter')

  assert.notEqual(listenIndex, -1, 'server should bind the configured TCP address explicitly')
  assert.ok(listenIndex < jenkinsCheckIndex, 'port binding should fail before Jenkins checks emit startup logs')
  assert.ok(listenIndex < routerSetupIndex, 'port binding should fail before Gin route debug output')
})
