import test from "node:test";
import assert from "node:assert/strict";

import { handleUnauthorizedResponse } from "./unauthorized.js";

test("401 clears the session and redirects to login", () => {
  const calls = [];
  const handled = handleUnauthorizedResponse(401, {
    clearSession() { calls.push("clear"); },
    pathname: "/users",
    redirect(path) { calls.push(`redirect:${path}`); },
  });
  assert.equal(handled, true);
  assert.deepEqual(calls, ["clear", "redirect:/login"]);
});

test("non-401 responses and the login page do not redirect", () => {
  const calls = [];
  assert.equal(handleUnauthorizedResponse(403, {
    clearSession() { calls.push("clear"); },
    pathname: "/users",
    redirect() { calls.push("redirect"); },
  }), false);
  assert.deepEqual(calls, []);

  assert.equal(handleUnauthorizedResponse(401, {
    clearSession() { calls.push("clear-login"); },
    pathname: "/login",
    redirect() { calls.push("redirect-login"); },
  }), true);
  assert.deepEqual(calls, ["clear-login"]);
});
