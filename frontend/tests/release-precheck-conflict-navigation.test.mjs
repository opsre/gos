import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const root = new URL("../", import.meta.url);

test("release precheck exposes and renders conflict-order navigation", async () => {
  const [types, listView, detailView] = await Promise.all([
    readFile(new URL("src/types/release.ts", root), "utf8"),
    readFile(new URL("src/views/release/ReleaseOrderListView.vue", root), "utf8"),
    readFile(new URL("src/views/release/ReleaseOrderDetailView.vue", root), "utf8"),
  ]);

  assert.match(types, /export interface ReleaseOrderPrecheck[\s\S]*conflict_order_id:\s*string;/);

  for (const source of [listView, detailView]) {
    assert.match(source, /(?:itemKey|item\.key)\s*===\s*["']concurrency_lock["']/);
    assert.match(source, /conflict_order_id/);
    assert.match(source, /name:\s*["']release-order-detail["']/);
    assert.match(source, /查看前序单/);
  }
});
