import test from 'node:test'
import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'

const routerURL = new URL('../src/router/index.ts', import.meta.url)
const layoutURL = new URL('../src/layouts/AppLayout.vue', import.meta.url)
const viewURL = new URL('../src/views/component/GitCredentialManagementView.vue', import.meta.url)
const apiURL = new URL('../src/api/git-credential.ts', import.meta.url)
const typesURL = new URL('../src/types/git-credential.ts', import.meta.url)
const routerSource = readFileSync(routerURL, 'utf8')
const layoutSource = readFileSync(layoutURL, 'utf8')
const viewSource = readFileSync(viewURL, 'utf8')
const apiSource = readFileSync(apiURL, 'utf8')
const typesSource = readFileSync(typesURL, 'utf8')

function extractStyleRule(source, selector) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = source.match(new RegExp(`${escaped}\\s*\\{([\\s\\S]*?)\\n\\}`))
  assert.ok(match, `expected to find style rule for ${selector}`)
  return match[1]
}

test('git credential management is registered in the router and component menu', () => {
  assert.ok(existsSync(viewURL), 'git credential management page should exist')
  assert.match(
    routerSource,
    /const GitCredentialManagementView = \(\) => import\('\.\.\/views\/component\/GitCredentialManagementView\.vue'\)/,
    'router should lazy-load the git credential management page',
  )
  assert.match(
    routerSource,
    /path:\s*'\/components\/credentials'[\s\S]*name:\s*'git-credential-management'[\s\S]*component:\s*GitCredentialManagementView[\s\S]*meta:\s*\{\s*title:\s*'凭证管理',\s*permission:\s*\['component\.credential\.view',\s*'component\.credential\.manage'\]\s*\}/,
    'router should expose the credential route with view/manage permissions',
  )
  assert.match(
    layoutSource,
    /route\.path\.startsWith\('\/components\/credentials'\)[\s\S]*return \['git-credential-management'\]/,
    'sidebar should activate the credential menu item',
  )
  assert.match(
    layoutSource,
    /function goToGitCredentialManagement\(\)[\s\S]*router\.push\('\/components\/credentials'\)/,
    'sidebar should navigate to the credential page',
  )
  assert.match(
    layoutSource,
    /const canViewGitCredential = computed\([\s\S]*component\.credential\.view[\s\S]*component\.credential\.manage/,
    'sidebar should resolve credential permissions from view or manage',
  )
  assert.match(
    layoutSource,
    /canViewGitCredential\.value \|\|/,
    'component menu visibility should include the credential permission',
  )
  assert.match(
    layoutSource,
    /key="gitops-management"[\s\S]*GitOps管理[\s\S]*key="git-credential-management"[\s\S]*@click="goToGitCredentialManagement"[\s\S]*凭证管理/,
    'component menu should render the credential entry right after GitOps管理',
  )
})

test('git credential page keeps the standard header, modal form and square table', () => {
  assert.match(
    viewSource,
    /<div class="page-header">[\s\S]*<div class="page-title">凭证管理<\/div>[\s\S]*<div class="page-header-actions">/,
    'credential page should keep the title on the left and actions on the right',
  )
  assert.match(
    viewSource,
    /class="application-toolbar-action-btn"[\s\S]*新增凭证/,
    'credential page should expose the create action as a toolbar button',
  )
  assert.match(
    viewSource,
    /按仓库地址前缀匹配（最长前缀优先），仓库地址以该前缀开头的应用会使用此凭证/,
    'credential page should explain longest-prefix matching',
  )
  assert.match(
    viewSource,
    /GitLab API 需要访问令牌/,
    'credential page should hint that GitLab API needs an access token',
  )
  assert.match(
    viewSource,
    /placeholder="http:\/\/git\.cloud\.local:9080"/,
    'base url field should document the internal GitLab prefix example',
  )
  assert.doesNotMatch(
    viewSource,
    /<a-alert/,
    'page hints should not use block alerts',
  )
  assert.match(
    viewSource,
    /const credentialColumns: TableColumnsType<GitCredential> = \[[\s\S]*title: '名称'[\s\S]*title: '地址前缀'[\s\S]*title: '认证方式'[\s\S]*title: '用户名'[\s\S]*title: '密钥'[\s\S]*title: '状态'[\s\S]*title: '备注'[\s\S]*title: '操作'/,
    'credential table should expose the required columns in order',
  )
  assert.match(viewSource, /formatCredentialState\(record\.secret_configured\)/, 'secret column should read 已配置/未配置')
  assert.match(
    viewSource,
    /<a-modal[\s\S]*wrap-class-name="credential-modal-wrap"/,
    'credential form should use the shared modal shell',
  )
  assert.match(
    viewSource,
    /<a-form[\s\S]*layout="vertical"[\s\S]*:rules="credentialFormRules"/,
    'credential form should use the vertical layout with field rules',
  )
  assert.match(
    viewSource,
    /:loading="savingCredential"[\s\S]*保存/,
    'credential modal save button should expose the saving state',
  )
  assert.match(
    viewSource,
    /<a-popconfirm[\s\S]*@confirm="deleteCredential\(record\)"/,
    'credential rows should delete through a popconfirm',
  )
  assert.match(
    viewSource,
    /placeholder="creatingCredential \? `请输入\$\{credentialSecretLabel\}` : '留空表示不修改'"/,
    'editing a credential should keep the secret optional',
  )
  assert.match(
    viewSource,
    /留空表示不修改已保存的密钥。/,
    'editing a credential should explain that a blank secret keeps the stored value',
  )
  assert.match(viewSource, /message\.success\('凭证已新增'\)/, 'credential creation should report success')
  assert.match(viewSource, /message\.error\(/, 'credential failures should surface through message.error')

  const tableShellRule = extractStyleRule(
    viewSource,
    '.credential-table :deep(.ant-table-container),\n.credential-table :deep(.ant-table-thead > tr > th),\n.credential-table :deep(.ant-table-tbody > tr > td),\n.credential-table :deep(.ant-table-tbody > tr:last-child > td)',
  )
  assert.match(tableShellRule, /border-radius:\s*0 !important/, 'credential table must keep square corners')
  assert.doesNotMatch(
    viewSource,
    /border-radius:\s*(18px|20px)/,
    'credential page must not reintroduce rounded table shells',
  )

  const fixedColumnRule = extractStyleRule(
    viewSource,
    '.credential-table :deep(.ant-table-tbody > tr > td.ant-table-cell-fix-right)',
  )
  assert.match(fixedColumnRule, /background:\s*#fff !important/, 'fixed action column should stay opaque')
})

test('git credential api and types follow the frozen contract', () => {
  assert.match(apiSource, /import \{ http \} from '\.\/http'/, 'api layer should reuse the shared http client')
  assert.match(
    apiSource,
    /http\.get<GitCredentialListResponse>\('\/git-credentials', \{ params \}\)/,
    'listGitCredentials should hit GET /git-credentials with query params',
  )
  assert.match(
    apiSource,
    /http\.get<GitCredentialDataResponse>\(`\/git-credentials\/\$\{id\}`\)/,
    'getGitCredentialByID should hit GET /git-credentials/:id',
  )
  assert.match(
    apiSource,
    /http\.post<GitCredentialDataResponse>\('\/git-credentials', payload\)/,
    'createGitCredential should POST /git-credentials',
  )
  assert.match(
    apiSource,
    /http\.put<GitCredentialDataResponse>\(`\/git-credentials\/\$\{id\}`, payload\)/,
    'updateGitCredential should PUT /git-credentials/:id',
  )
  assert.match(
    apiSource,
    /http\.delete\(`\/git-credentials\/\$\{id\}`\)/,
    'deleteGitCredential should DELETE /git-credentials/:id',
  )
  assert.match(
    apiSource,
    /http\.post<GitCredentialTestResponse>\(`\/git-credentials\/\$\{id\}\/test`\)/,
    'testGitCredentialByID should POST /git-credentials/:id/test',
  )

  assert.match(typesSource, /export type GitCredentialProvider = 'gitlab'/, 'provider type should stay frozen')
  assert.match(
    typesSource,
    /export type GitCredentialAuthType = 'token' \| 'password'/,
    'auth type should stay token/password',
  )
  assert.match(
    typesSource,
    /export type GitCredentialStatus = 'active' \| 'disabled'/,
    'status type should stay active/disabled',
  )
  assert.match(typesSource, /export interface GitCredential \{[\s\S]*secret_configured: boolean[\s\S]*\}/, 'GitCredential should expose secret_configured')
  assert.match(
    typesSource,
    /export interface GitCredentialTestResult \{[\s\S]*ok: boolean[\s\S]*message: string[\s\S]*gitlab_username\?: string/,
    'test result should carry ok/message/gitlab_username',
  )
  assert.match(
    typesSource,
    /export interface GitCredentialListResponse \{[\s\S]*data: GitCredential\[\][\s\S]*page: number[\s\S]*page_size: number[\s\S]*total: number/,
    'list response should keep the paged envelope',
  )
})
