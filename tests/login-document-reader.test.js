import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';

const script=await readFile(new URL('../internal/browser/login_document_poll.js',import.meta.url),'utf8');
function poll({image=true,code=true,hidden=false,denied=false,activate=true}={}){
  const element=(text,width,height)=>({children:[],textContent:text,getBoundingClientRect:()=>({width,height})});
  const imageElement=element('',160,160),number=element('42',40,40);
  const document={title:denied?'Access Denied':'Login',readyState:'complete',body:{innerText:'QR코드 로그인'},
    querySelectorAll:s=>s==='img,canvas'?(image?[imageElement]:[]):s==='body *'?(code?[number]:[]):[]};
  const context=vm.createContext({URL,location:{href:'https://login.coupang.com/login/login.pang'},document,window:{},HTMLElement:class{},
    getComputedStyle:()=>({display:hidden?'none':'block',visibility:'visible',opacity:'1'}),performance:{getEntriesByType:()=>[]}});
  return JSON.parse(vm.runInContext(script+`;pollSelectedLogin('synthetic',${activate})`,context));
}

test('visible QR image and approval number detect readiness independently of page copy',()=>{
  assert.equal(poll().status,'qr_ready');
  for(const options of [{image:false},{code:false},{hidden:true}])assert.equal(poll(options).status,'login_waiting');
});

test('QR evidence never overrides denial or an unrequested QR interaction',()=>{
  assert.equal(poll({denied:true}).status,'access_denied');
  assert.equal(poll({activate:false}).status,'authentication_required');
});
