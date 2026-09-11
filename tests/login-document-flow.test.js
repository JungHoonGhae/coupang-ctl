import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const read = path => readFileSync(new URL(path, import.meta.url), 'utf8');
const readerSource = read('../internal/browser/login_document_poll.js').replace('/* AUTH_READER */',
  read('../internal/browser/authentication_document_poll.js'));
const orderURL = 'https://mc.coupang.com/ssr/desktop/order/list';

function reader({url='https://login.coupang.com/login/login.pang',body='',status=200,auth='true'}={}){
  let clicks=0,qrVisible=false;
  class HTMLElement {textContent='QR코드 로그인';click(){clicks++;}}
  const image={children:[],textContent:'',getBoundingClientRect:()=>({width:160,height:160})};
  const code={children:[],textContent:'42',getBoundingClientRect:()=>({width:40,height:40})};
  const document={readyState:'complete',title:'Synthetic',body:{innerText:body},
    querySelectorAll:s=>s==='a,button,[role="tab"],li'?[new HTMLElement()]:s==='img,canvas'?(qrVisible?[image]:[]):s==='body *'?(qrVisible?[code]:[]):[],
    querySelector:s=>{assert.equal(s,'input[type="password"]');return null;}};
  const context=vm.createContext({URL,location:new URL(url),document,window:{},HTMLElement,AbortController,TextDecoder,
    getComputedStyle:()=>({display:'block',visibility:'visible',opacity:'1'}),
    performance:{getEntriesByType:()=>[{name:url,responseStatus:status}]},setTimeout:()=>1,clearTimeout(){},fetch:async endpoint=>{
      assert.equal(endpoint,'https://mc.coupang.com/ssr/api/member/auth');
      const response=new Response(auth,{headers:{'content-type':'application/json'}});
      Object.defineProperty(response,'url',{value:endpoint});return response;
    }});
  vm.runInContext(readerSource,context);
  return {document,showQR:()=>{qrVisible=true;},poll:(activate=false)=>JSON.parse(context.pollSelectedLogin('synthetic',activate)),clicks:()=>clicks};
}
test('CLI QR activation clicks once; readiness is not authentication and exports no secrets',()=>{
  const h=reader();
  assert.deepEqual(h.poll(),{status:'authentication_required'});assert.equal(h.clicks(),0);
  assert.equal(h.poll(true).status,'login_waiting');assert.equal(h.poll(true).status,'login_waiting');assert.equal(h.clicks(),1);
  h.document.body.innerText='휴대폰 카메라로 QR코드를 스캔 synthetic-private-value';
  assert.deepEqual(h.poll(true),{status:'login_waiting'});
  h.showQR();
  assert.deepEqual(h.poll(true),{status:'qr_ready'});
  assert.equal(h.clicks(),1);
});
test('denied/expired login and absent authentication evidence cannot report success',async()=>{
  for(const [options,want] of [[{status:403},'access_denied'],[{body:'CAPTCHA'},'access_denied'],[{body:'QR코드 만료'},'qr_expired']]){
    const h=reader(options);assert.deepEqual(h.poll(true),{status:want});assert.equal(h.clicks(),0);
  }
  const missing=reader({url:orderURL,auth:'{}'});
  missing.poll();await new Promise(resolve=>setImmediate(resolve));assert.equal(missing.poll().status,'authentication_data_missing');
});
test('source authentication verifies login without requiring an order document',async()=>{
  const h=reader({url:orderURL});
  assert.equal(h.poll().status,'loading');await new Promise(resolve=>setImmediate(resolve));
  assert.deepEqual(h.poll(),{status:'ok',authenticated:true});assert.equal(h.clicks(),0);
});
