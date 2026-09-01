import { chromium } from "playwright-core";

const otp = process.env.COUPANG_OTP;
if (!otp) throw new Error("Missing ephemeral COUPANG_OTP");
const browser = await chromium.connectOverCDP(process.env.COUPANG_CDP_URL ?? "http://127.0.0.1:9223");
const context = browser.contexts()[0];
const page = context.pages().find((candidate) => candidate.url().includes("login.coupang.com"));
if (!page) throw new Error("No active Coupang login page");

const targetFrame = page.frames().find((frame) => frame.locator('input[placeholder*="인증번호"]').count().catch(() => 0));
let loginFrame = page.mainFrame();
for (const frame of page.frames()) {
  if (await frame.locator('input[placeholder*="인증번호"]').count()) {
    loginFrame = frame;
    break;
  }
}
const otpInput = loginFrame.locator('input[placeholder*="인증번호"]').first();
await otpInput.fill(otp);
const buttons = loginFrame.locator('button:visible');
const labels = await buttons.allTextContents();
const submit = loginFrame.getByRole("button", { name: "로그인", exact: true });
await submit.click();
await page.waitForTimeout(3_000);

process.stdout.write(`${JSON.stringify({
  url: page.url(),
  title: await page.title(),
  loggedIn: !page.url().includes("login.coupang.com"),
  loginPageText: page.url().includes("login.coupang.com")
    ? (await page.locator("body").innerText()).split("\n").map((x) => x.trim()).filter(Boolean).filter((x) => /인증|오류|실패|올바르지|만료/.test(x)).slice(0, 10)
    : [],
  buttonLabels: labels.map((x) => x.trim()).filter(Boolean).slice(0, 15),
}, null, 2)}\n`);
process.exit(0);
