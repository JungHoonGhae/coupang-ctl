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
  let quantityObservation: unknown = null;
  page.on("response", async (response) => {
    const parsed = new URL(response.url());
    if (parsed.hostname !== "www.coupang.com" || parsed.pathname !== "/next-api/products/quantity-info") return;
    const contentType = response.headers()["content-type"] ?? "";
    try {
      const body = await response.body();
      if (body.length > 1_000_000) {
        quantityObservation = { status: response.status(), contentType, bodyBytes: body.length, tooLarge: true };
        return;
      }
      const parsedBody = JSON.parse(body.toString("utf8")) as unknown;
      quantityObservation = {
        status: response.status(), contentType, bodyBytes: body.length, jsonReadable: true,
        shape: shape(parsedBody),
      };
    } catch {
      quantityObservation = { status: response.status(), contentType, jsonReadable: false };
    }
  });
  const response = await page.goto(
    `https://www.coupang.com/vp/products/${productID}?vendorItemId=${vendorItemID}`,
    { waitUntil: "domcontentloaded", timeout: 25_000 },
  );
  await page.waitForTimeout(2_000);
  if (quantityObservation === null || (quantityObservation as { jsonReadable?: boolean }).jsonReadable === false) {
    quantityObservation = await page.evaluate(async ({ productID, vendorItemID }) => {
      const response = await fetch(
        `/next-api/products/quantity-info?productId=${encodeURIComponent(productID)}&vendorItemId=${encodeURIComponent(vendorItemID)}`,
        { credentials: "include", headers: { accept: "application/json" } },
      );
      const text = (await response.text()).slice(0, 1_000_001);
      let parsedBody: unknown = null;
      let jsonReadable = false;
      try {
        parsedBody = JSON.parse(text) as unknown;
        jsonReadable = true;
      } catch {}
      let responseShape: unknown = null;
      if (jsonReadable) {
        if (Array.isArray(parsedBody)) {
          const first = parsedBody.length > 0 ? parsedBody[0] : undefined;
          responseShape = {
            type: "array",
            length: parsedBody.length,
            itemType: Array.isArray(first) ? "array" : first === null ? "null" : typeof first,
          };
        } else if (parsedBody !== null && typeof parsedBody === "object") {
          responseShape = { type: "object", keys: Object.keys(parsedBody as Record<string, unknown>).sort().slice(0, 100) };
        } else {
          responseShape = parsedBody === null ? "null" : typeof parsedBody;
        }
      }
      return {
        status: response.status,
        contentType: response.headers.get("content-type") ?? "",
        bodyBytes: text.length,
        truncated: text.length > 1_000_000,
        jsonReadable,
        shape: responseShape,
      };
    }, { productID, vendorItemID });
  }
  const optionDocumentShape = await page.evaluate(() => {
    const candidates = [...document.querySelectorAll<HTMLElement>('[class*="option" i], [class*="selected" i], [data-vendor-item-id]')];
    return candidates.slice(0, 120).map((element) => {
      const text = (element.textContent ?? "").replace(/\s+/g, " ").trim();
      const ancestorClasses: string[] = [];
      let ancestor = element.parentElement;
      for (let depth = 0; ancestor && depth < 5; depth++, ancestor = ancestor.parentElement) {
        ancestorClasses.push(...ancestor.classList);
      }
      return {
        tag: element.tagName,
        classes: [...element.classList].sort(),
        ancestorClasses: [...new Set(ancestorClasses)].sort().slice(0, 100),
        attributeNames: element.getAttributeNames().filter((name) => name !== "style").sort(),
        childCount: element.children.length,
        textLength: text.length,
        hasCapacity: /\b\d{1,4}\s*(?:GB|TB)\b/i.test(text),
        hasComputerPart: /RTX|GTX|RADEON|VEGA|RYZEN|라이젠|코어|ULTRA/i.test(text),
        selected: element.getAttribute("aria-selected") === "true" || element.classList.contains("selected"),
      };
    });
  });
  const benefitDocumentSignals = await page.evaluate(() => {
    const bodyText = (document.body?.innerText ?? "").replace(/\s+/g, " ");
    const candidates = [...document.querySelectorAll<HTMLElement>("div,span,p,li,strong,button")]
      .filter((element) => {
        const text = (element.textContent ?? "").replace(/\s+/g, " ").trim();
        return text.length >= 2 && text.length <= 300 && /카드.{0,24}(?:할인|혜택)|카드할인/.test(text);
      });
    const tags: Record<string, number> = {};
    for (const element of candidates) tags[element.tagName] = (tags[element.tagName] ?? 0) + 1;
    return {
      bodyHasCardBenefitText: /카드.{0,24}(?:할인|혜택)|카드할인/.test(bodyText),
      cardSignalCount: candidates.length,
      cardSignalTags: tags,
      cardSignalClassTokens: [...new Set(candidates.flatMap((element) => [...element.classList]))].sort().slice(0, 100),
      currentCardSelectorCount: document.querySelectorAll('[class*="cardBenefit"]').length,
      couponClassSelectorCount: document.querySelectorAll('[class*="coupon"]').length,
      promotionClassSelectorCount: document.querySelectorAll('[class*="promotion"]').length,
    };
  });
  process.stdout.write(`${JSON.stringify({
    probe: "product_option_metadata",
    browser: headed ? "installed_chrome_headed" : "installed_chrome_headless",
    status: response?.status() ?? 0,
    finalHost: new URL(page.url()).hostname,
    quantityInfo: quantityObservation,
    optionDocumentShape,
    benefitDocumentSignals,
  }, null, 2)}\n`);
} finally {
  await browser.close();
}
