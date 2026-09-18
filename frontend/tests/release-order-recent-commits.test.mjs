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

test('release order list reads the recent commit cell from the persisted order fields', () => {
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
    /v-if="recentCommitSHA\(record\)"[\s\S]*class="release-recent-commit-sha"[\s\S]*shortCommitSHA\(record\.head_change_sha\)[\s\S]*class="release-recent-commit-title"[\s\S]*record\.head_change_title/,
    'recent commit cell should render the short sha plus the title of the persisted change commit',
  )
  assert.match(
    listSource,
    /:href="record\.head_change_url \|\| undefined"[\s\S]*target="_blank"[\s\S]*rel="noopener noreferrer"/,
    'recent commit cell should open the persisted commit url in a new tab',
  )
  assert.match(
    listSource,
    /<span v-else class="release-recent-commit-empty">-<\/span>/,
    'a release order without persisted commit fields should fall back to a dash',
  )
  assert.match(
    listSource,
    /a-tooltip[\s\S]*recentCommitTooltipTitle\(record\)[\s\S]*recentCommitTooltipMeta\(record\)[\s\S]*recentCommitRepositoryText\(record\)/,
    'recent commit tooltip should keep the plain-text title, meta and repository lines',
  )
  assert.doesNotMatch(
    listSource,
    /获取中/,
    'the list must not show a pending label now that commit info comes with the order payload',
  )
})

test('release order list tooltip spells out author, release instant, branch head and repository', () => {
  assert.match(
    listSource,
    /function recentCommitTooltipMeta\(record: ReleaseOrder\) \{[\s\S]*record\.head_change_author[\s\S]*formatRecentCommitTime\(record\.head_change_at\)/,
    'tooltip meta should read the persisted author and commit time',
  )
  assert.match(
    listSource,
    /const asOf = record\.started_at \|\| record\.created_at;[\s\S]*parts\.push\(`发布时点 \$\{dayjs\(asOf\)\.format\("YYYY-MM-DD HH:mm"\)\}`\)/,
    'the release instant should fall back from started_at to created_at and stay plain text',
  )
  assert.match(
    listSource,
    /const headSHA = shortCommitSHA\(record\.head_commit_sha\);[\s\S]*parts\.push\(`分支当时 HEAD \$\{headSHA\}`\)/,
    'the tooltip should name the branch head captured when the release order was created',
  )
  assert.match(
    listSource,
    /function recentCommitRepositoryText\(record: ReleaseOrder\) \{[\s\S]*\[record\.application_name, record\.head_commit_ref \|\| record\.git_ref\]/,
    'the repository line should reuse application name plus branch instead of opening a new request',
  )
})

test('release order list no longer polls a live recent commits api', () => {
  assert.doesNotMatch(
    listSource,
    /getReleaseOrderRecentCommits|loadVisibleOrderRecentCommits|recentCommitsMap|recentCommitsInflight|recentCommitsFetchedAt|recentCommitsRequestSeq/,
    'the list should not keep any live recent commit fetch state',
  )
  assert.doesNotMatch(
    listSource,
    /RECENT_COMMIT_LIMIT|RECENT_COMMIT_REFRESH_INTERVAL_MS|RECENT_COMMIT_RETRY_INTERVAL_MS/,
    'the batch limits and cache windows of the live api should be gone',
  )
  assert.doesNotMatch(
    listSource,
    /setInterval\([\s\S]{0,160}recentCommit/i,
    'recent commits must not add an independent polling timer',
  )
})

test('release order detail shows the persisted commit in the spotlight card', () => {
  assert.match(
    detailSource,
    /const spotlightCommit = computed\(\(\) => \{[\s\S]*record\.head_change_sha[\s\S]*record\.head_change_title[\s\S]*record\.head_change_author[\s\S]*record\.head_change_url[\s\S]*record\.head_change_at/,
    'the spotlight commit should be derived from the persisted release order fields',
  )
  assert.match(
    detailSource,
    /const spotlightHeadSHA = computed\(\(\) => shortCommitSHA\(order\.value\?\.head_commit_sha\)\)/,
    'the head chip should read the branch head stored on the release order',
  )
  assert.match(
    detailSource,
    /const spotlightCommitAsOfText = computed\(\(\) => \{[\s\S]*order\.value\?\.started_at \|\| order\.value\?\.created_at[\s\S]*return `发布时点 \$\{dayjs\(asOf\)\.format\("YYYY-MM-DD HH:mm"\)\}`/,
    'the release instant should fall back to the order times instead of the removed as_of field',
  )
  assert.match(
    detailSource,
    /class="release-spotlight-commits-head"[\s\S]*最近提交[\s\S]*class="release-spotlight-commits-meta"[\s\S]*spotlightCommitAsOfText[\s\S]*class="release-spotlight-head-chip"[\s\S]*HEAD \{\{ spotlightHeadSHA \}\}/,
    'spotlight commits head should name the release instant and only then pin the branch head chip',
  )
  assert.match(
    detailSource,
    /class="release-spotlight-commit"[\s\S]*shortCommitSHA\(spotlightCommit\.sha\)[\s\S]*spotlightCommit\.title[\s\S]*release-spotlight-commit-author[\s\S]*spotlightCommit\.author/,
    'the spotlight commit row should render sha, title and the author chip',
  )
})

test('release order detail hides the recent commits block when the fields are empty', () => {
  assert.match(
    detailSource,
    /const showSpotlightCommits = computed\(\s*\(\) => Boolean\(spotlightCommit\.value\) \|\| Boolean\(spotlightHeadSHA\.value\),?\s*\)/,
    'the block should be driven purely by the persisted fields',
  )
  assert.match(
    detailSource,
    /<div v-if="showSpotlightCommits" class="release-spotlight-commits">/,
    'the spotlight commits block should be skipped entirely for orders without commit info',
  )
  assert.doesNotMatch(
    detailSource,
    /loadReleaseOrderRecentCommits|recentCommitsLoading|recentCommitsRequestError|recentCommits\b|recentCommitAsOfText|recentCommitHeadSha|recentCommitList|recentCommitHint/,
    'the detail page should drop the live loader, its refs and the fallback hint copy',
  )
  assert.doesNotMatch(
    detailSource,
    /release-spotlight-commit-hint/,
    'the empty-state hint style should be removed with the live loader',
  )
  assert.doesNotMatch(
    detailSource,
    /获取中/,
    'no loading label should remain in the spotlight commits block',
  )
})

test('release spotlight keeps its status layout and chip palette', () => {
  assert.match(
    detailSource,
    /class="release-spotlight"[\s\S]*class="release-spotlight-head"[\s\S]*release-spotlight-\$\{spotlightStatusKey\}[\s\S]*class="release-spotlight-orb"[\s\S]*class="release-spotlight-title"[\s\S]*class="release-spotlight-description"[\s\S]*整体进度 · \{\{ spotlightDescription \}\}/,
    'spotlight status head should stack the status orb, the title plus the 整体进度 description',
  )
  assert.match(
    detailSource,
    /'release-spotlight-meta',\s*'status-tag',\s*statusToneClass\(currentBusinessStatus\),[\s\S]*statusText\(currentBusinessStatus\)/,
    'spotlight status head should end with a status pill naming the release order status',
  )
  assert.match(
    detailSource,
    /class="release-spotlight-head"[\s\S]*class="release-spotlight-time"[\s\S]*class="release-spotlight-commits"/,
    'the spotlight card should read top to bottom as status head, release instant and recent commits',
  )
  assert.match(
    detailSource,
    /class="release-spotlight-time"[\s\S]*发布时间[\s\S]*spotlightPublishedAt/,
    'the release instant row should pair the clock icon and label with the resolved time',
  )
  assert.match(
    detailSource,
    /const spotlightPublishedAt = computed\(\(\) =>\s*formatTime\(order\.value\?\.started_at \|\| order\.value\?\.created_at \|\| null\)/,
    'the release instant should fall back from started_at to created_at',
  )
  assert.match(
    detailSource,
    /\.release-spotlight-head-chip \{[\s\S]*background: #f5f3ff;[\s\S]*color: #6d28d9;/,
    'the branch head chip should use the purple chip palette',
  )
  assert.match(
    detailSource,
    /\.release-spotlight-commit-author \{[\s\S]*background: #f5f3ff;[\s\S]*color: #7c3aed;/,
    'the author chip should reuse the purple palette in a smaller size',
  )
  assert.match(
    detailSource,
    /\.release-spotlight-commit \+ \.release-spotlight-commit \{\s*border-top: 1px solid/,
    'spotlight commit rows should keep the hairline separator between consecutive rows',
  )
  for (const [statusKey, iconName] of [
    ['running', 'SyncOutlined'],
    ['queued', 'ClockCircleFilled'],
    ['success', 'CheckCircleFilled'],
    ['failed', 'CloseCircleFilled'],
    ['cancelled', 'StopFilled'],
  ]) {
    assert.match(
      detailSource,
      new RegExp(`<${iconName}[\\s\\S]{0,160}spotlightStatusKey === '${statusKey}'`),
      `the spotlight orb should keep its own icon branch for ${statusKey}`,
    )
  }
  assert.match(
    detailSource,
    /\.release-spotlight-orb-success \{[\s\S]*?background: #dcfce7;[\s\S]*?color: #15803d;/,
    'the success orb should use the soft green tint with a green check',
  )
  assert.match(
    detailSource,
    /\.release-spotlight-orb-failed \{[\s\S]*?background: #fee2e2;[\s\S]*?color: #b91c1c;/,
    'the failed orb should use the soft red tint',
  )
  assert.match(
    detailSource,
    /\.release-spotlight-orb-running \{[\s\S]*?background: #dbeafe;[\s\S]*?color: #1d4ed8;/,
    'the running orb should use the soft blue tint',
  )
  assert.match(
    detailSource,
    /\.release-spotlight-orb-queued,\s*\.release-spotlight-orb-pending \{[\s\S]*?background: #fef3c7;[\s\S]*?color: #b45309;/,
    'queued, pending and built-waiting-deploy should share the soft orange orb',
  )
  assert.match(
    detailSource,
    /\.release-spotlight-orb-cancelled \{[\s\S]*?background: #e2e8f0;[\s\S]*?color: #475569;/,
    'the cancelled orb should use the soft grey tint',
  )
  assert.doesNotMatch(
    detailSource,
    /release-spotlight-icon-wrap|release-spotlight-content|spotlightMeta/,
    'the old horizontal spotlight layout and its spotlightMeta copy should be gone',
  )
  assert.doesNotMatch(
    detailSource,
    /class="detail-inline-section-title">最近提交/,
    'the old recent commits section inside the collapsed panel should be gone',
  )
})

test('recent commits api and types follow the frozen persisted-field contract', () => {
  assert.doesNotMatch(
    apiSource,
    /getReleaseOrderRecentCommits|release-orders\/recent-commits/,
    'the live recent commits endpoint should be removed from the api layer',
  )
  assert.match(
    typesSource,
    /export interface ReleaseOrder \{[\s\S]*head_commit_sha\?: string;[\s\S]*head_commit_ref\?: string;[\s\S]*head_change_sha\?: string;[\s\S]*head_change_title\?: string;[\s\S]*head_change_author\?: string;[\s\S]*head_change_at\?: string \| null;[\s\S]*head_change_url\?: string;/,
    'release order should carry the seven persisted commit fields with the frozen names',
  )
  assert.doesNotMatch(
    typesSource,
    /export interface ReleaseOrder(GitCommit|RecentCommits|RecentCommitsResponse)\b/,
    'the live recent commits types should be gone',
  )
  assert.doesNotMatch(
    typesSource,
    /ReleaseOrderGitCommit|ReleaseOrderRecentCommits|ReleaseCommitChip/,
    'no live recent commit type should stay behind in the release types',
  )
})

test('recent commit helpers keep only the formatting the persisted fields need', () => {
  assert.match(
    utilSource,
    /export function formatRecentCommitTime\(value\?: string \| null\): string \{[\s\S]*return `\$\{formatRelativeCommitTime\(parsed\)\} · \$\{parsed\.format\('YYYY-MM-DD HH:mm:ss'\)\}`/,
    'commit time should pair a relative time with the absolute time',
  )
  assert.match(
    utilSource,
    /export function shortCommitSHA\(value\?: string \| null\): string \{\s*return String\(value \|\| ''\)\.trim\(\)\.slice\(0, 7\)\s*\}/,
    'short sha should be derived in one place for the list cell and the head chip',
  )
  assert.doesNotMatch(
    utilSource,
    /recentCommitErrorText|credential/,
    'the credential error copy only served the removed live api',
  )
})
