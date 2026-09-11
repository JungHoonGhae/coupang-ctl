import assert from 'node:assert/strict';
import test from 'node:test';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';

const source = readFileSync(new URL('../internal/browser/account_page_reader.js', import.meta.url), 'utf8').replace('export async function', 'async function');
const membershipURL = 'https://loyalty.coupang.com/loyalty/management/home';
const cashURL = 'https://cash.coupang.com/coupang-cash/home';
const membership = {
  loyaltyMemberInfo: {membershipStatus: 'ACTIVE', currentFee: 0, notMember: false, paidMember: true, trialMember: false, membershipOnHold: false, privateId: 'synthetic-private-id'},
  wowBenefitUsage: {totalAmount: 0, membershipDays: 30},
  paymentMethod: {paymentMethodDTO: {payMethodName: 'Synthetic card', payMethodAccount: 'synthetic-private-account'}},
};
const summary = {content: {expectedWowCardAccumulationAmount: {currency: 'KRW', amount: 0}}};
const reward = {cashableAmount: {currencyCode: 'KRW', amount: 100}, nonCashableAmount: {currencyCode: 'KRW', amount: 0}, createdAt: '2026-09-11T00:00:00Z', displayMessage: 'Synthetic WOW card', description: 'synthetic-private-description', accountNumber: 'synthetic-private-account'};
const page = (number, more, list = [reward]) => ({content: {currentPageNumber: number, nextPageExist: more, list}});
function fixture({kind = 'membership', href, ready = 'complete', status = 200, title = 'Synthetic 최근 3개월', data = membership, password = false, respond} = {}) {
  href ??= kind === 'membership' ? membershipURL : cashURL;
  const requests = [];
  const context = vm.createContext({URL, TextDecoder, Uint8Array, AbortController, setTimeout, clearTimeout,
    location: {href}, performance: {getEntriesByType: () => [{name: href, responseStatus: status}]},
    document: {title, readyState: ready, body: {innerText: ''}, querySelector: () => password ? {} : null,
      getElementById: () => data === undefined ? null : ({textContent: JSON.stringify({props: {pageProps: {data}}})})},
    fetch: async (url, options) => {
      requests.push(url);
      assert.equal(options.method, 'GET'); assert.equal(options.credentials, 'include'); assert.equal(options.redirect, 'error');
      assert.equal(new URL(url).origin, 'https://cash.coupang.com');
      const result = respond?.(url, requests.length) ?? (url.endsWith('expected-cash-accumulation') ? summary : page(Number(new URL(url).searchParams.get('page')), false));
      return result instanceof Response ? result : new Response(JSON.stringify(result), {headers: {'content-type': 'application/json'}});
    },
  });
  vm.runInContext(source, context);
  return {requests, navigate: href => { context.location.href = href; }, read: async (limit = 1) => JSON.parse(JSON.stringify(await context.readAccountPage(kind, limit)))};
}
test('membership keeps known zero/false and discards identifiers before transport', async () => {
  const f = fixture(), result = await f.read();
  assert.equal(result.status, 'ok');
  assert.equal(result.data.data.loyaltyMemberInfo.currentFee, 0);
  assert.equal(result.data.data.loyaltyMemberInfo.notMember, false);
  assert.equal(result.data.data.wowBenefitUsage.totalAmount, 0);
  assert.equal(result.data.benefit_window_months, 3);
  assert.equal(JSON.stringify(result).includes('synthetic-private'), false);
  assert.deepEqual(f.requests, []);
  const missing = await fixture({data: {...membership, wowBenefitUsage: {membershipDays: 30}}}).read();
  assert.equal(Object.hasOwn(missing.data.data.wowBenefitUsage, 'totalAmount'), false);
});
test('membership optional flags stay absent without losing independent fee or benefit evidence', async () => {
  const result = await fixture({data: {...membership, loyaltyMemberInfo: {membershipStatus:'ACTIVE', currentFee:0, paidMember:null}}}).read();
  assert.equal(result.status,'ok');
  assert.deepEqual(result.data.data.loyaltyMemberInfo,{membershipStatus:'ACTIVE',currentFee:0,membershipInfoVO:{},paymentProperty:{}});
  assert.equal(result.data.data.wowBenefitUsage.totalAmount,0);
  assert.equal(Object.hasOwn(result.data.data.paymentMethod,'recurringPayRegistered'),false);
});
test('account reads fail before fetching on wrong page, challenge, invalid bounds or malformed member flags', async () => {
  for (const options of [
    {href: 'https://example.invalid'}, {href: membershipURL + '?submit=true'}, {href: membershipURL + '#other'},
    {data: {...membership, loyaltyMemberInfo: {membershipStatus: 'ACTIVE',paidMember:'false'}}},
    {data: {...membership, loyaltyMemberInfo: {membershipStatus: 'ACTIVE',notMember:0}}},
    {data: {...membership, loyaltyMemberInfo: {currentFee:0}}},
  ]) assert.equal((await fixture(options).read()).status, 'account_data_missing');
  for (const limit of [0, 101, -1, 1.5]) { const f = fixture({kind: 'cash'}); assert.equal((await f.read(limit)).status, 'account_data_missing'); assert.equal(f.requests.length, 0); }
  for (const status of [403,429]) assert.equal((await fixture({status}).read()).status, 'access_denied');
  assert.equal((await fixture({title: 'Access Denied'}).read()).status, 'access_denied');
  assert.equal((await fixture({href: 'https://login.coupang.com/login'}).read()).status, 'authentication_required');
  assert.equal((await fixture({password: true}).read()).status, 'authentication_required');
  assert.equal((await fixture({ready: 'interactive'}).read()).status, 'loading');
});
test('cash reads only fixed GETs and returns label classification instead of raw descriptions', async () => {
  const f = fixture({kind: 'cash', respond: url => url.includes('transactions') ? page(1,true,[reward,{...reward,displayMessage:'Other reward',description:'Other'}]) : summary});
  const result = await f.read(1);
  assert.equal(result.status,'ok');
  assert.deepEqual(f.requests, ['https://cash.coupang.com/api/cash/expected-cash-accumulation','https://cash.coupang.com/api/cash/transactions?page=1']);
  assert.equal(result.pages[0].content.nextPageExist,true);
  assert.equal(result.pages[0].content.list.length,1);
  assert.equal(result.pages[0].content.list[0].source_wow_card_label,true);
  assert.equal(JSON.stringify(result).includes('synthetic-private'),false);
  assert.equal(Object.hasOwn(result.pages[0].content.list[0],'description'),false);
});

test('source-added opaque cash state is accepted only on the exact page and never forwarded', async () => {
  const f=fixture({kind:'cash',href:cashURL+'?cst=synthetic-opaque-state'});
  const result=await f.read();
  assert.equal(result.status,'ok');
  assert.equal(f.requests.some(url=>url.includes('cst=')||url.includes('synthetic-opaque-state')),false);
  assert.equal(JSON.stringify(result).includes('synthetic-opaque-state'),false);
  for(const suffix of ['?cst=','?cst=x&cst=y','?cst=x&submit=true','?cst=x#other','?cst=x#','?cst=%00','?cst='+ 'x'.repeat(4097)]) {
    const invalid = fixture({kind:'cash',href:cashURL+suffix});
    assert.equal((await invalid.read()).status,'account_data_missing');
    assert.equal(invalid.requests.length,0);
  }
  const credentials = fixture({kind:'cash',href:cashURL.replace('https://','https://user:password@')+'?cst=x'});
  assert.equal((await credentials.read()).status,'account_data_missing');
  assert.equal(credentials.requests.length,0);
  assert.equal((await fixture({href:membershipURL+'?cst=x'}).read()).status,'account_data_missing');
});
test('cash read discards a response if the page moves outside its scope while fetching', async () => {
  const f = fixture({kind:'cash',href:cashURL+'?cst=initial',respond:()=>{
    f.navigate(cashURL+'?cst=changed&other=1');
    return summary;
  }});
  assert.deepEqual(await f.read(),{status:'account_data_missing'});
  assert.equal(f.requests.length,1);
  const valid = fixture({kind:'cash',respond:url=>{
    valid.navigate(cashURL+'?cst=changed');
    return url.includes('transactions')?page(1,false):summary;
  }});
  assert.equal((await valid.read()).status,'ok');
});
test('cash stops at a verified terminal page and rejects repeated or incomplete page shapes', async () => {
  const f = fixture({kind:'cash',respond:url=>url.includes('transactions')?page(Number(new URL(url).searchParams.get('page')),url.endsWith('=1')):summary});
  const result=await f.read(10);
  assert.equal(result.status,'ok'); assert.equal(result.pages.length,2); assert.equal(f.requests.length,3);
  for(const invalid of [page(2,false),{content:{currentPageNumber:1,list:[]}},{content:{currentPageNumber:1,nextPageExist:false}},{content:{currentPageNumber:1,nextPageExist:null,list:[]}}]) {
    assert.equal((await fixture({kind:'cash',respond:url=>url.includes('transactions')?invalid:summary}).read()).status,'account_data_missing');
  }
});
test('cash access failures stop the operation without subsequent page requests', async () => {
  for (const [status, expected] of [[401,'authentication_required'],[403,'access_denied'],[429,'access_denied'],[500,'account_data_missing']]) {
    const f=fixture({kind:'cash',respond:url=>url.includes('transactions')?new Response('{}',{status,headers:{'content-type':'application/json'}}):summary});
    assert.equal((await f.read(10)).status,expected);assert.equal(f.requests.length,2);
  }
});
test('cash rejects oversized bodies, invalid currency, unsafe amounts and wrong content types', async () => {
  for(const result of [
    new Response(' '.repeat(256*1024+1),{headers:{'content-type':'application/json'}}),
    new Response('{}',{headers:{'content-type':'text/html'}}),
    {content:{expectedWowCardAccumulationAmount:{currency:'USD',amount:100}}},
    {content:{expectedWowCardAccumulationAmount:{currency:'KRW',amount:Number.MAX_SAFE_INTEGER+1}}},
    {content:{expectedWowCardAccumulationAmount:{amount:100}}},
  ]) assert.equal((await fixture({kind:'cash',respond:()=>result}).read()).status,'account_data_missing');
});
