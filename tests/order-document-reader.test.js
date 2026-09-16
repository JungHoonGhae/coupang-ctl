import assert from 'node:assert/strict';
import test from 'node:test';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';
import {webcrypto} from 'node:crypto';
import {setTimeout as delay} from 'node:timers/promises';

const shared=readFileSync(new URL('../internal/browser/order_page_reader.js',import.meta.url),'utf8').replace('export async function','async function');
const auth=readFileSync(new URL('../internal/browser/authentication_document_poll.js',import.meta.url),'utf8');
const authEndpoint='https://mc.coupang.com/ssr/api/member/auth';
const wrapper=readFileSync(new URL('../internal/browser/order_document_poll.js',import.meta.url),'utf8').replace('/* SHARED_READER */',shared).replace('/* AUTH_READER */',auth);
function harness({url='https://mc.coupang.com/ssr/desktop/order/list',body='',status=200,domain={orderList:[],hasNext:false},pending=false,nonIterableBytes=false,authenticated='true',authStatus=200,authPending=false,orderStatus=200}={}) {
 const calls=[],authCalls=[],timers=new Map();let timerID=0,orderReads=0;
 const location=new URL(url),window={};
 const document={readyState:'complete',title:'Synthetic',body:{innerText:body},querySelector:s=>{
  if(s!=='script#__NEXT_DATA__')return null;
  orderReads++;return {textContent:JSON.stringify({props:{pageProps:{domains:{desktopOrder:domain}}}})};
 }};
 const context=vm.createContext({URL,URLSearchParams,TextEncoder,TextDecoder,crypto:webcrypto,location,document,window,AbortController,performance:{getEntriesByType:()=>[{name:url,responseStatus:status}]},
  setTimeout:fn=>{const id=++timerID;timers.set(id,fn);return id;},clearTimeout:id=>timers.delete(id),
  fetch:async(url,options)=>{
   if(url===authEndpoint){
    authCalls.push({url,options});
    if(authPending)await new Promise((_,reject)=>options.signal.addEventListener('abort',()=>reject(Error('aborted')),{once:true}));
    const response=new Response(authenticated,{status:authStatus,headers:{'content-type':'application/json'}});
    Object.defineProperty(response,'url',{value:authEndpoint});return response;
   }
   calls.push({url,options});if(pending)await new Promise((_,reject)=>options.signal.addEventListener('abort',()=>reject(Error('aborted')),{once:true}));return{ok:orderStatus===200,status:orderStatus,text:async()=>JSON.stringify(domain)};
  }
 });
 if(nonIterableBytes)vm.runInContext('Uint8Array=class extends Uint8Array{constructor(...args){super(...args);return new Proxy(this,{get(target,key){if(key===Symbol.iterator||/^\\d+$/.test(String(key)))throw Error("synthetic typed-array access unavailable");return Reflect.get(target,key,target)}})}}',context);
 vm.runInContext(wrapper,context);
 const poll=(cursor=null,key='test')=>JSON.parse(context.pollOrderPage(cursor,key));
 const settle=async(cursor=null,key='test')=>{
  let result=poll(cursor,key);
  for(let i=0;i<30&&result.status==='loading';i++){await flush();result=poll(cursor,key);}
  return result;
 };
 return {calls,authCalls,window,poll,settle,expire:()=>[...timers.values()].forEach(fn=>fn()),document,location,orderReads:()=>orderReads};
}
const flush=()=>new Promise(resolve=>setImmediate(resolve));

test('orders automatically check native authentication before reading even plausible order data',async()=>{
 for(const cursor of [null,{year:2026,page:1}]){
  for(const [options,status] of [
   [{authenticated:'false'},'authentication_required'],
   [{authStatus:401},'authentication_required'],
   [{authStatus:403},'access_denied'],
   [{authStatus:429},'access_denied'],
   [{authenticated:'{}'},'authentication_data_missing'],
   [{authStatus:500},'authentication_data_missing'],
  ]){
   const h=harness(options);
   assert.deepEqual(await h.settle(cursor),{status});
   assert.deepEqual(h.poll(cursor),{status});
   assert.equal(h.authCalls.length,1);
   assert.equal(h.orderReads(),0);
   assert.equal(h.calls.length,0);
  }
 }
});

test('login, denial and non-order pages never yield authenticated order evidence',()=>{
 for(const [options,want] of [[{url:'https://login.coupang.com/login/login.pang'},'authentication_required'],[{status:403},'access_denied'],[{status:429},'access_denied'],[{body:'CAPTCHA'},'access_denied'],[{url:'https://www.coupang.com/np/search?q=synthetic'},'authentication_data_missing']]){
  const h=harness(options);assert.equal(h.poll().status,want);assert.equal(h.calls.length,0);assert.equal(h.authCalls.length,0);assert.deepEqual(Object.keys(h.window),[]);
 }
});
test('only structured orders with explicit pagination prove a protected read',async()=>{
 const h=harness();assert.equal(h.poll().status,'loading');assert.deepEqual(await h.settle(),{status:'ok',page:{orders:[]}});
 assert.equal(h.authCalls.length,1);assert.equal(h.orderReads(),1);
 const missing=harness({domain:{orderList:[]}});assert.equal((await missing.settle()).status,'order_data_missing');
});
test('partial order responses stop polling without exposing a page or retrying',async()=>{
 for(const cursor of [null,{year:2026,page:1}]){
  const h=harness({domain:{orderList:[],hasNext:false,partial:true}});
  assert.equal(h.poll(cursor).status,'loading');
  assert.deepEqual(await h.settle(cursor),{status:'order_partial'});
  assert.deepEqual(h.poll(cursor),{status:'order_partial'});
  assert.equal(h.calls.length,cursor?1:0);
 }
});
test('continuation is one cancellable same-origin GET even with repeated polls',async()=>{
 const h=harness();const cursor={year:2026,page:1};h.poll(cursor);h.poll(cursor);
 assert.equal((await h.settle(cursor)).status,'ok');assert.equal(h.calls.length,1);assert.equal(h.authCalls.length,1);
 assert.equal(h.calls[0].url,'/ssr/api/myorders/model?pageIndex=1&requestYear=2026&size=5');assert.equal(h.calls[0].options.method,'GET');assert.equal(h.calls[0].options.credentials,'include');
 assert.equal(h.calls[0].options.redirect,'error');
 const slow=harness({pending:true});await slow.settle(cursor);slow.expire();await flush();assert.equal(slow.calls[0].options.signal.aborted,true);assert.equal(slow.poll(cursor).status,'order_data_missing');
});
test('late completion cannot publish into a removed request generation',async()=>{
 const h=harness();h.poll(null,'old');await flush();h.poll(null,'old');const old=h.window.old;delete h.window.old;
 assert.equal((await h.settle(null,'new')).status,'ok');
 assert.equal(h.window.old,undefined);assert.equal(old.status,'loading');
});
test('authentication timeout never reads orders or starts a retry',async()=>{
 const h=harness({authPending:true});h.poll();h.expire();await flush();
 assert.deepEqual(h.poll(),{status:'authentication_data_missing'});
 assert.equal(h.authCalls.length,1);assert.equal(h.authCalls[0].options.signal.aborted,true);
 assert.equal(h.calls.length,0);assert.equal(h.orderReads(),0);
});
test('authentication success never overrides a later denied or expired order response',async()=>{
 for(const [orderStatus,status] of [[401,'authentication_required'],[403,'access_denied'],[429,'access_denied']]){
  const h=harness({orderStatus}),cursor={year:2026,page:1};
  assert.deepEqual(await h.settle(cursor),{status});
  assert.deepEqual(h.poll(cursor),{status});
  assert.equal(h.authCalls.length,1);assert.equal(h.calls.length,1);
 }
});
test('a login redirect during an order request aborts it and cannot expose a stale page',async()=>{
 const h=harness({pending:true}),cursor={year:2026,page:1};await h.settle(cursor);
 h.location.href='https://login.coupang.com/login/login.pang';
 assert.deepEqual(h.poll(cursor),{status:'authentication_required'});
 assert.equal(h.calls[0].options.signal.aborted,true);await flush();
 assert.deepEqual(h.poll(cursor),{status:'authentication_required'});
 assert.equal(h.calls.length,1);
});
test('protected order reference hashing tolerates unavailable page-realm typed-array access',async()=>{
 const domain={orderList:[{orderId:'synthetic-order',orderDate:'2026-01-01',totalPrice:1000,items:[]}],hasNext:false};
 const expected=harness({domain}),actual=harness({domain,nonIterableBytes:true});
 // WebCrypto completes on a worker thread. Event-loop turn counts do not
 // bound that work, especially after the asynchronous authentication stage.
 const deadline=Date.now()+5000;
 let expectedResult,actualResult;
 do {
  expectedResult=expected.poll();actualResult=actual.poll();
  if(expectedResult.status!=='loading'&&actualResult.status!=='loading')break;
  await delay(10);
 } while(Date.now()<deadline);
 assert.equal(expectedResult.status,'ok');
 assert.equal(actualResult.status,'ok');
 assert.deepEqual(actualResult,expectedResult);
 assert.match(actualResult.page.orders[0].source_ref,/^[a-f0-9]{64}$/);
});
