// Only this fixed account read contract runs in the dedicated browser page.
// Account numbers and raw cash transaction descriptions never leave the page.
export async function readAccountPage(kind, maxPages) {
  const missing = {status: 'account_data_missing'};
  const targets = {membership: 'https://loyalty.coupang.com/loyalty/management/home', cash: 'https://cash.coupang.com/coupang-cash/home'};
  if (!Object.hasOwn(targets, kind) || !Number.isInteger(maxPages) || maxPages < 1 || maxPages > 100) return missing;
  const isTarget = href => {
    try {
      const actual = new URL(href), expected = new URL(targets[kind]);
      if (actual.origin !== expected.origin || actual.pathname !== expected.pathname || actual.username || actual.password || actual.href.includes('#')) return false;
      if (actual.href === expected.href) return true;
      // The cash page adds opaque state after navigation. Validate its shape
      // only; never copy it into endpoint requests or the returned document.
      const params = [...actual.searchParams];
      return kind === 'cash' && params.length === 1 && params[0][0] === 'cst' && /^[^\x00-\x1f\x7f]{1,4096}$/.test(params[0][1]);
    } catch { return false; }
  };
  let current;
  try { current = new URL(location.href); } catch { return missing; }
  if (current.origin === 'https://login.coupang.com') return {status: 'authentication_required'};
  if (!isTarget(current.href)) return missing;
  if (document.querySelector('input[type="password"]')) return {status: 'authentication_required'};
  const text = document.title + ' ' + (document.body?.innerText || '');
  const status = performance.getEntriesByType('navigation').find(n => n.name === current.href)?.responseStatus;
  if ([403, 429].includes(status) || /access denied|captcha|보안문자|자동입력방지|접근.{0,8}(거부|제한)/i.test(text)) return {status: 'access_denied'};
  if (status >= 400) return missing;
  if (document.readyState !== 'complete') return {status: 'loading'};
  const object = v => v !== null && typeof v === 'object' && !Array.isArray(v);
  const integer = v => Number.isSafeInteger(v) && v >= 0;
  const string = (v, length = 100) => typeof v === 'string' && v.length <= length && !/[\x00-\x1f]/.test(v);
  const numbers = (value, fields) => {
    const out = {};
    if (!object(value)) return out;
    for (const field of fields) {
      if (value[field] == null) continue;
      if (!integer(value[field])) throw Error('invalid_number');
      out[field] = value[field];
    }
    return out;
  };
  const payment = value => {
    const out = {paymentMethodDTO: {}};
    if (!object(value)) return out;
    for (const field of ['payMethodType', 'payMethodName', 'payMethodIssuer']) {
      const item = value.paymentMethodDTO?.[field];
      if (item == null) continue;
      if (!string(item)) throw Error('invalid_payment_label');
      out.paymentMethodDTO[field] = item;
    }
    if (typeof value.recurringPayRegistered === 'boolean') out.recurringPayRegistered = value.recurringPayRegistered;
    return out;
  };
  if (kind === 'membership') {
    const encoded = document.getElementById('__NEXT_DATA__')?.textContent;
    if (!encoded) return {status: 'loading'};
    if (encoded.length > 2 * 1024 * 1024) return missing;
    try {
      const root = JSON.parse(encoded);
      const data = root?.props?.pageProps?.data ?? root?.query?.data;
      const info = data?.loyaltyMemberInfo;
      if (!object(info) || !string(info.membershipStatus) || !info.membershipStatus.trim()) return missing;
      const member = numbers(info, ['firstJoinDt', 'currentFee', 'nextPaymentDate']);
      member.membershipStatus = info.membershipStatus;
      for (const key of ['notMember', 'paidMember', 'trialMember', 'membershipOnHold']) {
        if (info[key] == null) continue;
        if (typeof info[key] !== 'boolean') return missing;
        member[key] = info[key];
      }
      if (info.subscriptionPlan != null) {
        if (!string(info.subscriptionPlan)) return missing;
        member.subscriptionPlan = info.subscriptionPlan;
      }
      member.membershipInfoVO = numbers(info.membershipInfoVO, ['membershipStartDt', 'membershipEndDt']);
      member.paymentProperty = numbers(info.paymentProperty, ['unitAmount', 'nextPaymentDt']);
      const clean = numbers(data, ['loyaltyFeeChangeDate']);
      clean.loyaltyMemberInfo = member;
      clean.paymentMethod = payment(data.paymentMethod);
      if (data.paymentMethods != null) {
        if (!Array.isArray(data.paymentMethods) || data.paymentMethods.length > 30) return missing;
        clean.paymentMethods = data.paymentMethods.map(payment);
      }
      clean.wowBenefitUsage = numbers(data.wowBenefitUsage, [
        'membershipDays', 'totalAmount', 'rocketFreeDeliveryAmount', 'dawnAndSamedayDeliveryAmount',
        'freshDeliveryAmount', 'freeDeliveryTotalAmount', 'wowOnlyDiscountAmount', 'freeReturnAmount',
        'rocketJikguFreeDeliveryAmount', 'eatsDiscountAmount', 'coupangDiscountAmount', 'additionalCashbackAmount',
        'retentionCashback', 'retailFreeShippingCount', 'ordersDawnAndSamedayCount', 'ordersRocketFreshCount',
        'freeReturnCount', 'jikguFreeShippingCount',
      ]);
      clean.wowBenefitUsage.wowBenefitUsageImprovedDtoV2 = numbers(data.wowBenefitUsage?.wowBenefitUsageImprovedDtoV2, ['numbersOrderEats']);
      return {status: 'ok', data: {data: clean, benefit_window_months: /(?:최근|지난)\s*3개월|3개월\s*누적/i.test(text) ? 3 : 0}};
    } catch { return missing; }
  }
  // Fixed GET endpoints observed in the account page. No resource-prefix
  // crawling, request replay to arbitrary URLs, or membership/payment writes.
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 25000);
  let bytesRead = 0;
  const read = async path => {
    if (!isTarget(location.href)) throw Error('account_data_missing');
    const response = await fetch('https://cash.coupang.com' + path, {
      method: 'GET', credentials: 'include', redirect: 'error', signal: controller.signal,
    });
    if (response.status === 401) throw Error('authentication_required');
    if ([403, 429].includes(response.status)) throw Error('access_denied');
    if (!response.ok || !/^application\/json(?:\s*;|$)/i.test(response.headers.get('content-type') || '')) throw Error('account_data_missing');
    const chunks = []; let length = 0;
    const reader = response.body.getReader();
    try {
      while (true) {
        const {value, done} = await reader.read();
        if (done) break;
        length += value.byteLength; bytesRead += value.byteLength;
        if (length > 256 * 1024 || bytesRead > 2 * 1024 * 1024) throw Error('account_data_missing');
        chunks.push(value);
      }
    } finally { await reader.cancel(); }
    const merged = new Uint8Array(length); let offset = 0;
    for (const chunk of chunks) { merged.set(chunk, offset); offset += chunk.length; }
    if (!isTarget(location.href)) throw Error('account_data_missing');
    return JSON.parse(new TextDecoder().decode(merged));
  };
  const money = value => {
    if (!object(value) || !Number.isSafeInteger(value.amount)) throw Error('account_data_missing');
    const currency = value.currencyCode || value.currency;
    if (currency !== 'KRW' || value.currencyCode && value.currency && value.currencyCode !== value.currency) throw Error('account_data_missing');
    return {amount: value.amount, currencyCode: 'KRW'};
  };
  try {
    const raw = (await read('/api/cash/expected-cash-accumulation'))?.content;
    if (!object(raw)) return missing;
    const summary = {content: {}};
    if (raw.expectedWowCardAccumulationAmount != null) summary.content.expectedWowCardAccumulationAmount = money(raw.expectedWowCardAccumulationAmount);
    for (const key of ['expectedWowCardAccumulationAmountThisMonth', 'expectedWowCardAccumulationAmountNextMonth']) {
      if (raw[key] == null) continue;
      summary.content[key] = {amount: money(raw[key].amount)};
      if (string(raw[key].earningDate, 10) && /^\d{4}-\d{2}-\d{2}$/.test(raw[key].earningDate)) summary.content[key].earningDate = raw[key].earningDate;
    }
    if (!Object.keys(summary.content).length) return missing;
    const pages = [];
    for (let number = 1; number <= maxPages; number++) {
      const page = (await read('/api/cash/transactions?page=' + number))?.content;
      if (!object(page) || page.currentPageNumber !== number || typeof page.nextPageExist !== 'boolean' || !Array.isArray(page.list) || page.list.length > 500) return missing;
      const list = [];
      for (const row of page.list) {
        if (!object(row) || !string(row.displayMessage, 4000) || !string(row.description, 4000)) return missing;
        const label = (row.displayMessage + ' ' + row.description).toLowerCase();
        if (!label.includes('와우카드') && !label.includes('wow card')) continue;
        if (!string(row.createdAt, 40)) return missing;
        list.push({source_wow_card_label: true, cashableAmount: money(row.cashableAmount), nonCashableAmount: money(row.nonCashableAmount), createdAt: row.createdAt});
      }
      pages.push({content: {currentPageNumber: number, nextPageExist: page.nextPageExist, list}});
      if (JSON.stringify({summary, pages}).length > 512 * 1024) return missing;
      if (!page.nextPageExist) break;
    }
    return {status: 'ok', summary, pages};
  } catch (error) {
    return {status: ['access_denied', 'authentication_required'].includes(error.message) ? error.message : 'account_data_missing'};
  } finally { clearTimeout(timer); controller.abort(); }
}
