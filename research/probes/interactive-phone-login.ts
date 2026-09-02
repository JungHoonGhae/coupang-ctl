import { chmod, mkdir } from "node:fs/promises";
import { chromium } from "playwright-core";

const phone = process.env.COUPANG_PHONE;
if (!phone) throw new Error("Missing Doppler-injected COUPANG_PHONE");

const profileDir = process.env.COUPANG_PROFILE_DIR ?? "/tmp/coupangctl-auth-profile";
const timeoutMs = Number(process.env.COUPANG_LOGIN_TIMEOUT_MS ?? 10 * 60_000);

await mkdir(profileDir, { recursive: true, mode: 0o700 });
await chmod(profileDir, 0o700);
const context = await chromium.launchPersistentContext(profileDir, {
  headless: false,
  channel: "chrome",
});
const page = await context.newPage();

try {
  const response = await page.goto("https://login.coupang.com/login/login.pang", {
    waitUntil: "domcontentloaded",
    timeout: 20_000,
  });
  if (!response || response.status() >= 400) {
    throw new Error(`Login page returned HTTP ${response?.status() ?? "unknown"}`);
  }

  await page.getByText("휴대폰번호 로그인", { exact: true }).first().click();
  const phoneInput = page.locator(
    'input[type="tel"], input[name*="phone" i], input[placeholder*="휴대폰"]',
  ).first();
  await phoneInput.fill(phone);

  process.stdout.write(`${JSON.stringify({
    status: "waiting-for-user",
    action: "Complete the visible CAPTCHA, request the SMS, enter the OTP, and finish login in Chrome.",
    timeoutSeconds: Math.floor(timeoutMs / 1000),
  })}\n`);

  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const loggedIn = !new URL(page.url()).hostname.startsWith("login.");
    if (loggedIn) {
      process.stdout.write(`${JSON.stringify({
        status: "ok",
        profileDir,
        cookieCount: (await context.cookies()).length,
      })}\n`);
      process.exitCode = 0;
      break;
    }
    await page.waitForTimeout(1_000);
  }

  if (new URL(page.url()).hostname.startsWith("login.")) {
    process.stdout.write(`${JSON.stringify({
      status: "error",
      stage: "interactive-login",
      message: "Timed out before login completed.",
    })}\n`);
    process.exitCode = 1;
  }
} finally {
  await context.close();
}
