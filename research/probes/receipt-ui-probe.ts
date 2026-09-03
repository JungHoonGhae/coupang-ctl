import { spawn } from "node:child_process";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { homedir, tmpdir } from "node:os";
import { join } from "node:path";
import { createServer } from "node:net";
import { chromium, type BrowserContext, type Page } from "playwright-core";

type StoredCookie = {
  name: string;
  value: string;
  domain: string;
  path: string;
  expires?: number;
  http_only?: boolean;
  secure?: boolean;
  session?: boolean;
  same_site?: "Strict" | "Lax" | "None";
};

type SafeCall = {
  method: string;
  origin: string;
  path: string;
  queryNames: string[];
  status: number;
  shape?: unknown;
};

const stateRoot = process.env.COUPANGCTL_STATE_DIR ?? join(homedir(), "Library", "Application Support", "coupangctl");
const stored = JSON.parse(await readFile(join(stateRoot, "session.json"), "utf8")) as { cookies: StoredCookie[] };
const profile = await mkdtemp(join(tmpdir(), "coupangctl-receipt-"));
const port = await availablePort();
const chrome = spawn("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", [
  `--user-data-dir=${profile}`,
  "--remote-debugging-address=127.0.0.1",
  `--remote-debugging-port=${port}`,
  "--no-first-run",
  "--no-default-browser-check",
  "about:blank",
], { stdio: "ignore" });

try {
  await waitForCDP(port);
  const browser = await chromium.connectOverCDP(`http://127.0.0.1:${port}`);
  const context = browser.contexts()[0] as BrowserContext;
  await context.addCookies(stored.cookies.map((cookie) => ({
    name: cookie.name,
    value: cookie.value,
    domain: cookie.domain,
    path: cookie.path,
    httpOnly: cookie.http_only ?? false,
    secure: cookie.secure ?? false,
    sameSite: cookie.same_site,
    ...(cookie.session || !cookie.expires || cookie.expires <= 0 ? {} : { expires: cookie.expires }),
  })));

  const page = await context.newPage();
  const calls: SafeCall[] = [];
  observeSafeReceiptCalls(page, calls);
  const response = await page.goto("https://mc.coupang.com/ssr/desktop/order/list", {
    waitUntil: "domcontentloaded",
    timeout: 20_000,
  });
  if (!response || response.status() >= 400) throw new Error(`protected page unavailable (${response?.status() ?? 0})`);
  await page.waitForTimeout(1_000);

  const controlsBefore = await receiptControls(page);
  calls.length = 0;
  const trigger = page.locator("a,button").filter({ hasText: /영수증|거래명세서|카드전표/ }).first();
  let popupTarget: ReturnType<typeof safeTarget> | null = null;
  if (await trigger.count()) {
    const popupPromise = context.waitForEvent("page", { timeout: 3_000 }).catch(() => null);
    await trigger.click({ timeout: 5_000 }).catch(() => undefined);
    const popup = await popupPromise;
    await page.waitForTimeout(1_000);
    if (popup) {
      await popup.waitForLoadState("domcontentloaded", { timeout: 5_000 }).catch(() => undefined);
      popupTarget = safeTarget(popup.url());
      await popup.close();
    }
  }
  const controlsAfter = await receiptControls(page);
  const receiptControlShape = await page.locator('button,a,label,[role="tab"],input,select,option').evaluateAll((elements) => elements.flatMap((element) => {
    const text = (element.textContent ?? "").replace(/\s+/g, " ").trim();
    const aria = element.getAttribute("aria-label") ?? "";
    const combined = `${text} ${aria}`.trim();
    if (!/현금|카드|전표|영수증|거래명세|조회|기간/.test(combined)) return [];
    return [{
      tag: element.tagName.toLowerCase(),
      role: element.getAttribute("role") ?? "",
      type: element.getAttribute("type") ?? "",
      name: element.getAttribute("name") ?? "",
      text: combined.replace(/\d+/g, "<n>").slice(0, 120),
    }];
  }));
  const receiptTextSignals = await page.locator("body").evaluate((body) => [...new Set(((body as HTMLElement).innerText ?? "")
    .split(/\n+/)
    .map((line) => line.replace(/\s+/g, " ").trim())
    .filter((line) => line.length > 0 && line.length <= 160)
    .filter((line) => /현금|카드|전표|영수증|거래명세|조회|기간/.test(line))
    .map((line) => line
      .replace(/[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}/g, "<email>")
      .replace(/\d+/g, "<n>")))].slice(0, 80));
  const cardView = page.getByText("신용카드 매출전표", { exact: true }).first();
  let cardViewOpened = false;
  if (await cardView.count()) {
    cardViewOpened = await cardView.click({ timeout: 5_000 }).then(() => true).catch(() => false);
    await page.waitForTimeout(500);
  }
  const buttonKinds = await page.getByRole("button").evaluateAll((buttons) => {
    const counts: Record<string, number> = {};
    for (const button of buttons) {
      const text = (button.textContent ?? "").trim();
      const kind = /조회|검색/.test(text) ? "query"
        : /다운로드|내려받기|엑셀|pdf/i.test(text) ? "download"
        : /신청|요청|발급/.test(text) ? "request"
        : /현금/.test(text) ? "cash"
        : /카드|전표/.test(text) ? "card"
        : /거래명세/.test(text) ? "vendor"
        : "other";
      counts[kind] = (counts[kind] ?? 0) + 1;
    }
    return counts;
  });
  const queryButton = page.getByRole("button", { name: /조회|검색/ }).first();
  if (await queryButton.count()) {
    await queryButton.click({ timeout: 5_000 }).catch(() => undefined);
    await page.waitForTimeout(1_500);
  }
  const formShape = await page.locator("form").evaluateAll((forms) => forms.map((form) => ({
    method: (form.getAttribute("method") ?? "get").toUpperCase(),
    action: (() => {
      const parsed = new URL(form.getAttribute("action") ?? location.href, location.href);
      return { origin: parsed.origin, path: parsed.pathname.replace(/\d{5,}/g, "<redacted>"), queryNames: [...new Set(parsed.searchParams.keys())].sort() };
    })(),
    fields: [...form.querySelectorAll("input,select")].map((field) => ({
      tag: field.tagName.toLowerCase(),
      type: field.getAttribute("type") ?? "",
      name: field.getAttribute("name") ?? "",
    })),
  })));
  const nextDataText = await page.locator("script#__NEXT_DATA__").textContent().catch(() => null);
  let pageStateShape: unknown = null;
  if (nextDataText) {
    const nextData = JSON.parse(nextDataText) as Record<string, any>;
    pageStateShape = describe(nextData?.props?.pageProps?.domains?.paymentReceipt ?? null, 7);
  }
  const safeReads = await page.evaluate(async () => {
    const now = new Date();
    const from = `${now.getFullYear()}.01.01`;
    const to = `${now.getFullYear()}.${String(now.getMonth() + 1).padStart(2, "0")}.${String(now.getDate()).padStart(2, "0")}`;
    const reads = [
      ["cash_history", "/ssr/api/payment-receipt/cash/download-request-histories", { pageIndex: "0", size: "5" }],
      ["card_history", "/ssr/api/payment-receipt/card/download-request-histories", { pageIndex: "0", size: "5" }],
      ["cash_summary", "/ssr/api/payment-receipt/cash/receipt-summary", { from, to }],
      ["card_summary", "/ssr/api/payment-receipt/card/receipt-summary", { from, to, cardId: "", cardNumber: "", displayCardName: "" }],
    ] as const;
    const result: Record<string, unknown> = {};
    for (const [key, path, query] of reads) {
      const target = new URL(path, location.origin);
      for (const [name, value] of Object.entries(query)) target.searchParams.set(name, value);
      const response = await fetch(target, { credentials: "include" });
      let payload: unknown = null;
      try { payload = await response.json(); } catch {}
      result[key] = {
        method: "GET",
        origin: target.origin,
        path: target.pathname,
        queryNames: [...target.searchParams.keys()].sort(),
        status: response.status,
        payload,
      };
    }
    return result;
  });
  const safeReadShapes = Object.fromEntries(Object.entries(safeReads).map(([key, raw]) => {
    const { payload, ...metadata } = raw as Record<string, unknown>;
    const envelope = payload as { success?: unknown; data?: Record<string, unknown> } | null;
    const data = envelope?.data;
    return [key, {
      ...metadata,
      shape: describe(payload, 7),
      facts: {
        success: envelope?.success === true,
        ...(key.endsWith("_history") ? {
          pageIndex: typeof data?.pageIndex === "number" ? data.pageIndex : null,
          pageSize: typeof data?.pageSize === "number" ? data.pageSize : null,
          itemCount: Array.isArray(data?.list) ? data.list.length : null,
        } : {}),
      },
    }];
  }));
  const endpointPaths = new Set<string>();
  const endpointLiterals = new Set<string>();
  const scriptURLs = await page.evaluate(() => [...new Set([
    ...[...document.querySelectorAll("script[src]")].map((script) => (script as HTMLScriptElement).src),
    ...performance.getEntriesByType("resource").map((entry) => entry.name),
  ].filter((source) => /^https:\/\/.+\.js(?:\?|$)/i.test(source)))].slice(0, 100));
  for (const scriptURL of scriptURLs) {
    const scriptResponse = await context.request.get(scriptURL).catch(() => null);
    if (!scriptResponse?.ok()) continue;
    const source = await scriptResponse.text();
    for (const match of source.matchAll(/\/(?:ssr\/api\/)?payment-receipt(?:\/[a-z0-9_./-]+)?/gi)) {
      endpointPaths.add(match[0].split("?")[0]);
    }
    if (!/payment-?receipt|downloadHistories|vendorReceipt/i.test(source)) continue;
    for (const match of source.matchAll(/["'`]([^"'`]{0,160}(?:payment-?receipt|downloadHistories|vendorReceipt)[^"'`]{0,160})["'`]/gi)) {
      const literal = match[1]
        .replace(/\\\//g, "/")
        .replace(/\d{5,}/g, "<redacted>")
        .trim();
      if (literal.length > 0 && literal.length <= 240) endpointLiterals.add(literal);
      if (endpointLiterals.size >= 100) break;
    }
  }
  process.stdout.write(`${JSON.stringify({
    controlsBefore,
    controlsAfter,
    receiptControlShape,
    receiptTextSignals,
    cardViewOpened,
    formShape,
    buttonKinds,
    calls,
    currentTarget: safeTarget(page.url()),
    popupTarget,
    pageStateShape,
    safeReadShapes,
    endpointPaths: [...endpointPaths].sort(),
    endpointLiterals: [...endpointLiterals].sort(),
    scriptTargets: scriptURLs.map(safeTarget),
  }, null, 2)}\n`);
  await browser.close();
} finally {
  if (chrome.exitCode === null) {
    await new Promise<void>((resolve) => {
      chrome.once("exit", () => resolve());
      chrome.kill("SIGTERM");
    });
  }
  await rm(profile, { recursive: true, force: true });
}

function observeSafeReceiptCalls(page: Page, calls: SafeCall[]): void {
  page.on("response", async (response) => {
    const parsed = new URL(response.url());
    if (!/receipt|proof|invoice|cash|card/i.test(`${parsed.pathname}?${parsed.searchParams}`)) return;
    const entry: SafeCall = {
      method: response.request().method(),
      origin: parsed.origin,
      path: parsed.pathname.replace(/\d{5,}/g, "<redacted>"),
      queryNames: [...new Set(parsed.searchParams.keys())].sort(),
      status: response.status(),
    };
    calls.push(entry);
    if ((response.headers()["content-type"] ?? "").includes("json")) {
      try {
        entry.shape = describe(await response.json(), 4);
      } catch {
        entry.shape = { unreadable: true };
      }
    }
  });
}

async function receiptControls(page: Page): Promise<Array<Record<string, unknown>>> {
  return await page.locator("a,button").evaluateAll((elements) => elements.flatMap((element) => {
    const text = (element.textContent ?? "").trim();
    let kind = "";
    if (/거래명세서/.test(text)) kind = "transaction_statement";
    else if (/현금영수증/.test(text)) kind = "cash_receipt";
    else if (/카드.*(전표|영수증)/.test(text)) kind = "card_receipt";
    else if (/영수증/.test(text)) kind = "receipt";
    if (!kind) return [];
    const href = element.getAttribute("href");
    let target = null;
    if (href) {
      const parsed = new URL(href, location.href);
      target = {
        origin: parsed.origin,
        path: parsed.pathname.replace(/\d{5,}/g, "<redacted>"),
        queryNames: [...new Set(parsed.searchParams.keys())].sort(),
      };
    }
    return [{ tag: element.tagName.toLowerCase(), kind, target }];
  }));
}

function safeTarget(raw: string): { origin: string; path: string; queryNames: string[] } {
  const parsed = new URL(raw);
  return {
    origin: parsed.origin,
    path: parsed.pathname.replace(/\d{5,}/g, "<redacted>"),
    queryNames: [...new Set(parsed.searchParams.keys())].sort(),
  };
}

async function availablePort(): Promise<number> {
  return await new Promise((resolve, reject) => {
    const server = createServer();
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      if (!address || typeof address === "string") return reject(new Error("port unavailable"));
      const port = address.port;
      server.close((error) => error ? reject(error) : resolve(port));
    });
  });
}

async function waitForCDP(port: number): Promise<void> {
  for (let attempt = 0; attempt < 100; attempt++) {
    try {
      const response = await fetch(`http://127.0.0.1:${port}/json/version`);
      if (response.ok) return;
    } catch {}
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error("Chrome CDP unavailable");
}

function describe(value: unknown, depth: number): unknown {
  if (depth <= 0) return jsonType(value);
  if (Array.isArray(value)) {
    return { type: "array", length: value.length, ...(value.length > 0 ? { item: describe(value[0], depth - 1) } : {}) };
  }
  if (value !== null && typeof value === "object") {
    return Object.fromEntries(Object.entries(value as Record<string, unknown>)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, child]) => [key, describe(child, depth - 1)]));
  }
  return jsonType(value);
}

function jsonType(value: unknown): string {
  if (value === null) return "null";
  if (Array.isArray(value)) return "array";
  return typeof value;
}
