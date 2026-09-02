import { mkdtemp, readFile, rm } from "node:fs/promises";
import { homedir, tmpdir } from "node:os";
import { join } from "node:path";
import { spawn } from "node:child_process";
import { createServer } from "node:net";
import { chromium, type BrowserContext } from "playwright-core";

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

const stateRoot = process.env.COUPANGCTL_STATE_DIR ?? join(homedir(), "Library", "Application Support", "coupangctl");
const stored = JSON.parse(await readFile(join(stateRoot, "session.json"), "utf8")) as { cookies: StoredCookie[] };
const profile = await mkdtemp(join(tmpdir(), "coupangctl-pagination-"));
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
  const observedOrderResponses: Array<{method: string; path: string; query: Record<string, string>; status: number}> = [];
  page.on("response", (candidate) => {
    const parsed = new URL(candidate.url());
    if (parsed.hostname !== "mc.coupang.com" || !/order/i.test(parsed.pathname)) return;
    observedOrderResponses.push({
      method: candidate.request().method(),
      path: parsed.pathname,
      query: Object.fromEntries([...parsed.searchParams.entries()].filter(([, value]) => /^\d+$/.test(value))),
      status: candidate.status(),
    });
  });
  const response = await page.goto("https://mc.coupang.com/ssr/desktop/order/list?pageIndex=9&periodYear=2026", {
    waitUntil: "domcontentloaded",
    timeout: 20_000,
  });
  if (!response || response.status() >= 400) {
    throw new Error(`protected page unavailable (${response?.status() ?? 0})`);
  }
  await page.waitForTimeout(1_000);
  // A string expression keeps tsx/esbuild's helper functions out of the page realm.
  const controls = await page.evaluate(`(() => {
    const result = [];
    for (const element of document.querySelectorAll("a,button")) {
      const raw = element.getAttribute("href");
      let target = null;
      if (raw) {
        const parsed = new URL(raw, location.href);
        if (parsed.hostname === "mc.coupang.com" && parsed.pathname === "/ssr/desktop/order/list") {
          target = {
            path: parsed.pathname,
            query: Object.fromEntries([...parsed.searchParams.entries()].filter(([, value]) => /^\\d+$/.test(value))),
          };
        }
      }
      const label = (element.getAttribute("aria-label") ?? element.textContent ?? "").trim();
      const classHasPagination = /pag|next|prev/i.test(element.className?.toString() ?? "");
      const hasDirectionalLabel = /^(다음|이전|next|previous)$/i.test(label);
      if (target || classHasPagination || hasDirectionalLabel) {
        result.push({
          tag: element.tagName.toLowerCase(),
          label: hasDirectionalLabel ? label : "",
          target,
          className: (element.className?.toString() ?? "").slice(0, 160),
          ariaDisabled: element.getAttribute("aria-disabled"),
          classHasPagination,
        });
      }
    }
    return result;
  })()`);
  const nextButton = page.getByRole("button", { name: "다음", exact: true }).last();
  let afterNextQuery: Record<string, string> | null = null;
  let afterNextState: unknown = null;
  let modelShape: unknown = null;
  if (await nextButton.count()) {
    observedOrderResponses.length = 0;
    const modelResponsePromise = page.waitForResponse((candidate) => {
      const parsed = new URL(candidate.url());
      return parsed.hostname === "mc.coupang.com" && parsed.pathname === "/ssr/api/myorders/model";
    });
    await nextButton.click();
    const modelResponse = await modelResponsePromise;
    modelShape = describe(await modelResponse.json(), 4);
    afterNextQuery = Object.fromEntries(
      [...new URL(page.url()).searchParams.entries()].filter(([, value]) => /^\d+$/.test(value)),
    );
    afterNextState = await page.evaluate(`(() => {
      const script = document.querySelector("script#__NEXT_DATA__");
      if (!script?.textContent) return null;
      const root = JSON.parse(script.textContent);
      const pagination = root?.props?.pageProps?.domains?.desktopOrder?.orderPagination ?? {};
      const query = {};
      for (const [key, value] of Object.entries(root?.query ?? {})) {
        if ((typeof value === "string" && /^\\d+$/.test(value)) || typeof value === "number") query[key] = value;
      }
      return {
        query,
        pagination: Object.fromEntries(Object.entries(pagination).filter(([key, value]) =>
          ["hasNext", "hasPrev", "nextPageIndex", "nextYear", "prevPageIndex", "prevYear"].includes(key) &&
          (typeof value === "boolean" || typeof value === "number")
        )),
      };
    })()`);
  }
  process.stdout.write(`${JSON.stringify({
    urlQuery: Object.fromEntries(new URL(response.url()).searchParams),
    controls,
    afterNextQuery,
    observedOrderResponses,
    modelShape,
    afterNextState,
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
    return {
      type: "array",
      length: value.length,
      ...(value.length > 0 ? { item: describe(value[0], depth - 1) } : {}),
    };
  }
  if (value !== null && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>)
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([key, child]) => [key, describe(child, depth - 1)]),
    );
  }
  return jsonType(value);
}

function jsonType(value: unknown): string {
  if (value === null) return "null";
  if (Array.isArray(value)) return "array";
  return typeof value;
}
