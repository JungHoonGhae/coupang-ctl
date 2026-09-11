import assert from 'node:assert/strict';
import test from 'node:test';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';
import {webcrypto} from 'node:crypto';

const shared=readFileSync(new URL('../internal/browser/order_page_reader.js',import.meta.url),'utf8').replace('export async function','async function');
const wrapper=readFileSync(new URL('../internal/browser/order_document_poll.js',import.meta.url),'utf8').replace('/* SHARED_READER */',shared);
function harness({url='https://mc.coupang.com/ssr/desktop/order/list',body='',status=200,domain={orderList:[],hasNext:false},pending=false,nonIterableBytes=false}={}) {
 const calls=[],timers=new Map();let timerID=0;
 const location=new URL(url),window={};
 const document={readyState:'complete',title:'Synthetic',body:{innerText:body},querySelector:s=>s==='script#__NEXT_DATA__'?{textContent:JSON.stringify({props:{pageProps:{domains:{desktopOrder:domain}}}})}:null};
 const context=vm.createContext({URL,URLSearchParams,TextEncoder,crypto:webcrypto,location,document,window,AbortController,performance:{getEntriesByType:()=>[{name:url,responseStatus:status}]},
  setTimeout:fn=>{const id=++timerID;timers.set(id,fn);return id;},clearTimeout:id=>timers.delete(id),
  fetch:async(url,options)=>{calls.push({url,options});if(pending)await new Promise((_,reject)=>options.signal.addEventListener('abort',()=>reject(Error('aborted')),{once:true}));return{ok:true,status:200,text:async()=>JSON.stringify(domain)};}
 });
 if(nonIterableBytes)vm.runInContext('Uint8Array=class extends Uint8Array{constructor(...args){super(...args);return new Proxy(this,{get(target,key){if(key===Symbol.iterator||/^\\d+$/.test(String(key)))throw Error("synthetic typed-array access unavailable");return Reflect.get(target,key,target)}})}}',context);
 vm.runInContext(wrapper,context);
 return {calls,window,poll:(cursor=null,key='test')=>JSON.parse(context.pollOrderPage(cursor,key)),expire:()=>[...timers.values()].forEach(fn=>fn()),document};
}
const flush=()=>new Promise(resolve=>setImmediate(resolve));

test('login, denial and non-order pages never yield authenticated order evidence',()=>{
 for(const [options,want] of [[{url:'https://login.coupang.com/login/login.pang'},'authentication_required'],[{status:403},'access_denied'],[{body:'CAPTCHA'},'access_denied'],[{url:'https://www.coupang.com/np/search?q=synthetic'},'loading']]){
  const h=harness(options);assert.equal(h.poll().status,want);assert.equal(h.calls.length,0);assert.deepEqual(Object.keys(h.window),[]);
 }
});
test('only structured orders with explicit pagination prove a protected read',async()=>{
 const h=harness();assert.equal(h.poll().status,'loading');await flush();assert.deepEqual(h.poll(),{status:'ok',page:{orders:[]}});
 const missing=harness({domain:{orderList:[]}});missing.poll();await flush();assert.equal(missing.poll().status,'order_data_missing');
});
test('partial order responses stop polling without exposing a page or retrying',async()=>{
 for(const cursor of [null,{year:2026,page:1}]){
  const h=harness({domain:{orderList:[],hasNext:false,partial:true}});
  assert.equal(h.poll(cursor).status,'loading');await flush();
  assert.deepEqual(h.poll(cursor),{status:'order_partial'});
  assert.deepEqual(h.poll(cursor),{status:'order_partial'});
  assert.equal(h.calls.length,cursor?1:0);
 }
});
test('continuation is one cancellable same-origin GET even with repeated polls',async()=>{
 const h=harness();const cursor={year:2026,page:1};h.poll(cursor);h.poll(cursor);await flush();
 assert.equal(h.poll(cursor).status,'ok');assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].url,'/ssr/api/myorders/model?pageIndex=1&requestYear=2026&size=5');assert.equal(h.calls[0].options.method,'GET');assert.equal(h.calls[0].options.credentials,'include');
 const slow=harness({pending:true});slow.poll(cursor);await flush();slow.expire();await flush();assert.equal(slow.calls[0].options.signal.aborted,true);assert.equal(slow.poll(cursor).status,'order_data_missing');
});
test('late completion cannot publish into a removed request generation',async()=>{
 const h=harness();h.poll(null,'old');const old=h.window.old;delete h.window.old;h.poll(null,'new');await flush();
 assert.equal(h.window.old,undefined);assert.equal(old.status,'loading');assert.equal(h.poll(null,'new').status,'ok');
});
test('protected order reference hashing tolerates unavailable page-realm typed-array access',async()=>{
 const domain={orderList:[{orderId:'synthetic-order',orderDate:'2026-01-01',totalPrice:1000,items:[]}],hasNext:false};
 const expected=harness({domain}),actual=harness({domain,nonIterableBytes:true});
 expected.poll();actual.poll();
 for(let i=0;i<20;i++){await flush();if(expected.poll().status!=='loading'&&actual.poll().status!=='loading')break;}
 assert.equal(actual.poll().status,'ok');
 assert.deepEqual(actual.poll(),expected.poll());
 assert.match(actual.poll().page.orders[0].source_ref,/^[a-f0-9]{64}$/);
});
