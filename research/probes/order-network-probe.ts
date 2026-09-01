import { chromium } from "playwright-core";

const browser = await chromium.connectOverCDP(process.env.COUPANG_CDP_URL ?? "http://127.0.0.1:9223");
const context = browser.contexts()[0];
const page = await context.newPage();
const calls: Array<Record<string, unknown>> = [];

page.on("response", async (response) => {
  const request = response.request();
  const contentType = response.headers()["content-type"] ?? "";
  if (!["xhr", "fetch"].includes(request.resourceType()) && !contentType.includes("json")) return;
  const parsedURL = new URL(response.url());
  let shape: unknown;
  if (contentType.includes("json")) {
    try {
      const data = await response.json();
      shape = Array.isArray(data)
        ? { type: "array", length: data.length, itemKeys: Object.keys(data[0] ?? {}) }
        : { type: typeof data, keys: Object.keys(data ?? {}) };
    } catch {
      shape = { unreadable: true };
    }
  }
  calls.push({
    origin: parsedURL.origin,
    path: parsedURL.pathname,
    queryNames: [...parsedURL.searchParams.keys()],
    method: request.method(),
    status: response.status(),
    contentType,
    shape,
  });
});

await page.goto("https://mc.coupang.com/ssr/desktop/order/list", {
  waitUntil: "domcontentloaded",
  timeout: 30_000,
}).catch(() => undefined);
await page.waitForTimeout(4_000);

const unique = [...new Map(calls.map((entry) => [`${entry.method} ${entry.origin}${entry.path}`, entry])).values()];
process.stdout.write(`${JSON.stringify({
  finalOrigin: new URL(page.url()).origin,
  finalPath: new URL(page.url()).pathname,
  redirectedToLogin: page.url().includes("login.coupang.com"),
  calls: unique,
}, null, 2)}\n`);
await page.close();
process.exit(0);
