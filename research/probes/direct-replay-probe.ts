import { chromium } from "playwright-core";

const browser = await chromium.connectOverCDP(process.env.COUPANG_CDP_URL ?? "http://127.0.0.1:9223");
const context = browser.contexts()[0];
const cookies = await context.cookies("https://www.coupang.com");
const cookie = cookies.map(({ name, value }) => `${name}=${value}`).join("; ");
const url = "https://www.coupang.com/next-api/review?productId=9024163013&page=1&size=3&sortBy=ORDER_SCORE_ASC&ratingSummary=true&ratings=&market=";
const response = await fetch(url, {
  headers: {
    cookie,
    referer: "https://www.coupang.com/vp/products/9024163013?itemId=26462308675&vendorItemId=93437588392",
    "user-agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 Safari/537.36",
    accept: "application/json, text/plain, */*",
  },
});
const body = await response.text();
let summary: unknown = body.slice(0, 120);
try {
  const parsed = JSON.parse(body);
  summary = { keys: Object.keys(parsed), dataKeys: Object.keys(parsed.rData ?? {}) };
} catch {}
process.stdout.write(`${JSON.stringify({ status: response.status, contentType: response.headers.get("content-type"), bytes: body.length, summary }, null, 2)}\n`);
process.exit(0);
