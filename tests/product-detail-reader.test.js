import assert from 'node:assert/strict';
import test from 'node:test';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const shared=readFileSync(new URL('../internal/browser/product_inspection_reader.js',import.meta.url),'utf8');
const wrapper=readFileSync(new URL('../internal/browser/product_detail_poll.js',import.meta.url),'utf8').replace('/* SHARED_READER */',shared);
function harness({pending=false,navigation=[]}={}) {
 const calls=[],timers=new Map();let timerID=0;
 const location=new URL('https://www.coupang.com/vp/products/101?itemId=201');
 const product={'@type':'Product',url:location.href,name:'Synthetic',offers:{price:0,priceCurrency:'KRW',url:location.href}};
 const document={readyState:'complete',title:'Synthetic',body:{innerText:''},querySelector:()=>null,querySelectorAll:s=>s.includes('ld+json')?[{textContent:JSON.stringify(product)}]:[]};
 const window={};
 const context=vm.createContext({URL,location,document,window,performance:{getEntriesByType:type=>type==='navigation'?navigation:[]},AbortController,setTimeout:(fn,ms)=>{const id=++timerID;if(ms===10000)timers.set(id,fn);else queueMicrotask(fn);return id;},clearTimeout:id=>timers.delete(id),fetch:async(url,options)=>{
  calls.push({url,signal:options.signal});
  if(pending)await new Promise((resolve,reject)=>options.signal.addEventListener('abort',()=>reject(new Error('cancelled')),{once:true}));
  return {ok:true,json:async()=>({})};
 }});
 vm.runInContext(wrapper,context);
 const poll=(key='request1')=>JSON.parse(context.pollProductInspection({product_id:'101',item_id:'201'},key));
 return {poll,calls,context,window,document,location,expire:()=>[...timers.values()].forEach(fn=>fn())};
}
const flush=()=>new Promise(resolve=>setImmediate(resolve));

test('current document HTTP 403 is denied even without a recognized error phrase',()=>{
 const name='https://www.coupang.com/vp/products/101?itemId=201';
 const h=harness({navigation:[{name,responseStatus:403}]});
 h.document.body.innerText='Synthetic source error';
 assert.equal(h.poll().status,'access_denied');
 assert.equal(h.calls.length,0);assert.deepEqual(Object.keys(h.window),[]);
 for(const entry of [{name:name.replace('201','999'),responseStatus:403},{name,responseStatus:200},{name,responseStatus:0}]) {
  const unaffected=harness({navigation:[entry]});
  assert.equal(unaffected.poll().status,'loading');
 }
});

test('detail poll starts the shared reader once and returns exact-option evidence',async()=>{
 const h=harness();assert.equal(h.poll().status,'loading');h.poll();await flush();
 const result=h.poll();assert.equal(result.status,'ok');
 assert.equal(result.inspection.product.item_id,'201');assert.equal(result.inspection.product.current_amount,0);
 assert.equal(result.inspection.product.field_evidence.find(e=>e.field==='price.current_amount').scope,'selected_option');
 assert.equal(h.calls.length,1); // review read only: no unobserved vendor option request
});

test('deadline aborts pending requests and never converts timeout into success',async()=>{
 const h=harness({pending:true});h.poll();await flush();assert.equal(h.calls.length,1);
 h.expire();await flush();assert.equal(h.calls[0].signal.aborted,true);assert.equal(h.poll().status,'inspection_failed');
});

test('removed generation cannot publish a late result into another request',async()=>{
 const h=harness();h.poll('old');const state=h.window.old;delete h.window.old;
 h.poll('new');await flush();assert.equal(h.window.old,undefined);assert.equal(h.poll('new').status,'ok');
 assert.equal(state.status,'loading');
});

test('wrong option, login, block and non-ready page do not start detail endpoints',()=>{
 for(const kind of ['option','login','block','loading']){
  const h=harness();
  if(kind==='option')h.location.search='?itemId=999';
  if(kind==='login')h.location.href='https://login.coupang.com/login/login.pang';
  if(kind==='block')h.document.body.innerText='CAPTCHA';
  if(kind==='loading')h.document.readyState='interactive';
  const result=h.poll();assert.equal(result.status,{option:'inspection_failed',login:'authentication_required',block:'access_denied',loading:'loading'}[kind]);assert.equal(h.calls.length,0);
 }
});
