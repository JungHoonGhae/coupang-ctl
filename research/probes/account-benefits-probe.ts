import { spawn } from "node:child_process";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { createServer } from "node:net";
import { homedir, tmpdir } from "node:os";
import { join } from "node:path";
import { chromium, type BrowserContext, type Page, type Response } from "playwright-core";

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

type NetworkObservation = {
  method: string;
  origin: string;
  path: string;
  queryNames: string[];
  status: number;
  contentType: string;
  shape?: unknown;
  relevantKeyPaths?: string[];
};

const stateRoot = process.env.COUPANGCTL_STATE_DIR ?? join(homedir(), "Library", "Application Support", "coupangctl");
const stored = JSON.parse(await readFile(join(stateRoot, "session.json"), "utf8")) as { cookies: StoredCookie[] };
const profile = await mkdtemp(join(tmpdir(), "coupangctl-account-benefits-"));
const port = await availablePort();
const headed = process.env.COUPANG_PROBE_HEADED === "1";
const chromeArguments = [
  `--user-data-dir=${profile}`,
  "--remote-debugging-address=127.0.0.1",
  `--remote-debugging-port=${port}`,
  "--no-first-run",
  "--no-default-browser-check",
  "about:blank",
];
if (!headed) chromeArguments.unshift("--headless=new");
const chrome = spawn("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", chromeArguments, { stdio: "ignore" });

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

  const observations: unknown[] = [];
  const home = await inspectPage(context, "https://mc.coupang.com/ssr/desktop/order/list");
  observations.push(home.summary);
  const accountSeeds = [
    "https://loyalty.coupang.com/loyalty/management/home",
    "https://cash.coupang.com/coupang-cash/home",
  ];
  const seedResults = [];
  for (const url of accountSeeds) {
    const inspected = await inspectPage(context, url);
    seedResults.push(inspected);
    observations.push(inspected.summary);
  }
  const discovered = [...new Set([home, ...seedResults]
    .flatMap((result) => result.relevantLinks)
    .filter((link) => allowedAccountURL(link))
    .filter((link) => !accountSeeds.includes(link)))]
    .slice(0, 8);
  for (const url of discovered) observations.push((await inspectPage(context, url)).summary);

  process.stdout.write(`${JSON.stringify({
    probe: "account_benefits_metadata",
    browser: headed ? "installed_chrome_headed" : "installed_chrome_headless",
    authenticatedHome: !home.summary.redirectedToLogin,
    discoveredPageCount: discovered.length,
    observations,
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

async function inspectPage(context: BrowserContext, targetURL: string): Promise<{
  summary: Record<string, unknown>;
  relevantLinks: string[];
}> {
  const page = await context.newPage();
  const calls: NetworkObservation[] = [];
  const loadedScripts = new Set<string>();
  const scriptRouteShapes = new Set<string>();
  const tasks: Promise<void>[] = [];
  page.on("response", (response) => {
    const request = response.request();
    const contentType = response.headers()["content-type"] ?? "";
    if (request.resourceType() === "script") {
      loadedScripts.add(response.url());
      tasks.push(observeScriptRoutes(response).then((routes) => {
        for (const route of routes) scriptRouteShapes.add(route);
      }));
      return;
    }
    if (!["xhr", "fetch"].includes(request.resourceType()) && !contentType.includes("json")) return;
    tasks.push(observeResponse(response).then((entry) => {
      calls.push(entry);
    }));
  });

  try {
    const response = await page.goto(targetURL, { waitUntil: "domcontentloaded", timeout: 30_000 }).catch(() => undefined);
    await page.waitForTimeout(2_000);
    await Promise.allSettled(tasks);
    const dom = await inspectDOM(page);
    const finalURL = new URL(page.url());
    return {
      summary: {
        requested: safeURL(targetURL),
        status: response?.status() ?? 0,
        finalOrigin: finalURL.origin,
        finalPath: redactPath(finalURL.pathname),
        redirectedToLogin: finalURL.hostname === "login.coupang.com",
        domSignals: dom.signals,
        jsonStateKeyPaths: dom.jsonStateKeyPaths,
        selectedStateShapes: dom.selectedStateShapes,
        relevantLinkShapes: dom.relevantLinks.map(safeURL),
        scriptSources: [...new Set([...dom.scriptSources, ...loadedScripts])].map(safeURL).slice(0, 80),
        scriptRouteShapes: [...scriptRouteShapes].sort().slice(0, 160),
        network: uniqueCalls(calls),
      },
      relevantLinks: dom.relevantLinks,
    };
  } finally {
    await page.close();
  }
}

async function observeScriptRoutes(response: Response): Promise<string[]> {
  if (response.status() !== 200) return [];
  try {
    const text = await response.text();
    if (text.length > 8_000_000) return [];
    const routes = new Set<string>();
    const pattern = /["'](\/[^"'\\\s]{2,180})["']/g;
    for (const match of text.matchAll(pattern)) {
      const candidate = match[1];
      if (!/membership|loyalty|subscription|billing|payment|renew|fee|history|invoice|charge/i.test(candidate)) continue;
      try {
        const parsed = new URL(candidate, response.url());
        if (!/(^|\.)coupang\.com$/.test(parsed.hostname)) continue;
        routes.add(`${parsed.origin}${redactPath(parsed.pathname)}${parsed.searchParams.size > 0 ? `?<${[...parsed.searchParams.keys()].sort().join(",")}>` : ""}`);
      } catch {}
    }
    return [...routes];
  } catch {
    return [];
  }
}

async function observeResponse(response: Response): Promise<NetworkObservation> {
  const request = response.request();
  const parsedURL = new URL(response.url());
  const contentType = response.headers()["content-type"] ?? "";
  const observation: NetworkObservation = {
    method: request.method(),
    origin: parsedURL.origin,
    path: redactPath(parsedURL.pathname),
    queryNames: [...parsedURL.searchParams.keys()].sort(),
    status: response.status(),
    contentType: contentType.split(";")[0],
  };
  if (!contentType.includes("json")) return observation;
  try {
    const body = await response.json();
    observation.shape = jsonShape(body, 4);
    observation.relevantKeyPaths = relevantKeyPaths(body, 6);
  } catch {
    observation.shape = { unreadable: true };
  }
  return observation;
}

async function inspectDOM(page: Page): Promise<{
  signals: Record<string, boolean | number>;
  relevantLinks: string[];
  scriptSources: string[];
  jsonStateKeyPaths: string[];
  selectedStateShapes: Record<string, unknown>;
}> {
  return await page.evaluate(`(() => {
    const text = document.body?.innerText ?? "";
    const patterns = {
      wowMembership: /와우\\s*멤버십/i,
      membershipFee: /월회비|멤버십\\s*요금/i,
      nextBilling: /다음\\s*결제|결제\\s*예정/i,
      membershipCancel: /멤버십\\s*해지|해지하기/i,
      membershipJoin: /멤버십\\s*(가입|시작)|와우\\s*시작/i,
      paymentMethod: /결제\\s*수단|쿠페이/i,
      wowCard: /와우\\s*카드/i,
      coupangCash: /쿠팡\\s*캐시|적립/i,
      benefit: /혜택|무료\\s*배송|무료\\s*반품/i,
      savings: /절약|아낀|배송비|반품비/i,
      benefitThreeMonthWindow: /(?:최근|지난)\\s*3개월|3개월\\s*누적/i,
    };
    const relevantLinks = [...document.querySelectorAll("a[href]")]
      .filter((anchor) => /와우|멤버십|결제\\s*수단|쿠페이|쿠팡\\s*캐시|적립|혜택/i.test(anchor.textContent ?? ""))
      .map((anchor) => {
        try { return new URL(anchor.getAttribute("href") ?? "", location.href).href; }
        catch { return ""; }
      })
      .filter(Boolean);
    const scriptSources = [...document.querySelectorAll("script[src]")]
      .map((script) => {
        try { return new URL(script.getAttribute("src") ?? "", location.href).href; }
        catch { return ""; }
      })
      .filter(Boolean);
    const statePaths = new Set();
    let nextState = null;
    const walk = (value, path, depth) => {
      if (depth > 8 || value === null || typeof value !== "object") return;
      for (const [key, child] of Object.entries(value)) {
        const childPath = path ? path + "." + key : key;
        if (/membership|wow|reward|cash|card|payment|benefit|subscription|saving|fee|billing|renew/i.test(key)) {
          statePaths.add(childPath + ":" + (Array.isArray(child) ? "array" : child === null ? "null" : typeof child));
        }
        if (Array.isArray(child)) {
          if (child.length > 0) walk(child[0], childPath + "[]", depth + 1);
        } else {
          walk(child, childPath, depth + 1);
        }
      }
    };
    for (const script of document.querySelectorAll('script[type="application/json"], script#__NEXT_DATA__')) {
      try {
        const parsed = JSON.parse(script.textContent ?? "null");
        walk(parsed, "", 0);
        if (script.id === "__NEXT_DATA__") nextState = parsed;
      } catch {}
    }
    const shape = (value, depth) => {
      if (depth < 0) return Array.isArray(value) ? "array" : value === null ? "null" : typeof value;
      if (Array.isArray(value)) return { type: "array", length: value.length, item: value.length > 0 ? shape(value[0], depth - 1) : undefined };
      if (value === null || typeof value !== "object") return value === null ? "null" : typeof value;
      return Object.fromEntries(Object.entries(value).slice(0, 100).map(([key, child]) => [key, shape(child, depth - 1)]));
    };
    const data = nextState?.props?.pageProps?.data ?? nextState?.query?.data ?? null;
    return {
      signals: {
        ...Object.fromEntries(Object.entries(patterns).map(([key, pattern]) => [key, pattern.test(text)])),
        relevantAnchorCount: relevantLinks.length,
      },
      relevantLinks: [...new Set(relevantLinks)].slice(0, 20),
      scriptSources: [...new Set(scriptSources)].slice(0, 40),
      jsonStateKeyPaths: [...statePaths].sort().slice(0, 160),
      selectedStateShapes: data ? {
        loyaltyMemberInfo: shape(data.loyaltyMemberInfo, 4),
        loyaltyFeeChangeDate: {
          type: Array.isArray(data.loyaltyFeeChangeDate) ? "array" : data.loyaltyFeeChangeDate === null ? "null" : typeof data.loyaltyFeeChangeDate,
          positive: typeof data.loyaltyFeeChangeDate === "number" && data.loyaltyFeeChangeDate > 0,
          plausibleEpochMillis: typeof data.loyaltyFeeChangeDate === "number" && data.loyaltyFeeChangeDate >= 946684800000 && data.loyaltyFeeChangeDate <= 4102444800000,
        },
        paymentMethod: shape(data.paymentMethod, 4),
        paymentMethods: shape(data.paymentMethods, 4),
        wowBenefitUsage: shape(data.wowBenefitUsage, 5),
      } : {},
    };
  })()`);
}

function jsonShape(value: unknown, depth: number): unknown {
  if (depth < 0) return jsonType(value);
  if (Array.isArray(value)) return { type: "array", length: value.length, item: value.length > 0 ? jsonShape(value[0], depth - 1) : undefined };
  if (value === null || typeof value !== "object") return jsonType(value);
  return Object.fromEntries(Object.entries(value as Record<string, unknown>).slice(0, 80).map(([key, child]) => [key, jsonShape(child, depth - 1)]));
}

function relevantKeyPaths(root: unknown, maxDepth: number): string[] {
  const result = new Set<string>();
  const walk = (value: unknown, path: string, depth: number) => {
    if (depth > maxDepth || value === null || typeof value !== "object") return;
    for (const [key, child] of Object.entries(value as Record<string, unknown>)) {
      const childPath = path ? `${path}.${key}` : key;
      if (/membership|memberShip|wow|reward|cash|card|payment|benefit|subscription|saving|fee|billing|renew|instrument|issuer|bank|account/i.test(key)) {
        result.add(`${childPath}:${jsonType(child)}`);
      }
      if (Array.isArray(child)) {
        if (child.length > 0) walk(child[0], `${childPath}[]`, depth + 1);
      } else {
        walk(child, childPath, depth + 1);
      }
    }
  };
  walk(root, "", 0);
  return [...result].sort().slice(0, 120);
}

function uniqueCalls(calls: NetworkObservation[]): NetworkObservation[] {
  return [...new Map(calls.map((call) => [`${call.method} ${call.origin}${call.path}`, call])).values()]
    .filter((call) => /coupang\.com$/.test(new URL(call.origin).hostname))
    .filter((call) => /wow|member|loyalty|pay|cash|benefit|subscription|card|account|bank/i.test(call.path) || (call.relevantKeyPaths?.length ?? 0) > 0)
    .slice(0, 80);
}

function allowedAccountURL(value: string): boolean {
  try {
    const url = new URL(value);
    return /(^|\.)coupang\.com$/.test(url.hostname) && /wow|member|loyalty|pay|cash|benefit/i.test(`${url.pathname}?${url.search}`);
  } catch {
    return false;
  }
}

function safeURL(value: string): string {
  const url = new URL(value);
  return `${url.origin}${redactPath(url.pathname)}${url.searchParams.size > 0 ? `?<${[...url.searchParams.keys()].sort().join(",")}>` : ""}`;
}

function redactPath(value: string): string {
  return value.replace(/[A-Za-z0-9_-]{16,}/g, "<opaque>").replace(/\d{4,}/g, "<id>");
}

function jsonType(value: unknown): string {
  if (value === null) return "null";
  if (Array.isArray(value)) return "array";
  return typeof value;
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
    } catch {
      // Chrome has not opened its loopback CDP port yet.
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error("Chrome CDP unavailable");
}
