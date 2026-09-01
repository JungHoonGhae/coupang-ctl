import { chromium } from "playwright-core";

const cdpURL = process.env.COUPANG_CDP_URL ?? "http://127.0.0.1:9223";
const browser = await chromium.connectOverCDP(cdpURL);
const context = browser.contexts()[0];
const page = context.pages().find((candidate) => candidate.url().includes("coupang.com")) ?? context.pages()[0];

if (!page) throw new Error("No Coupang page found in the CDP browser");

const requests = new Map<string, { method: string; resourceType: string; status: number; contentType: string }>();
page.on("response", async (response) => {
  const request = response.request();
  const resourceType = request.resourceType();
  const contentType = response.headers()["content-type"] ?? "";
  if (!["xhr", "fetch"].includes(resourceType) && !contentType.includes("json")) return;
  const url = new URL(response.url());
  url.searchParams.delete("clickEventId");
  requests.set(url.toString(), {
    method: request.method(),
    resourceType,
    status: response.status(),
    contentType,
  });
});

await page
  .goto("https://www.coupang.com/np/search?q=%EC%97%90%EC%96%B4%ED%8C%9F", {
    waitUntil: "domcontentloaded",
    timeout: 20_000,
  })
  .catch(() => undefined);
await page.waitForTimeout(2_000);

process.stdout.write(`${JSON.stringify([...requests.entries()].map(([url, meta]) => ({ url, ...meta })), null, 2)}\n`);
process.exit(0);
