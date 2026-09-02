import { chromium } from "playwright-core";

const phone = process.env.COUPANG_PHONE;
if (!phone) throw new Error("Missing Doppler-injected COUPANG_PHONE");

// When COUPANG_CDP_URL is set, attach to a separately launched real Chrome
// carrying only the product's minimal raw-CDP flags. This distinguishes the
// browser launch fingerprint from the DOM action itself.
const cdpURL = process.env.COUPANG_CDP_URL;
const browser = cdpURL
  ? await chromium.connectOverCDP(cdpURL)
  : await chromium.launch({ headless: false, channel: "chrome" });
const context = cdpURL ? browser.contexts()[0] : await browser.newContext();
const page = await context.newPage();
const authNetwork: Array<Record<string, unknown>> = [];
page.on("response", (response) => {
  const request = response.request();
  if (!/coupang\.com$/i.test(new URL(response.url()).hostname)) return;
  if (request.method() === "GET" && !/login|auth|cert|sms|phone/i.test(response.url())) return;
  authNetwork.push({
    method: request.method(),
    path: new URL(response.url()).pathname,
    status: response.status(),
    contentType: response.headers()["content-type"] ?? null,
  });
});
page.on("requestfailed", (request) => {
  if (!/coupang\.com$/i.test(new URL(request.url()).hostname)) return;
  authNetwork.push({
    method: request.method(),
    path: new URL(request.url()).pathname,
    failure: request.failure()?.errorText ?? "unknown",
  });
});
const response = await page
  .goto("https://mc.coupang.com/ssr/desktop/order/list", { waitUntil: "domcontentloaded", timeout: 20_000 })
  .catch(() => null);
if (!response || response.status() >= 400) {
  process.stdout.write(`${JSON.stringify({
    status: "error",
    stage: "login-page",
    httpStatus: response?.status() ?? null,
    url: page.url(),
  }, null, 2)}\n`);
  await browser.close();
  process.exit(1);
}

await page.getByText("휴대폰번호 로그인", { exact: true }).first().click();
await page.waitForTimeout(1_000);
const phoneInput = page.locator('input[type="tel"], input[name*="phone" i], input[placeholder*="휴대폰"]').first();
await phoneInput.fill(phone);

const buttons = await page.locator("button").allTextContents();
const sendButton = page.locator('button:has-text("인증번호"), button:has-text("전송"), button:has-text("받기")').first();
const sendLabel = (await sendButton.textContent().catch(() => null))?.trim() ?? null;
let sent = false;
if (sendLabel && await sendButton.isEnabled()) {
  await sendButton.click();
  sent = true;
  await page.waitForTimeout(2_000);
}

const visibleMessages = (await page.locator('body').innerText()).split("\n").map((line) => line.trim()).filter(Boolean).filter((line) => /인증|오류|실패|발송|전송/.test(line)).slice(0, 12);
const requestFailed = visibleMessages.some((line) => /실패|오류/.test(line));

process.stdout.write(`${JSON.stringify({
  url: page.url(),
  title: await page.title(),
  sendLabel,
  sent: sent && !requestFailed,
  requestFailed,
  network: authNetwork.slice(-20),
  otpInputVisible: await page.locator('input:visible[placeholder*="인증번호"]').count(),
  visibleMessages,
  buttons: buttons.map((label) => label.trim()).filter(Boolean).slice(0, 15),
}, null, 2)}\n`);
await browser.close();
