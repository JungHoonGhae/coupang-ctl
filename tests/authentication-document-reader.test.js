import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const source = readFileSync(new URL('../internal/browser/authentication_document_poll.js', import.meta.url), 'utf8');
const target = 'https://mc.coupang.com/ssr/desktop/order/list';
const endpoint = 'https://mc.coupang.com/ssr/api/member/auth';

function harness({url=target, status=200, body='', ready='complete', password=false, value='true', responseStatus=200, contentType='application/json', fetchImpl}={}) {
  const calls=[], timers=new Map(), window={};
  const location=new URL(url);
  const document={title:'Synthetic',readyState:ready,body:{innerText:body},querySelector(selector){
    assert.equal(selector,'input[type="password"]','authentication must not inspect order or member data');
    return password?{}:null;
  }};
  const context=vm.createContext({URL,window,location,document,AbortController,TextDecoder,
    performance:{getEntriesByType:()=>[{name:url,responseStatus:status}]},
    setTimeout(fn){const id=timers.size+1;timers.set(id,fn);return id;},clearTimeout(id){timers.delete(id);},
    fetch:async (url,options)=>{
      calls.push({url,options});
      if(fetchImpl)return fetchImpl(url,options);
      const response=new Response(value,{status:responseStatus,headers:{'content-type':contentType}});
      Object.defineProperty(response,'url',{value:endpoint});
      return response;
    },
  });
  vm.runInContext(source,context);
  const poll=()=>JSON.parse(context.pollAuthenticationPage('synthetic_auth'));
  const settle=async()=>{
    let result=poll();
    for(let i=0;i<20&&result.status==='loading';i++){await new Promise(resolve=>setImmediate(resolve));result=poll();}
    return result;
  };
  return {poll,settle,calls,timers,window,location,document};
}

test('only native true verifies authentication; no orders or private payload leave the page',async()=>{
  const h=harness();
  assert.deepEqual(await h.settle(),{status:'ok',authenticated:true});
  assert.deepEqual(h.poll(),{status:'ok',authenticated:true});
  assert.equal(h.calls.length,1);
  assert.equal(h.calls[0].url,endpoint);
  assert.equal(h.calls[0].options.method,'GET');
  assert.equal(h.calls[0].options.credentials,'include');
  assert.equal(h.calls[0].options.redirect,'error');
  assert.equal(h.calls[0].options.cache,'no-store');
  assert.equal(h.timers.size,0);
  assert.deepEqual(await harness({value:'false'}).settle(),{status:'authentication_required'});
});

test('malformed, missing, coercible, private or oversized replies remain unknown',async()=>{
  for(const value of ['', 'null', '0', '1', '"true"', '{}', '[]', '{"authenticated":true,"name":"synthetic-private"}', 'true false', ' '.repeat(1025)+'true']) {
    assert.deepEqual(await harness({value}).settle(),{status:'authentication_data_missing'},value.slice(0,60));
  }
  assert.deepEqual(await harness({value:'true',contentType:'text/html'}).settle(),{status:'authentication_data_missing'});
});

test('source HTTP failures distinguish expired auth, access block and unknown without retry',async()=>{
  for(const [responseStatus,status] of [[401,'authentication_required'],[403,'access_denied'],[429,'access_denied'],[500,'authentication_data_missing']]) {
    const h=harness({responseStatus,value:'synthetic-private-error'});
    assert.deepEqual(await h.settle(),{status});
    assert.equal(h.calls.length,1);
  }
  const h=harness({fetchImpl:async()=>{throw Error('synthetic-private-url-or-token');}});
  assert.deepEqual(await h.settle(),{status:'authentication_data_missing'});
});

test('unexpected bootstrap, denied pages and login controls never invoke the endpoint',()=>{
  for(const [options,status] of [
    [{url:'https://login.coupang.com/login/login.pang'},'authentication_required'],
    [{url:target+'?unexpected=1'},'authentication_data_missing'],
    [{url:target+'#fragment'},'authentication_data_missing'],
    [{url:target.replace('mc.','other.')},'authentication_data_missing'],
    [{url:target.replace('https://','https://synthetic@')},'authentication_data_missing'],
    [{password:true},'authentication_required'],
    [{status:401},'authentication_required'],[{status:403},'access_denied'],[{status:429},'access_denied'],
    [{status:500},'authentication_data_missing'],[{body:'CAPTCHA'},'access_denied'],[{ready:'loading'},'loading'],
  ]){
    const h=harness(options);assert.deepEqual(h.poll(),{status});assert.equal(h.calls.length,0);
  }
});

test('a late response cannot verify a changed document or bypass the timeout',async()=>{
  let resolve;
  const h=harness({fetchImpl:()=>new Promise(r=>{resolve=r;})});
  assert.equal(h.poll().status,'loading');
  h.location.href='https://example.invalid/';
  const response=new Response('true',{headers:{'content-type':'application/json'}});
  Object.defineProperty(response,'url',{value:endpoint});
  resolve(response);
  await new Promise(r=>setImmediate(r));
  assert.deepEqual(h.poll(),{status:'authentication_data_missing'});
  assert.equal(h.calls[0].options.signal.aborted,true);

  let finish;
  const slow=harness({fetchImpl:()=>new Promise(r=>{finish=r;})});
  slow.poll();[...slow.timers.values()][0]();
  assert.equal(slow.calls[0].options.signal.aborted,true);
  const late=new Response('true',{headers:{'content-type':'application/json'}});
  Object.defineProperty(late,'url',{value:endpoint});
  finish(late);await new Promise(r=>setImmediate(r));
  assert.deepEqual(slow.poll(),{status:'authentication_data_missing'});
});

test('a successful response from an unexpected destination cannot verify authentication',async()=>{
  const h=harness({fetchImpl:async()=>{
    const response=new Response('true',{headers:{'content-type':'application/json'}});
    Object.defineProperty(response,'url',{value:'https://example.invalid/'});
    return response;
  }});
  assert.deepEqual(await h.settle(),{status:'authentication_data_missing'});
});
