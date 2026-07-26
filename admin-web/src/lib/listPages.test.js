import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const listPages = [
  "UsersPage.jsx",
  "PostsPage.jsx",
  "CasesPage.jsx",
  "AdminAccountsPage.jsx",
];

test("all four paginated management pages use the shared regression-safe hook", async () => {
  for (const file of listPages) {
    const source = await readFile(new URL(`../pages/${file}`, import.meta.url), "utf8");
    assert.match(source, /usePagedList\(/, `${file} must use the shared list hook`);
  }
});
