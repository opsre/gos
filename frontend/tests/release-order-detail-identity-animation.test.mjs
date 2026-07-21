import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const root = new URL("../", import.meta.url);

test("release detail manually switches release name and order number with a fixed copy action", async () => {
  const source = await readFile(
    new URL("src/views/release/ReleaseOrderDetailView.vue", root),
    "utf8",
  );

  assert.match(source, /CopyOutlined/);
  assert.match(source, /SwapOutlined/);
  assert.match(source, /heroShowsReleaseName/);
  assert.match(source, /release_name/);
  assert.match(source, /function toggleHeroIdentity\(\)/);
  assert.doesNotMatch(source, /window\.setInterval/);
  assert.match(source, /<Transition name="release-identity-swap" mode="out-in">/);
  assert.match(source, /切换到发布单号/);
  assert.match(source, /切换到发布名称/);
  assert.match(source, /aria-label="复制发布单号"/);
  assert.match(source, /navigator\.clipboard\.writeText\(text\)/);
  assert.match(source, /发布单号已复制/);
  assert.match(source, /@media \(prefers-reduced-motion: reduce\)/);
});
