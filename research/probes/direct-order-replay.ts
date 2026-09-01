import { chromium } from "playwright-core";

const browser = await chromium.connectOverCDP(process.env.COUPANG_CDP_URL ?? "http://127.0.0.1:9223");
const context = browser.contexts()[0];
const cookies = await context.cookies("https://mc.coupang.com");
const bootstrapPage = await context.newPage();
let capturedHeaders: Record<string, string> = {};
bootstrapPage.on("request", async (request) => {
  if (request.isNavigationRequest() && request.url().startsWith("https://mc.coupang.com/ssr/desktop/order/list")) {
    capturedHeaders = await request.allHeaders();
  }
});
await bootstrapPage.goto("https://mc.coupang.com/ssr/desktop/order/list", { waitUntil: "domcontentloaded", timeout: 30_000 });
await bootstrapPage.close();
for (const name of Object.keys(capturedHeaders)) {
  if (name.startsWith(":")) delete capturedHeaders[name];
}
delete capturedHeaders["accept-encoding"];
delete capturedHeaders["content-length"];
const response = await fetch("https://mc.coupang.com/ssr/desktop/order/list", {
  headers: {
    ...capturedHeaders,
    cookie: cookies.map(({ name, value }) => `${name}=${value}`).join("; "),
    accept: "text/html,application/xhtml+xml",
    "user-agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 Safari/537.36",
  },
  redirect: "manual",
});
const html = await response.text();
const match = html.match(/<script id="__NEXT_DATA__" type="application\/json">([\s\S]*?)<\/script>/);
if (!match) {
  process.stdout.write(`${JSON.stringify({ status: response.status, bytes: html.length, nextData: false })}\n`);
  process.exit(1);
}
const nextData = JSON.parse(match[1]);
const orders = nextData.props?.pageProps?.domains?.desktopOrder;
if (!orders) {
  process.stdout.write(`${JSON.stringify({
    status: response.status,
    bytes: html.length,
    nextData: true,
    page: nextData.page,
    pagePropKeys: Object.keys(nextData.props?.pageProps ?? {}),
  }, null, 2)}\n`);
  process.exit(1);
}
process.stdout.write(`${JSON.stringify({
  status: response.status,
  bytes: html.length,
  nextData: true,
  orderCountOnPage: orders.orderList.length,
  pagination: orders.orderPagination,
  searchKeys: Object.keys(orders.search),
}, null, 2)}\n`);
process.exit(0);
