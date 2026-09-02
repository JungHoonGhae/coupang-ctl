import { spawn, spawnSync } from "node:child_process";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { createServer } from "node:net";
import { homedir, tmpdir } from "node:os";
import { join } from "node:path";
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

type SampleRef = { productID: string; vendorItemID: string };

const stateRoot = process.env.COUPANGCTL_STATE_DIR ?? join(homedir(), "Library", "Application Support", "coupangctl");
const sampleLimit = 3;
const samples = loadSampleRefs(join(stateRoot, "coupangctl.sqlite3"), sampleLimit);
const stored = JSON.parse(await readFile(join(stateRoot, "session.json"), "utf8")) as { cookies: StoredCookie[] };
const profile = await mkdtemp(join(tmpdir(), "coupangctl-category-probe-"));
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

  const observations = [];
  for (let index = 0; index < samples.length; index++) {
    observations.push(await inspectSample(context, samples[index], index + 1));
  }
  process.stdout.write(`${JSON.stringify({
    probe: "category_metadata",
    browser: "installed_chrome_headed",
    requestedSamples: sampleLimit,
    observedSamples: observations.length,
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

async function inspectSample(context: BrowserContext, sample: SampleRef, sampleNumber: number) {
  const page = await context.newPage();
  const networkCategoryPaths = new Set<string>();
  const responseTasks: Promise<void>[] = [];
  page.on("response", (response) => {
    const contentType = response.headers()["content-type"] ?? "";
    if (!contentType.includes("json")) return;
    responseTasks.push((async () => {
      try {
        const parsed = await response.json();
        for (const path of categoryKeyPaths(parsed, 5)) networkCategoryPaths.add(path);
      } catch {
        // Shape-only probe: unreadable or oversized responses are ignored.
      }
    })());
  });

  try {
    const response = await page.goto(
      `https://www.coupang.com/vp/products/${sample.productID}?vendorItemId=${sample.vendorItemID}`,
      { waitUntil: "domcontentloaded", timeout: 25_000 },
    );
    await page.waitForTimeout(1_500);
    await Promise.allSettled(responseTasks);
    const documentMetadata = await inspectDocument(page) as Record<string, unknown>;
    return {
      sample: sampleNumber,
      status: response?.status() ?? 0,
      finalHost: new URL(page.url()).hostname,
      ...documentMetadata,
      networkCategoryPaths: [...networkCategoryPaths].sort().slice(0, 80),
    };
  } finally {
    await page.close();
  }
}

async function inspectDocument(page: Page) {
  return await page.evaluate(`(() => {
    const typeOf = (value) => Array.isArray(value) ? "array" : value === null ? "null" : typeof value;
    const paths = (root, maxDepth) => {
      const found = new Set();
      const walk = (value, path, depth) => {
        if (depth > maxDepth || value === null || typeof value !== "object") return;
        for (const [key, child] of Object.entries(value)) {
          const childPath = path ? path + "." + key : key;
          if (/category|breadcrumb|taxonomy/i.test(key)) found.add(childPath + ":" + typeOf(child));
          if (Array.isArray(child)) {
            if (child.length > 0) walk(child[0], childPath + "[]", depth + 1);
          } else {
            walk(child, childPath, depth + 1);
          }
        }
      };
      walk(root, "", 0);
      return [...found].sort().slice(0, 80);
    };

    let nextDataPaths = [];
    const nextData = document.querySelector("script#__NEXT_DATA__")?.textContent;
    if (nextData) {
      try { nextDataPaths = paths(JSON.parse(nextData), 8); } catch {}
    }

    let jsonLDDocumentCount = 0;
    let breadcrumbListCount = 0;
    const breadcrumbItemCounts = [];
    const breadcrumbItemShapes = [];
    const jsonLDCategoryPaths = new Set();
    for (const script of document.querySelectorAll('script[type="application/ld+json"]')) {
      try {
        const parsed = JSON.parse(script.textContent ?? "null");
        const documents = Array.isArray(parsed) ? parsed : [parsed];
        for (const item of documents) {
          jsonLDDocumentCount++;
          for (const path of paths(item, 8)) jsonLDCategoryPaths.add(path);
          if (item?.["@type"] === "BreadcrumbList") {
            breadcrumbListCount++;
            const elements = Array.isArray(item.itemListElement) ? item.itemListElement : [];
            breadcrumbItemCounts.push(elements.length);
            if (elements.length > 0) {
              breadcrumbItemShapes.push(Object.fromEntries(
                Object.entries(elements[0]).map(([key, value]) => [key, typeOf(value)])
              ));
            }
          }
        }
      } catch {}
    }

    const breadcrumbAnchors = [...document.querySelectorAll(
      '[class*="breadcrumb" i] a, nav[aria-label*="breadcrumb" i] a, a[href*="/np/categories/"]'
    )];
    const hrefShapes = [...new Set(breadcrumbAnchors.map((anchor) => {
      try {
        const url = new URL(anchor.getAttribute("href") ?? "", location.href);
        return {
          host: url.hostname,
          pathShape: url.pathname.replace(/\\d+/g, "<id>"),
          queryNames: [...url.searchParams.keys()].sort(),
        };
      } catch { return null; }
    }).filter(Boolean).map((value) => JSON.stringify(value)))].map((value) => JSON.parse(value));

    return {
      titlePresent: document.title.length > 0,
      nextDataPaths,
      jsonLDDocumentCount,
      breadcrumbListCount,
      breadcrumbItemCounts,
      breadcrumbItemShapes,
      jsonLDCategoryPaths: [...jsonLDCategoryPaths].sort(),
      domBreadcrumbAnchorCount: breadcrumbAnchors.length,
      domBreadcrumbHrefShapes: hrefShapes.slice(0, 20),
    };
  })()`);
}

function categoryKeyPaths(root: unknown, maxDepth: number): string[] {
  const found = new Set<string>();
  const walk = (value: unknown, path: string, depth: number) => {
    if (depth > maxDepth || value === null || typeof value !== "object") return;
    for (const [key, child] of Object.entries(value as Record<string, unknown>)) {
      const childPath = path ? `${path}.${key}` : key;
      if (/category|breadcrumb|taxonomy/i.test(key)) found.add(`${childPath}:${jsonType(child)}`);
      if (Array.isArray(child)) {
        if (child.length > 0) walk(child[0], `${childPath}[]`, depth + 1);
      } else {
        walk(child, childPath, depth + 1);
      }
    }
  };
  walk(root, "", 0);
  return [...found];
}

function loadSampleRefs(databasePath: string, limit: number): SampleRef[] {
  const query = `WITH products AS (
      SELECT MAX(product_id) AS product_id, vendor_item_id
      FROM order_items
      WHERE COALESCE(product_id, '') != '' AND COALESCE(vendor_item_id, '') != ''
      GROUP BY vendor_item_id
    )
    SELECT products.product_id || '|' || products.vendor_item_id
    FROM products LEFT JOIN product_categories cached
      ON cached.product_key = 'vendor:' || products.vendor_item_id
    WHERE cached.product_key IS NULL
      OR (cached.source = 'coupang_product_jsonld_breadcrumb_v1' AND cached.breadcrumb_json = '[]')
    ORDER BY products.vendor_item_id LIMIT ${limit};`;
  const result = spawnSync("sqlite3", [databasePath, query], { encoding: "utf8" });
  if (result.status !== 0) throw new Error("could not load private sample references");
  return result.stdout.trim().split("\n").filter(Boolean).map((line) => {
    const [productID, vendorItemID] = line.split("|");
    if (!/^\d+$/.test(productID) || !/^\d+$/.test(vendorItemID)) throw new Error("invalid private sample reference");
    return { productID, vendorItemID };
  });
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
    } catch {}
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error("Chrome CDP unavailable");
}
