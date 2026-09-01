import { chromium } from "playwright-core";

const cdpURL = process.env.COUPANG_CDP_URL ?? "http://127.0.0.1:9223";
const browser = await chromium.connectOverCDP(cdpURL);
const context = browser.contexts()[0];
const page = await context.newPage();
const captured: Array<Record<string, unknown>> = [];

page.on("response", async (response) => {
  const request = response.request();
  const type = request.resourceType();
  const contentType = response.headers()["content-type"] ?? "";
  if (!["xhr", "fetch"].includes(type) && !contentType.includes("json")) return;
  let body: unknown;
  if (contentType.includes("json")) {
    try {
      const parsed = await response.json();
      body = Array.isArray(parsed)
        ? { kind: "array", length: parsed.length, sampleKeys: Object.keys(parsed[0] ?? {}) }
        : { kind: typeof parsed, keys: Object.keys(parsed ?? {}) };
    } catch {
      body = { unreadable: true };
    }
  }
  captured.push({
    url: response.url(),
    method: request.method(),
    type,
    status: response.status(),
    contentType,
    body,
  });
});

await page.goto("https://www.coupang.com/np/search?q=%EC%97%90%EC%96%B4%ED%8C%9F", {
  waitUntil: "domcontentloaded",
  timeout: 20_000,
});
const href = await page.locator('a[href*="/vp/products/"]').first().getAttribute("href");
if (!href) throw new Error("No product URL found");
const productURL = new URL(href, "https://www.coupang.com").toString();
captured.length = 0;
await page.goto(productURL, { waitUntil: "domcontentloaded", timeout: 20_000 }).catch(() => undefined);
await page.waitForTimeout(2_000);
const reviewTab = page.locator('a[href*="review"], button:has-text("상품평"), a:has-text("상품평")').first();
if (await reviewTab.count()) {
  await reviewTab.scrollIntoViewIfNeeded().catch(() => undefined);
  await reviewTab.click({ timeout: 3_000 }).catch(() => undefined);
  await page.waitForTimeout(2_000);
}

process.stdout.write(`${JSON.stringify({ productURL, captured }, null, 2)}\n`);
await page.close();
process.exit(0);
