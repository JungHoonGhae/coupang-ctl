import { chromium } from "playwright-core";

const email = process.env.COUPANG_EMAIL;
const password = process.env.COUPANG_PASSWORD;
if (!email || !password) throw new Error("Missing Doppler-injected Coupang credentials");

const browser = await chromium.connectOverCDP(process.env.COUPANG_CDP_URL ?? "http://127.0.0.1:9223");
const context = browser.contexts()[0];
const page = await context.newPage();
await page.goto("https://login.coupang.com/login/login.pang", {
  waitUntil: "domcontentloaded",
  timeout: 20_000,
});

const emailInput = page.locator('input[type="email"], input[name*="email" i], #login-email-input').first();
const passwordInput = page.locator('input[type="password"]').first();
await emailInput.fill(email);
await passwordInput.fill(password);
await page.locator('button[type="submit"], button:has-text("로그인"), input[type="submit"]').first().click();
await page.waitForTimeout(3_000);

const state = {
  url: page.url(),
  title: await page.title(),
  loginFormVisible: await passwordInput.isVisible().catch(() => false),
  challengeVisible: await page.locator('text=/인증|보안|captcha|자동입력|본인확인/i').count(),
  errorText: await page.locator('[class*="error" i], [role="alert"]').allTextContents().catch(() => []),
};
process.stdout.write(`${JSON.stringify(state, null, 2)}\n`);
process.exit(0);
