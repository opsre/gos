import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const root = new URL("../", import.meta.url);

test("release search home is the post-login landing page and opens an exact order match", async () => {
  const [view, router, layout, login] = await Promise.all([
    readFile(new URL("src/views/release/ReleaseOrderSearchView.vue", root), "utf8"),
    readFile(new URL("src/router/index.ts", root), "utf8"),
    readFile(new URL("src/layouts/AppLayout.vue", root), "utf8"),
    readFile(new URL("src/views/login/LoginView.vue", root), "utf8"),
  ]);

  assert.match(router, /path:\s*['"]\/release-search['"]/);
  assert.match(router, /name:\s*['"]release-order-search['"]/);
  assert.match(router, /path:\s*['"]\/['"][\s\S]*redirect:\s*['"]\/release-search['"]/);
  assert.match(login, /redirect\s*\|\|\s*['"]\/release-search['"]/);
  assert.match(layout, /key="release-home"/);
  assert.match(layout, /<a-menu-item key="release-home"[\s\S]*?<HomeOutlined \/>[\s\S]*?首页[\s\S]*?<\/a-menu-item>/);
  assert.ok(
    layout.indexOf('key="release-home"') < layout.indexOf('key="application-management"'),
    "首页应位于应用管理上方",
  );
  assert.doesNotMatch(
    layout.match(/<a-sub-menu v-if="showReleaseMenu"[\s\S]*?<\/a-sub-menu>/)?.[0] || "",
    /key="release-home"/,
  );

  assert.match(view, /role="search"/);
  assert.match(view, /@keydown\.enter\.prevent="handleSearch"/);
  assert.match(view, /请输入完整发布单号/);
  assert.match(view, /listReleaseOrders\(\{/);
  assert.match(view, /item\.order_no[\s\S]*toUpperCase\(\)\s*===\s*keyword/);
  assert.match(view, /name:\s*"release-order-detail"/);
  assert.match(view, /未找到发布单/);
  assert.match(view, /全部发布单/);
  assert.match(view, /新建发布单/);
  assert.match(view, /审批待办/);
  assert.doesNotMatch(view, /release-search-mark/);
  assert.doesNotMatch(view, /Google/);
  assert.match(view, /\.release-search-feedback\s*\{[\s\S]*?justify-content:\s*center/);
});
