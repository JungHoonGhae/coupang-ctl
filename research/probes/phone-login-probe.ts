import { chromium } from "playwright-core";

const phone = process.env.COUPANG_PHONE;
if (!phone) throw new Error("Missing Doppler-injected COUPANG_PHONE");

const browser = await chromium.connectOverCDP(process.env.COUPANG_CDP_URL ?? "http://127.0.0.1:9223");
const context = browser.contexts()[0];
for (const oldPage of context.pages().filter((candidate) => candidate.url().includes("login.coupang.com"))) {
  await oldPage.close().catch(() => undefined);
}
const page = await context.newPage();
await page.goto("https://login.coupang.com/login/login.pang", { waitUntil: "domcontentloaded", timeout: 20_000 });

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

process.stdout.write(`${JSON.stringify({
  url: page.url(),
  title: await page.title(),
  sendLabel,
  sent,
  otpInputVisible: await page.locator('input:visible[placeholder*="인증번호"]').count(),
  visibleMessages: (await page.locator('body').innerText()).split("\n").map((line) => line.trim()).filter(Boolean).filter((line) => /인증|오류|실패|발송|전송/.test(line)).slice(0, 12),
  buttons: buttons.map((label) => label.trim()).filter(Boolean).slice(0, 15),
}, null, 2)}\n`);
process.exit(0);
