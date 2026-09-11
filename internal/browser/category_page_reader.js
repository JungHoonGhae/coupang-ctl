export function readProductCategoryPage(target) {
  const missing = {status: 'category_data_missing'};
  let expected, current;
  const safeProduct = u => {
    if (u.origin !== 'https://www.coupang.com' || u.username || u.password || u.hash || !/^\/vp\/products\/\d{1,24}$/.test(u.pathname)) return false;
    // Product pages can install a URLSearchParams polyfill whose keys() result
    // is not iterable. forEach preserves duplicate/unknown-key checks there.
    let valid = true;
    u.searchParams.forEach((value, key) => {
      if (!['itemId', 'vendorItemId'].includes(key) || u.searchParams.getAll(key).length !== 1 || !/^\d{1,24}$/.test(value)) valid = false;
    });
    return valid;
  };
  try {
    expected = new URL(target); current = new URL(location.href);
    if (!safeProduct(expected)) return missing;
  } catch { return missing; }
  if (current.origin === 'https://login.coupang.com' || document.querySelector('input[type="password"]')) return {status: 'authentication_required'};
  const body = document.title + ' ' + (document.body?.innerText || '');
  const nav = performance.getEntriesByType('navigation').find(n => n.name === current.href);
  if (nav?.responseStatus === 403 || nav?.responseStatus === 429 || /access denied|captcha|보안문자|자동입력방지|접근.{0,8}(거부|제한)/i.test(body)) return {status: 'access_denied'};
  if (!safeProduct(current) || current.pathname !== expected.pathname) return missing;
  if (document.readyState !== 'complete') return {status: 'loading'};
  if ([404, 410].includes(nav?.responseStatus)) return {status: 'category_unavailable'};
  if (nav?.responseStatus >= 400) return missing;
  const canonical = document.querySelector('link[rel="canonical"]')?.href;
  if (canonical) {
    try { const u = new URL(canonical); if (!safeProduct(u) || u.pathname !== expected.pathname) return missing; }
    catch { return missing; }
  }
  // Only source-native breadcrumbs leave the page. Product/account payloads
  // and unrelated JSON-LD fields are never included in the result.
  const candidates = [], nodes = [];
  let bytes = 0, productObserved = false;
  for (const script of document.querySelectorAll('script[type="application/ld+json"]')) {
    const text = script.textContent || '';
    bytes += text.length;
    if (bytes > 2 * 1024 * 1024) return missing;
    try { nodes.push(JSON.parse(text)); } catch { return missing; }
  }
  let visited = 0;
  while (nodes.length) {
    if (++visited > 1000) return missing;
    const node = nodes.shift();
    if (Array.isArray(node)) { nodes.push(...node); continue; }
    if (!node || typeof node !== 'object') continue;
    if (Array.isArray(node['@graph'])) nodes.push(...node['@graph']);
    if (node['@type'] === 'Product') {
      try { const u = new URL(node.url); productObserved ||= safeProduct(u) && u.pathname === expected.pathname; } catch {}
    }
    if (node['@type'] !== 'BreadcrumbList') continue;
    if (!Array.isArray(node.itemListElement) || node.itemListElement.length < 1 || node.itemListElement.length > 14) return missing;
    const rows = [];
    for (const row of node.itemListElement) {
      if (row?.['@type'] !== 'ListItem' || !Number.isSafeInteger(row.position) || row.position < 1 ||
          typeof row.name !== 'string' || !row.name.trim() || [...row.name.trim()].length > 100) return missing;
      let u;
      try { u = new URL(row.item); } catch { return missing; }
      if (u.origin !== 'https://www.coupang.com' || u.username || u.password || u.hash || u.search ||
          !(u.pathname === '/' || /^\/np\/categories\/\d{1,20}$/.test(u.pathname) || u.pathname === expected.pathname)) return missing;
      rows.push({'@type': 'ListItem', position: row.position, name: row.name.trim(), item: u.href});
    }
    rows.sort((a, b) => a.position - b.position);
    if (rows.some((row, i) => i > 0 && row.position === rows[i - 1].position)) return missing;
    const value = {'@type': 'BreadcrumbList', itemListElement: rows};
    if (!candidates.some(c => JSON.stringify(c) === JSON.stringify(value))) candidates.push(value);
  }
  if (candidates.length > 1) return missing;
  if (!candidates.length && !productObserved) return {status: 'loading'};
  return {status: 'ok', category: {json_ld: candidates}};
}
