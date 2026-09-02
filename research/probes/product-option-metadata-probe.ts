import { readFile } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";
import { chromium } from "playwright-core";

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

const productID = process.env.COUPANG_PRODUCT_ID ?? "";
const vendorItemID = process.env.COUPANG_VENDOR_ITEM_ID ?? "";
if (!/^\d{1,24}$/.test(productID) || !/^\d{1,24}$/.test(vendorItemID)) {
  throw new Error("COUPANG_PRODUCT_ID and COUPANG_VENDOR_ITEM_ID must be numeric public identifiers");
}

function typeOf(value: unknown): string {
  if (Array.isArray(value)) return "array";
  if (value === null) return "null";
  return typeof value;
}

function shape(value: unknown, depth = 0): unknown {
  if (depth > 5 || value === null || typeof value !== "object") return typeOf(value);
  if (Array.isArray(value)) {
    return {
      type: "array",
      item: value.length > 0 ? shape(value[0], depth + 1) : "unknown",
    };
  }
  const record = value as Record<string, unknown>;
  const keys = Object.keys(record).sort().slice(0, 100);
  return {
    type: "object",
    fields: Object.fromEntries(keys.map((key) => [key, shape(record[key], depth + 1)])),
  };
}

const headed = process.env.COUPANGCTL_PRODUCT_PROBE_HEADED === "1";
const browser = await chromium.launch({ channel: "chrome", headless: !headed });
try {
  const context = await browser.newContext();
  const stateRoot = process.env.COUPANGCTL_STATE_DIR ?? join(homedir(), "Library", "Application Support", "coupangctl");
  const stored = JSON.parse(await readFile(join(stateRoot, "session.json"), "utf8")) as { cookies: StoredCookie[] };
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
  let quantityShape: unknown = null;
  page.on("response", async (response) => {
    const parsed = new URL(response.url());
    if (parsed.hostname !== "www.coupang.com" || parsed.pathname !== "/next-api/products/quantity-info") return;
    try {
      quantityShape = shape(await response.json());
    } catch {
      quantityShape = { unreadable: true };
    }
  });
  const response = await page.goto(
    `https://www.coupang.com/vp/products/${productID}?vendorItemId=${vendorItemID}`,
    { waitUntil: "domcontentloaded", timeout: 25_000 },
  );
  await page.waitForTimeout(2_000);
  if (quantityShape === null) {
    const fetched = await page.evaluate(async ({ productID, vendorItemID }) => {
      const response = await fetch(
        `/next-api/products/quantity-info?productId=${encodeURIComponent(productID)}&vendorItemId=${encodeURIComponent(vendorItemID)}`,
        { credentials: "include", headers: { accept: "application/json" } },
      );
      return response.ok ? await response.json() : null;
    }, { productID, vendorItemID });
    quantityShape = shape(fetched);
  }
  const optionDocumentShape = await page.evaluate(() => {
    const candidates = [...document.querySelectorAll<HTMLElement>('[class*="option" i], [class*="selected" i], [data-vendor-item-id]')];
    return candidates.slice(0, 120).map((element) => {
      const text = (element.textContent ?? "").replace(/\s+/g, " ").trim();
      return {
        tag: element.tagName,
        classes: [...element.classList].sort(),
        attributeNames: element.getAttributeNames().filter((name) => name !== "style").sort(),
        childCount: element.children.length,
        textLength: text.length,
        hasCapacity: /\b\d{1,4}\s*(?:GB|TB)\b/i.test(text),
        hasComputerPart: /RTX|GTX|RADEON|VEGA|RYZEN|라이젠|코어|ULTRA/i.test(text),
        selected: element.getAttribute("aria-selected") === "true" || element.classList.contains("selected"),
      };
    });
  });
  process.stdout.write(`${JSON.stringify({
    probe: "product_option_metadata",
    browser: headed ? "installed_chrome_headed" : "installed_chrome_headless",
    status: response?.status() ?? 0,
    finalHost: new URL(page.url()).hostname,
    quantityInfoShape: quantityShape,
    optionDocumentShape,
  }, null, 2)}\n`);
} finally {
  await browser.close();
}
