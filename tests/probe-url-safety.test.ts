import assert from "node:assert/strict";
import test from "node:test";

import { isCoupangHostURL, isCoupangLoginURL } from "../research/probes/coupang-url.js";

test("accepts Coupang hosts at a parsed DNS boundary", () => {
  assert.equal(isCoupangHostURL("https://coupang.com/"), true);
  assert.equal(isCoupangHostURL("https://www.coupang.com/np/search"), true);
  assert.equal(isCoupangHostURL("https://LOGIN.COUPANG.COM/login"), true);
});

test("rejects lookalike, credential, and malformed URLs", () => {
  assert.equal(isCoupangHostURL("https://notcoupang.com/"), false);
  assert.equal(isCoupangHostURL("https://login.coupang.com.evil.example/"), false);
  assert.equal(isCoupangHostURL("https://login.coupang.com@evil.example/"), false);
  assert.equal(isCoupangHostURL("not a URL"), false);
});

test("recognizes only the exact login host", () => {
  assert.equal(isCoupangLoginURL("https://login.coupang.com/login"), true);
  assert.equal(isCoupangLoginURL("https://mc.coupang.com/ssr/desktop/order/list"), false);
  assert.equal(isCoupangLoginURL("https://login.coupang.com.evil.example/"), false);
});
