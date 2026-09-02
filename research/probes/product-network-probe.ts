import { chromium } from "playwright-core";

const cdpURL = process.env.COUPANG_CDP_URL ?? "http://127.0.0.1:9223";

function redactURL(rawURL: string): string {
  const url = new URL(rawURL);
  for (const key of [
    "productId", "itemId", "vendorItemId", "productIds", "itemIds", "vendorItemIds",
    "searchId", "clickEventId",
  ]) {
    if (url.searchParams.has(key)) url.searchParams.set(key, "<redacted>");
  }
  url.pathname = url.pathname.replace(/\/vp\/products\/\d+/, "/vp/products/<redacted>");
  url.pathname = url.pathname.replace(/\/next-api\/vfpbanner\/\d+/, "/next-api/vfpbanner/<redacted>");
  return url.toString();
}

function responseShape(value: unknown, depth = 0): unknown {
  if (value === null) return "null";
  if (Array.isArray(value)) {
    return {
      kind: "array",
      length: value.length,
      item: value.length > 0 && depth < 3 ? responseShape(value[0], depth + 1) : "unknown",
    };
  }
  if (typeof value !== "object") return typeof value;
  const record = value as Record<string, unknown>;
  const keys = Object.keys(record).sort().slice(0, 40);
  return {
    kind: "object",
    keys,
    fields: depth < 3
      ? Object.fromEntries(keys.map((key) => [key, responseShape(record[key], depth + 1)]))
      : undefined,
  };
}

function isProductReadResponse(rawURL: string): boolean {
  const url = new URL(rawURL);
  return (url.hostname === "www.coupang.com" && url.pathname.startsWith("/next-api/")) ||
    (url.hostname === "reco.coupang.com" && url.pathname === "/recommend/widget");
}

const browser = await chromium.connectOverCDP(cdpURL);
const context = browser.contexts()[0];
const page = await context.newPage();
const captured: Array<Record<string, unknown>> = [];

page.on("response", async (response) => {
  if (!isProductReadResponse(response.url())) return;
  const request = response.request();
  const type = request.resourceType();
  const contentType = response.headers()["content-type"] ?? "";
  if (!["xhr", "fetch"].includes(type) && !contentType.includes("json")) return;
  let body: unknown;
  if (contentType.includes("json")) {
    try {
      const parsed = await response.json();
      body = responseShape(parsed);
    } catch {
      body = { unreadable: true };
    }
  }
  captured.push({
    url: redactURL(response.url()),
    method: request.method(),
    type,
    status: response.status(),
    contentType,
    body,
  });
});

await page.goto("https://www.coupang.com/np/search?q=%EC%97%90%EC%96%B4%ED%8C%9F", {
  waitUntil: "domcontentloaded",
  timeout: 20_000,
});
const searchDocument = await page.evaluate(() => {
  const links = [...document.querySelectorAll<HTMLAnchorElement>('a[href*="/vp/products/"]')];
  const first = links[0] ?? null;
  const container = first?.closest("li, article") as HTMLElement | null;
  const classTokens = container
    ? [...container.querySelectorAll<HTMLElement>("[class]")]
        .flatMap((element) => [...element.classList])
        .filter((token, index, all) => all.indexOf(token) === index)
        .filter((token) => /product|price|rating|review|delivery|badge|image|name|discount/i.test(token))
        .slice(0, 40)
    : [];
  const scripts = [...document.scripts].map((script) => ({
    id: script.id || null,
    type: script.type || null,
    bytes: script.textContent?.length ?? 0,
  })).filter((entry) => entry.id || entry.type === "application/ld+json").slice(0, 30);
  const cardStructure = container
    ? [...container.querySelectorAll<HTMLElement>("*")].slice(0, 120).map((element) => {
        const text = (element.childNodes.length === 1 && element.firstChild?.nodeType === Node.TEXT_NODE
          ? element.textContent ?? "" : "").replace(/\s+/g, " ").trim();
        const totalText = (element.textContent ?? "").replace(/\s+/g, " ").trim();
        return {
          tag: element.tagName,
          classes: [...element.classList].sort(),
          ancestorClasses: [...(element.parentElement?.classList ?? [])].sort(),
          zone: element.closest('[class*="PriceArea"]') ? "price" : element.closest('[class*="ProductRating"]') ? "rating" : null,
          ownTextLength: text.length,
          hasWonAmount: /\d[\d,]*\s*원/.test(text),
          hasDecimal: /\d\.\d/.test(text),
          hasParenthesesNumber: /\([\d,]+\)/.test(text),
          hasOnlyNumber: /^[\d,.]+$/.test(text),
          ownDigitCount: (text.match(/\d/g) ?? []).length,
          totalTextLength: totalText.length,
          totalDigitCount: (totalText.match(/\d/g) ?? []).length,
          totalHasParentheses: /\([\d,]+\)/.test(totalText),
          ariaLabelLength: (element.getAttribute("aria-label") ?? "").length,
          titleLength: (element.getAttribute("title") ?? "").length,
          hasInlineWidth: /width\s*:/.test(element.getAttribute("style") ?? ""),
          image: element.tagName === "IMG",
        };
      }).filter((entry) => entry.classes.length > 0 || entry.ownTextLength > 0 || entry.image)
    : [];
  const jsonLDSchema = [...document.querySelectorAll<HTMLScriptElement>('script[type="application/ld+json"]')]
    .map((script) => {
      try {
        const value = JSON.parse(script.textContent ?? "null") as unknown;
        return responseShape(value);
      } catch {
        return { unreadable: true };
      }
    });
  const sortingControls = [...document.querySelectorAll<HTMLElement>('*')]
	.filter((element) => {
	  const label = (element.textContent ?? '').replace(/\s+/g, ' ').trim();
	  return label.length > 0 && label.length <= 40 && /쿠팡\s*랭킹|판매량|낮은\s*가격|높은\s*가격|최신순|상품평|리뷰/.test(label);
	})
    .slice(0, 30)
    .map((element) => {
      const rawHref = element instanceof HTMLAnchorElement ? element.href : '';
      let query: Record<string, string> = {};
      if (rawHref) {
        const parsed = new URL(rawHref, location.origin);
        query = Object.fromEntries([...parsed.searchParams.entries()].map(([key, value]) => [key, key === 'q' ? '<redacted>' : value]));
      }
	  const relatedInput = element instanceof HTMLLabelElement && element.htmlFor
		? document.getElementById(element.htmlFor) as HTMLInputElement | null
		: element.querySelector<HTMLInputElement>('input');
      return {
        tag: element.tagName,
        label: (element.textContent ?? '').replace(/\s+/g, ' ').trim().slice(0, 80),
		classes: [...element.classList].sort(),
		role: element.getAttribute('role'),
		dataAttributes: element.getAttributeNames().filter((name) => name.startsWith('data-')).sort(),
		for: element instanceof HTMLLabelElement ? element.htmlFor : null,
		input: relatedInput ? { type: relatedInput.type, name: relatedInput.name, value: relatedInput.value } : null,
        value: element.getAttribute('value'),
        query,
      };
    });
  return {
    productLinkCount: links.length,
    firstLinkAttributes: first ? first.getAttributeNames().sort() : [],
    containerTag: container?.tagName ?? null,
    containerClasses: container ? [...container.classList].sort() : [],
    descendantClassTokens: classTokens,
    cardStructure,
    scripts,
    jsonLDSchema,
    sortingControls,
  };
});
const href = await page.locator('a[href*="/vp/products/"]').first().getAttribute("href");
if (!href) throw new Error("No product URL found");
const productURL = new URL(href, "https://www.coupang.com").toString();
captured.length = 0;
await page.goto(productURL, { waitUntil: "domcontentloaded", timeout: 20_000 }).catch(() => undefined);
await page.waitForTimeout(2_000);
const productDocument = await page.evaluate(() => ({
  jsonLDScriptCount: document.querySelectorAll('script[type="application/ld+json"]').length,
  nextDataPresent: document.getElementById("__NEXT_DATA__") !== null,
  imageCount: document.images.length,
  detailImageCandidates: document.querySelectorAll(
    '#productDetail img, .product-detail img, [class*="detail"] img, [id*="detail"] img',
  ).length,
  hasPriceText: /[0-9][0-9,]*\s*원/.test(document.body?.innerText ?? ""),
  hasReviewText: /상품평|리뷰/.test(document.body?.innerText ?? ""),
  hasCardText: /카드.{0,12}(할인|혜택)|카드할인/.test(document.body?.innerText ?? ""),
  hasCouponText: /쿠폰/.test(document.body?.innerText ?? ""),
}));
const reviewTab = page.locator('a[href*="review"], button:has-text("상품평"), a:has-text("상품평")').first();
if (await reviewTab.count()) {
  await reviewTab.scrollIntoViewIfNeeded().catch(() => undefined);
  await reviewTab.click({ timeout: 3_000 }).catch(() => undefined);
  await page.waitForTimeout(2_000);
}

process.stdout.write(`${JSON.stringify({ searchDocument, productURL: redactURL(productURL), productDocument, captured }, null, 2)}\n`);
await page.close();
process.exit(0);
