import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const listURL = new URL('../src/views/release/ReleaseOrderListView.vue', import.meta.url)
const detailURL = new URL('../src/views/release/ReleaseOrderDetailView.vue', import.meta.url)
const apiURL = new URL('../src/api/release.ts', import.meta.url)
const typesURL = new URL('../src/types/release.ts', import.meta.url)
const utilURL = new URL('../src/utils/recent-commit.ts', import.meta.url)
const listSource = readFileSync(listURL, 'utf8')
const detailSource = readFileSync(detailURL, 'utf8')
const apiSource = readFileSync(apiURL, 'utf8')
const typesSource = readFileSync(typesURL, 'utf8')
const utilSource = readFileSync(utilURL, 'utf8')

test('release order list exposes a recent commit column fed by the batch api', () => {
  assert.match(
    listSource,
    /const initialColumns:[\s\S]*\{ title: "最近提交", key: "recent_commit", width: 180 \}[\s\S]*\{ title: "操作", key: "actions", width: 160 \}/,
    'release order list should render the recent commit column before the action column',
  )
  assert.match(
    listSource,
    /<template v-else-if="column\.key === 'recent_commit'">/,
    'recent commit column should have its own body cell branch',
  )
  assert.match(
    listSource,
    /class="release-recent-commit-sha"[\s\S]*recentCommitItem\(record\)\?\.short_sha[\s\S]*class="release-recent-commit-title"[\s\S]*recentCommitItem\(record\)\?\.title/,
    'recent commit cell should render short sha plus the commit title',
  )
  assert.match(
    listSource,
    /:href="recentCommitItem\(record\)\?\.web_url"[\s\S]*target="_blank"[\s\S]*rel="noopener noreferrer"/,
    'recent commit cell should open the commit in a new tab',
  )
  assert.match(
    listSource,
    /a-tooltip[\s\S]*recentCommitTooltipTitle\(record\)[\s\S]*recentCommitChips\(record\)[\s\S]*git-commit-chip/,
    'recent commit tooltip should render title plus the commit info chips',
  )
  assert.match(
    listSource,
    /<span v-else class="release-recent-commit-empty">\{\{\s*recentCommitPending\(record\) \? "获取中…" : "-"\s*\}\}<\/span>/,
    'recent commit cell should show a loading label while pending and fall back to a dash otherwise',
  )
  assert.match(
    listSource,
    /const recentCommitsMap = reactive<Record<string, ReleaseOrderRecentCommits>>\(\{\}\)/,
    'recent commits should be cached per order id',
  )
  assert.match(
    listSource,
    /const recentCommitsInflight = new Set<string>\(\)/,
    'recent commit requests should be deduplicated by an inflight set',
  )
  assert.match(
    listSource,
    /let recentCommitsRequestSeq = 0/,
    'recent commit requests should be guarded by a request sequence',
  )
  assert.match(
    listSource,
    /async function loadVisibleOrderRecentCommits\([\s\S]*\+\+recentCommitsRequestSeq[\s\S]*const currentIDs[\s\S]*getReleaseOrderRecentCommits\(targetIDs, RECENT_COMMIT_LIMIT\)/,
    'recent commit loader should follow the realtime-progress structure with the batch path',
  )
  assert.match(
    listSource,
    /void loadVisibleOrderRecentCommits\(response\.data, \{ force: !silent \}\)/,
    'release order list should refresh recent commits after a successful list load without a new timer',
  )
  assert.match(
    listSource,
    /const RECENT_COMMIT_REFRESH_INTERVAL_MS = 60_000;/,
    'recent commits should cache results between list polls instead of querying every 10s',
  )
  assert.doesNotMatch(
    listSource,
    /recentCommitsTimer|setInterval\([\s\S]{0,160}loadVisibleOrderRecentCommits/,
    'recent commits must follow the existing polling instead of adding an independent timer',
  )
  assert.match(
    listSource,
    /return recentCommitErrorText\(info\.error\)/,
    'recent commit failures should surface through the tooltip helper instead of a toast',
  )
  assert.doesNotMatch(
    listSource,
    /message\.(error|warning)\([^)]*[Cc]ommit/,
    'recent commit errors should not spam messages',
  )
})

test('release order detail renders recent commits inside the base info collapse', () => {
  assert.match(
    detailSource,
    /header="基础信息与参数快照"[\s\S]*class="detail-inline-section-title">最近提交[\s\S]*deploySnapshotSectionTitle/,
    'recent commits section should sit between base info and the deploy snapshot',
  )
  assert.match(
    detailSource,
    /class="detail-inline-section-summary detail-recent-commit-repo"[\s\S]*recentCommitRepositoryText\(recentCommits\)[\s\S]*recentCommits\?\.web_url/,
    'recent commits header should show the repository path, ref and repo link',
  )
  assert.match(
    detailSource,
    /class="detail-recent-commit-item"[\s\S]*class="detail-recent-commit-sha"[\s\S]*commit\.short_sha[\s\S]*class="detail-recent-commit-title"[\s\S]*commit\.title[\s\S]*recentCommitItemChips\(commit\)[\s\S]*git-commit-chip/,
    'recent commits list should render sha link, title and the author/time chips',
  )
  assert.match(
    detailSource,
    /const recentCommits = ref<ReleaseOrderRecentCommits \| null>\(null\)/,
    'detail page should keep the recent commit payload in its own ref',
  )
  assert.match(
    detailSource,
    /const recentCommitsError = await loadReleaseOrderRecentCommits\(orderID\.value, silent\);[\s\S]*detailErrors\.push\(`最近提交：\$\{recentCommitsError\}`\)/,
    'recent commits should load after the detail batch and only record errors',
  )
  assert.match(
    detailSource,
    /if \(!silent\) \{\s*recentCommitsLoading\.value = true;\s*\}/,
    'silent refresh should not flash the recent commit skeleton',
  )
  assert.match(
    detailSource,
    /<a-skeleton v-if="recentCommitsLoading" active :paragraph="\{ rows: 2 \}" \/>/,
    'recent commits should show a skeleton while loading',
  )
  assert.match(
    detailSource,
    /<div v-else class="detail-recent-commit-hint">\{\{ recentCommitHint \}\}<\/div>/,
    'recent commits should show a readable hint when empty or failed',
  )
  assert.match(
    utilSource,
    /if \(lowered\.includes\('credential'\) \|\| text\.includes\('凭证'\)\) \{\s*return '未配置 Git 凭证'\s*\}/,
    'missing credential errors should read as 未配置 Git 凭证',
  )
  assert.match(
    utilSource,
    /return `\$\{formatRelativeCommitTime\(parsed\)\} · \$\{parsed\.format\('YYYY-MM-DD HH:mm:ss'\)\}`/,
    'commit time should pair a relative time with the absolute time',
  )
})

test('recent commits api and types follow the frozen contract', () => {
  assert.match(
    apiSource,
    /export async function getReleaseOrderRecentCommits\(\s*orderIDs: string\[\],\s*limit = 5,\s*\): Promise<ReleaseOrderRecentCommitsResponse>/,
    'api should expose the batch recent commits helper with a default limit',
  )
  assert.match(
    apiSource,
    /http\.get<ReleaseOrderRecentCommitsResponse>\(\s*"\/release-orders\/recent-commits",[\s\S]*order_ids: ids\.join\(","\),[\s\S]*limit,/,
    'recent commits request should send order_ids and limit',
  )
  assert.match(
    typesSource,
    /export interface ReleaseOrderGitCommit \{[\s\S]*short_sha: string;[\s\S]*committed_at: string;[\s\S]*web_url: string;[\s\S]*\}/,
    'commit type should keep the frozen fields',
  )
  assert.match(
    typesSource,
    /export interface ReleaseOrderRecentCommits \{[\s\S]*repository: string;[\s\S]*ref: string;[\s\S]*web_url: string;[\s\S]*credential_name\?: string;[\s\S]*commits: ReleaseOrderGitCommit\[\];[\s\S]*error\?: string;[\s\S]*\}/,
    'recent commits type should keep repository/ref/credential/error fields',
  )
  assert.match(
    typesSource,
    /export interface ReleaseOrderRecentCommitsResponse \{\s*data: Record<string, ReleaseOrderRecentCommits>;\s*\}/,
    'recent commits response should be keyed by order id',
  )
})

test('recent commits show the last real change instead of a merge action', () => {
  assert.match(
    listSource,
    /function isMergeCommit\(commit: ReleaseOrderGitCommit \| null \| undefined\) \{\s*return \/\^merge\\b\/i\.test\(String\(commit\?\.title \|\| ""\)\.trim\(\)\);\s*\}/,
    'list should classify merge commits by their message',
  )
  assert.match(
    listSource,
    /function recentCommitItem\(record: ReleaseOrder\): ReleaseOrderGitCommit \| null \{[\s\S]*commits\.find\(\(item\) => !isMergeCommit\(item\)\) \|\| commits\[0\]/,
    'list cell should show the newest non-merge commit and fall back to the branch head',
  )
  assert.match(
    listSource,
    /tone: "head",[\s\S]*?label: "分支当时 HEAD",[\s\S]*?value: anchor\.short_sha/,
    'list tooltip chip should still name the branch head at the release instant',
  )
  assert.match(
    detailSource,
    /function recentCommitList\(commits: ReleaseOrderRecentCommits \| null\)[\s\S]*filter\(\(item\) => !\/\^merge\\b\/i\.test\(String\(item\.title \|\| ""\)\.trim\(\)\)\)[\s\S]*changes\.length > 0 \? changes : items/,
    'detail list should skip merge commits and fall back to the unfiltered list',
  )
  assert.match(
    detailSource,
    /tone: "head",[\s\S]*?label: "分支当时 HEAD",[\s\S]*?value: anchor\.short_sha/,
    'detail chips should name the branch head when the shown commits skip it',
  )
})
