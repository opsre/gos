// Read-only Jenkins fixture for isolated onboarding browser verification.
// Run: node tests/fixtures/onboarding-jenkins.mjs [port]
import { createServer } from 'node:http'
const port = Number(process.argv[2] || 18882)
let rejectedWrites = 0
const definitions = {
  'demo-ci': [
    {name: 'DEPLOY_ENV', description: '部署环境，由发布时明确选择', _class: 'hudson.model.ChoiceParameterDefinition', choices: ['dev', 'prod'], defaultParameterValue: {value: 'dev'}},
    {name: 'PROJECT_NAME', description: '选择当前应用所属项目', _class: 'hudson.model.StringParameterDefinition', defaultParameterValue: {value: ''}},
  ],
  'demo-cd': [
    {name: 'CI_JOB', description: '上游构建任务', _class: 'hudson.model.StringParameterDefinition', defaultParameterValue: {value: ''}},
    {name: 'CI_BUILD', description: '本次构建号', _class: 'hudson.model.StringParameterDefinition', defaultParameterValue: {value: ''}},
  ],
}
createServer((req, res) => {
  if (req.method !== 'GET') { rejectedWrites++; res.writeHead(405); res.end('Execution disabled in fixture'); return }
  const path = new URL(req.url, `http://127.0.0.1:${port}`).pathname
  if (path === '/fixture-status') { res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify({rejectedWrites})); return }
  if (path.endsWith('/config.xml')) {
    res.setHeader('Content-Type', 'application/xml')
    res.end('<flow-definition><description>Read-only fixture</description><definition class="org.jenkinsci.plugins.workflow.cps.CpsFlowDefinition"><script>pipeline { agent any; stages { stage("Example") { steps { echo "fixture" } } } }</script><sandbox>true</sandbox></definition></flow-definition>')
    return
  }
  res.setHeader('Content-Type', 'application/json')
  if (path === '/api/json') { res.end(JSON.stringify({jobs: Object.keys(definitions).map(name => ({name, url: `http://127.0.0.1:${port}/job/${name}/`}))})); return }
  const name = path.match(/^\/job\/([^/]+)\/api\/json$/)?.[1]
  if (name && definitions[name]) { res.end(JSON.stringify({name, fullName: name, actions: [{parameterDefinitions: definitions[name]}]})); return }
  res.writeHead(404); res.end('{}')
}).listen(port, '127.0.0.1', () => process.stdout.write(`Read-only Jenkins fixture: http://127.0.0.1:${port}\n`))
