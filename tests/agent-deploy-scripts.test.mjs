import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const gatewayScript = readFileSync(new URL('../scripts/agent/deploy_gateway_prod.sh', import.meta.url), 'utf8')
const signverifyScript = readFileSync(new URL('../scripts/agent/deploy_gc_signverify_prod.sh', import.meta.url), 'utf8')

test('gateway deploy script validates java runtime before restart', () => {
  assert.match(gatewayScript, /resolve_java_bin\(\)/, 'gateway deploy script should define a java resolver for non-login shells')
  assert.match(gatewayScript, /JAVA_BIN="\$\(resolve_java_bin\)"/, 'gateway deploy script should resolve java before invoking jar-start')
  assert.match(gatewayScript, /export PATH="\$\(dirname "\$\{JAVA_BIN\}"\):\$PATH"/, 'gateway deploy script should prepend the resolved java directory to PATH')
  assert.match(gatewayScript, /if \[ ! -f "jar-start" \]/, 'gateway deploy script should fail fast when jar-start is missing in the app directory')
  assert.match(gatewayScript, /command -v java/, 'gateway deploy script should check whether java is already available on PATH')
})

test('gc-signverify deploy script uses project path based jar naming and validates java runtime before restart', () => {
  assert.match(signverifyScript, /PROJECT_PATH="\{project_name\}"/, 'gc-signverify deploy script should accept the full project path placeholder')
  assert.match(signverifyScript, /MODULE_NAME="\$\(basename "\$\{PROJECT_PATH\}"\)"/, 'gc-signverify deploy script should derive the module name from the project path')
  assert.match(signverifyScript, /APP_DIR="\/home\/java\/\$\{PROJECT_PATH\}"/, 'gc-signverify deploy script should deploy into the nested project path directory')
  assert.match(signverifyScript, /TARGET_JAR_NAME="\$\{MODULE_NAME\}\.jar"/, 'gc-signverify deploy script should standardize the runtime jar name to the module name')
  assert.match(signverifyScript, /DOWNLOAD_OBJECT_PATH="\$\{PROJECT_PATH\}-\$\{IMAGE_VERSION\}\.jar"/, 'gc-signverify deploy script should download the artifact by project path and image version')
  assert.match(signverifyScript, /DOWNLOADED_JAR_NAME="\$\{MODULE_NAME\}-\$\{IMAGE_VERSION\}\.jar"/, 'gc-signverify deploy script should store the downloaded file under the module name')
  assert.match(signverifyScript, /resolve_java_bin\(\)/, 'gc-signverify deploy script should define a java resolver for non-login shells')
  assert.match(signverifyScript, /JAVA_BIN="\$\(resolve_java_bin\)"/, 'gc-signverify deploy script should resolve java before invoking jar-start')
  assert.match(signverifyScript, /export PATH="\$\(dirname "\$\{JAVA_BIN\}"\):\$PATH"/, 'gc-signverify deploy script should prepend the resolved java directory to PATH')
  assert.match(signverifyScript, /resolve_jar_start_script\(\)/, 'gc-signverify deploy script should resolve the correct jar-start launcher file')
  assert.match(signverifyScript, /JAR_START_SCRIPT="\$\(resolve_jar_start_script\)"/, 'gc-signverify deploy script should resolve the jar-start launcher before restart')
  assert.match(signverifyScript, /sh "\$\{JAR_START_SCRIPT\}" "\$\{TARGET_JAR_NAME\}" restart "\$\{RUN_ENV\}"/, 'gc-signverify deploy script should restart the app through the resolved jar-start launcher')
  assert.match(signverifyScript, /command -v java/, 'gc-signverify deploy script should check whether java is already available on PATH')
})
