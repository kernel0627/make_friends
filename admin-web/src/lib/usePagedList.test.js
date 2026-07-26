import test from "node:test";
import assert from "node:assert/strict";

import { fallbackPageForEmptyResult, requestDelayForFilters } from "./usePagedList.js";

test("falls back after deleting the only item on the last page", () => {
  assert.equal(
    fallbackPageForEmptyResult(
      { page: 3, pageSize: 20 },
      { items: [], total: 40, page: 3, pageSize: 20 },
    ),
    2,
  );
});

test("does not move a non-empty or first page", () => {
  assert.equal(
    fallbackPageForEmptyResult(
      { page: 2, pageSize: 20 },
      { items: [{ id: "one" }], total: 21, page: 2, pageSize: 20 },
    ),
    0,
  );
  assert.equal(
    fallbackPageForEmptyResult(
      { page: 1, pageSize: 20 },
      { items: [], total: 0, page: 1, pageSize: 20 },
    ),
    0,
  );
});

test("debounces keyword searches but applies other filters immediately", () => {
  assert.equal(requestDelayForFilters({ keyword: "alice" }), 300);
  assert.equal(requestDelayForFilters({ keyword: "" }), 0);
});
