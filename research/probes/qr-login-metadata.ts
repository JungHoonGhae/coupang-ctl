import { chromium, type Response } from "playwright-core";

type Shape =
  | { type: "null" }
  | { type: "array"; length: number; item?: Shape }
  | { type: "object"; keys: Record<string, Shape> }
  | { type: "string" | "number" | "boolean" | "unknown" };

type NetworkRecord = {
  method: string;
  host: string;
  path: string;
  status: number;
  contentType: string | null;
  bodyShape?: Shape;
};

const protectedOrderURL = "https://mc.coupang.com/ssr/desktop/order/list";
const headless = process.env.COUPANGCTL_QR_PROBE_HEADLESS !== "0";
const records: NetworkRecord[] = [];
const qrEndpointPaths = new Set<string>();

function safePath(url: URL): string {
  if (url.pathname === "/login/login.pang" || url.pathname === "/login/qrcode/create.pang") {
    return url.pathname;
  }
  if (url.pathname.startsWith("/resources/")) {
    return url.pathname.replace(/^\/resources\/[^/]+\//, "/resources/<version>/");
  }
  if (url.hostname === "ljc.coupang.com" && url.pathname === "/api/v3/web/submit") {
    return url.pathname;
  }
  return "/<redacted>";
}

function shape(value: unknown, depth = 0): Shape {
  if (value === null) return { type: "null" };
  if (Array.isArray(value)) {
    return {
      type: "array",
      length: value.length,
      ...(value.length > 0 && depth < 3 ? { item: shape(value[0], depth + 1) } : {}),
    };
  }
  if (typeof value === "object") {
    const keys: Record<string, Shape> = {};
    if (depth < 3) {
      for (const key of Object.keys(value as Record<string, unknown>).sort().slice(0, 40)) {
        keys[key] = shape((value as Record<string, unknown>)[key], depth + 1);
      }
    }
    return { type: "object", keys };
  }
  if (typeof value === "string") return { type: "string" };
  if (typeof value === "number") return { type: "number" };
  if (typeof value === "boolean") return { type: "boolean" };
  return { type: "unknown" };
}

async function recordResponse(response: Response): Promise<void> {
  const request = response.request();
  const url = new URL(response.url());
  if (!url.hostname.endsWith("coupang.com")) return;
  const contentType = response.headers()["content-type"] ?? null;
  if (url.pathname.endsWith("/dist/login.min.js")) {
    try {
      const source = await response.text();
      for (const match of source.matchAll(/\/login\/qrcode\/[A-Za-z0-9_.-]+/g)) {
        qrEndpointPaths.add(match[0]);
      }
    } catch {
      // The response shape is still useful if the script body is unavailable.
    }
  }
  const relevant =
    request.resourceType() === "xhr" ||
    request.resourceType() === "fetch" ||
    /login|auth|qr|sso|cert/i.test(url.pathname);
  if (!relevant) return;

  const record: NetworkRecord = {
    method: request.method(),
    host: url.hostname,
    path: safePath(url),
    status: response.status(),
    contentType,
  };
  if (contentType?.includes("json")) {
    try {
      record.bodyShape = shape(await response.json());
    } catch {
      record.bodyShape = { type: "unknown" };
    }
  }
  records.push(record);
}

// This probe intentionally uses a fresh research-only browser profile. It does
// not read credentials, persist a QR image, approve a login, or export session
// state. Production code must not import Playwright.
const browser = await chromium.launch({ channel: "chrome", headless });
const context = await browser.newContext();
const page = await context.newPage();
page.on("response", (response) => void recordResponse(response));

const initial = await page.goto(protectedOrderURL, {
  waitUntil: "domcontentloaded",
  timeout: 25_000,
}).catch(() => null);

let qrTabFound = false;
let qrSurfaceReady = false;
if (initial) {
  const qrTab = page.getByText("QR코드 로그인", { exact: true }).first();
  qrTabFound = (await qrTab.count()) > 0;
  if (qrTabFound) {
    await qrTab.click({ timeout: 5_000 }).catch(() => undefined);
    await page.waitForTimeout(8_000);
    qrSurfaceReady = (await page.getByText(/휴대폰 카메라로 QR코드를 스캔|남은시간/).count()) > 0;
  }
}

await browser.close();
process.stdout.write(`${JSON.stringify({
  browserMode: headless ? "headless" : "headed",
  initialStatus: initial?.status() ?? null,
  loginOriginReached:
    page.url().startsWith("https://login.coupang.com/") ||
    records.some((record) => record.host === "login.coupang.com"),
  qrTabFound,
  qrSurfaceReady,
  qrEndpointPaths: [...qrEndpointPaths].sort(),
  network: records.slice(-40),
}, null, 2)}\n`);
