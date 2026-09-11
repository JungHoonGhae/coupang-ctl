import test from 'node:test';
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {mkdtemp, mkdir, writeFile, readFile, rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {EventEmitter} from 'node:events';

const runner = new URL('../internal/browser/camofox_runner.mjs', import.meta.url);

// Run the production program with deterministic dependency boundaries. Only
// imports are replaced; no lifecycle statements are rewritten. This needs no
// experimental VM flags, real processes, filesystem state, or network access.
async function cancelRunnerDuring(phase) {
  const source = await readFile(runner, 'utf8');
  const imports = source.match(/^import .*;$/gm);
  assert.equal(imports.length, 6);
  const program = source.replace(/^import .*;\n/gm, '');
  const signals = new Map();
  let cancelled = false, spawned = 0, killed = 0, stdout = '';
  const cancelAt = boundary => {
    if (phase !== boundary || cancelled) return;
    cancelled = true;
    signals.get('SIGINT')();
    signals.get('SIGTERM')();
  };
  const server = new EventEmitter();
  Object.assign(server, {
    exitCode: null, signalCode: null,
    stdout: new EventEmitter(), stderr: new EventEmitter(),
    kill(signal) {
      assert.equal(signal, 'SIGTERM');
      killed++;
      queueMicrotask(() => { server.exitCode = 0; server.emit('close'); });
      return true;
    },
  });
  const input = JSON.stringify({
    config: {node: '/synthetic/node', server: '/synthetic/server.mjs', engine_dir: '/synthetic/engine', user_id: 'synthetic'},
    state_dir: '/synthetic/state', mode: 'orders', script: "console.log('COUPANGCTL_RESULT {}')",
  });
  const process = {
    umask() {}, env: {}, ppid: 2,
    stdin: (async function* () { yield Buffer.from(input); })(),
    on(signal, handler) { signals.set(signal, handler); },
    stdout: {write(line) { stdout += line; }},
    exit() { assert.fail('synthetic input must be accepted'); },
  };
  const dependencies = {
    process, Buffer, AbortController, AbortSignal, URL,
    spawn() { spawned++; return server; },
    randomBytes: () => Buffer.alloc(32),
    async stat() { cancelAt('stat'); return {isFile: () => true}; },
    async mkdir() { cancelAt('mkdir'); },
    dirname: () => '/synthetic', join,
    net: {createServer: () => ({
      listen(_port, _host, ready) { ready(); },
      address: () => ({port: 12345}),
      close(done) { cancelAt('port'); done(); },
    })},
    async fetch() { cancelAt('post-spawn'); throw new Error('synthetic cancelled request'); },
    delay: async () => {}, setInterval: () => ({unref() {}}), clearInterval() {},
  };
  const AsyncFunction = Object.getPrototypeOf(async function() {}).constructor;
  await new AsyncFunction(...Object.keys(dependencies), program)(...Object.values(dependencies));
  return {cancelled, spawned, killed, stdout, code: process.exitCode, serverExited: server.exitCode === 0};
}

for (const phase of ['stat', 'mkdir', 'port']) {
  test(`startup cancellation during ${phase} never starts a server`, async () => {
    const result = await cancelRunnerDuring(phase);
    assert.equal(result.cancelled, true);
    assert.equal(result.spawned, 0);
    assert.equal(result.killed, 0);
    assert.equal(result.code, 1);
    assert.equal(result.stdout, '');
  });
}

test('post-spawn cancellation waits for one server shutdown and emits no result', async () => {
  const result = await cancelRunnerDuring('post-spawn');
  assert.equal(result.cancelled, true);
  assert.equal(result.spawned, 1);
  assert.equal(result.killed, 1);
  assert.equal(result.serverExited, true);
  assert.equal(result.code, 1);
  assert.equal(result.stdout, '');
});

async function runFixture(t, {shutdown = 'clean', persistence = true, page = false, mode = 'search', script = "console.log('COUPANGCTL_RESULT '+JSON.stringify({result:{status:'ok'}}))"} = {}) {
  const dir = await mkdtemp(join(tmpdir(), 'camofox-runner-test-'));
  t.after(() => rm(dir, {recursive: true, force: true}));
  await mkdir(join(dir, 'engine'));
  await writeFile(join(dir, 'engine/version.json'), '{}');
  const server = join(dir, 'server.mjs');
  await writeFile(server, `
    import http from 'node:http';
    import {writeFileSync} from 'node:fs';
    const e=process.env;
    const guarded=e.CAMOFOX_BIND_HOST==='127.0.0.1' && e.CAMOFOX_INTERACTIVE===${JSON.stringify(mode==='login'?'desktop':'off')} &&
      e.CAMOFOX_CRASH_REPORT_ENABLED==='false' && e.CAMOFOX_DISABLE_DEFAULT_ADDONS==='1' &&
      !e.HTTP_PROXY && !e.HTTPS_PROXY && !e.CAMOFOX_PROXY_SERVER;
    const s=http.createServer((req,res)=>{
      if(!guarded || req.headers.authorization!=='Bearer '+e.CAMOFOX_ACCESS_KEY){res.writeHead(401);res.end('{}');return;}
      if(req.url==='/health'){res.end('{}');return;}
      if(${page}){
        if(req.url==='/tabs'){res.end(JSON.stringify({tabId:'synthetic'}));return;}
        if(req.url==='/tabs/synthetic/evaluate'){res.end(JSON.stringify({result:{approvalCode:'42',png:Buffer.from('synthetic-image-only').toString('base64')}}));return;}
        if(req.method==='DELETE'){res.end('{}');return;}
      }
      res.writeHead(500);res.end(JSON.stringify({error:'Execution context was destroyed synthetic-private-value'}));
    });
    s.listen(Number(e.CAMOFOX_PORT),'127.0.0.1');
    if(${persistence})console.log(JSON.stringify({msg:'plugin loaded',plugin:'persistence'}));
    process.on('SIGTERM',()=>{
      if(${JSON.stringify(shutdown)}==='hang')return;
      if(${JSON.stringify(shutdown)}==='persist-error')console.log(JSON.stringify({msg:'failed to persist storage state',error:'synthetic-private-value'}));
      writeFileSync(${JSON.stringify(join(dir, 'checkpoint'))},'closed');
      process.exit(${shutdown === 'error' ? 1 : 0});
    });
  `);
  const child = spawn(process.execPath, [runner.pathname], {
    env: {...process.env, HTTP_PROXY: 'http://synthetic.invalid', HTTPS_PROXY: 'http://synthetic.invalid', CAMOFOX_PROXY_SERVER: 'http://synthetic.invalid'},
    stdio: ['pipe', 'pipe', 'pipe'],
  });
  const timeout = setTimeout(() => child.kill('SIGINT'), 13000);
  t.after(() => clearTimeout(timeout));
  child.stdin.end(JSON.stringify({config: {node: process.execPath, server, engine_dir: join(dir, 'engine'), user_id: 'synthetic'}, state_dir: join(dir, 'state'), script, mode}));
  let stdout = '', stderr = '';
  child.stdout.on('data', b => { stdout += b; });
  child.stderr.on('data', b => stderr += b);
  const code = await new Promise((resolve, reject) => {child.on('close', resolve);child.on('error', reject);});
  return {code, stdout, stderr, dir};
}

test('isolated runner returns a result only after clean shutdown', async t => {
  const r = await runFixture(t);
  assert.equal(r.code, 0);
  assert.equal(await readFile(join(r.dir, 'checkpoint'), 'utf8'), 'closed');
  assert.match(r.stdout, /^COUPANGCTL_RESULT /);
  assert.equal(r.stderr, '');
});

for (const shutdown of ['error', 'hang', 'persist-error']) {
  test(`runner refuses success after ${shutdown} shutdown`, async t => {
    const r = await runFixture(t, {shutdown});
    assert.notEqual(r.code, 0);
    assert.equal(r.stdout, '');
    assert.equal(r.stderr, '');
  });
}

test('missing persistence plugin fails before a page can be acquired', async t => {
  const r=await runFixture(t,{persistence:false});assert.notEqual(r.code,0);assert.equal(r.stdout,'');assert.equal(r.stderr,'');
});

test('only explicit login operation selects a desktop runtime', async t => {
  const r=await runFixture(t,{mode:'login'});assert.equal(r.code,0);assert.equal(r.stderr,'');
});

test('SMS and OTP operations are rejected before browser startup',async t=>{
  for(const mode of ['login_phone','login_sms','login_otp']){
    const r=await runFixture(t,{mode});
    assert.notEqual(r.code,0);assert.equal(r.stdout,'');assert.equal(r.stderr,'');
    await assert.rejects(readFile(join(r.dir,'checkpoint')), {code:'ENOENT'});
  }
});

test('protected navigation is confined to order and authentication operations', async t => {
  const script=`try{await openTab('https://mc.coupang.com/ssr/desktop/order/list')}catch{console.log('COUPANGCTL_RESULT '+JSON.stringify({error:'synthetic'}))}`;
  const denied=await runFixture(t,{script});assert.equal(denied.code,0);
  assert.equal(JSON.parse(denied.stdout.slice('COUPANGCTL_RESULT '.length)).runtime_error,undefined);
  const allowed=await runFixture(t,{mode:'orders',script});assert.equal(allowed.code,0);
  assert.equal(JSON.parse(allowed.stdout.slice('COUPANGCTL_RESULT '.length)).runtime_error.reason,'document_changed');
});

test('runtime diagnostics classify errors without exposing payloads', async t => {
  const r = await runFixture(t, {script: `try {await openTab('https://www.coupang.com/np/search?q=synthetic')}catch{console.log('COUPANGCTL_RESULT '+JSON.stringify({error:'filter_unavailable'}))}`});
  assert.equal(r.code, 0);
  const data = JSON.parse(r.stdout.slice('COUPANGCTL_RESULT '.length));
  assert.equal(data.runtime_error.reason, 'document_changed');
  assert.equal(r.stdout.includes('synthetic-private-value'), false);
  assert.equal(r.stderr, '');
});

test('search accepts category navigation but rejects ambiguous or unrelated targets',async t=>{
 for(const [url,allowed] of [
  ['https://www.coupang.com/np/categories/123456',true],
  ['https://www.coupang.com/np/categories/123456?sorter=bestAsc',true],
  ['https://www.coupang.com/np/categories/123456?sorter=scoreDesc',false],
  ['https://www.coupang.com/np/search?q=synthetic&sorter=bestAsc',false],
  ['https://www.coupang.com/np/categories/123456?sorter=salePriceAsc&page=2',true],
  ['https://www.coupang.com/np/search?q=synthetic&sorter=scoreDesc&page=1',true],
  ['https://www.coupang.com/np/categories/abc',false],
  ['https://www.coupang.com/np/categories/123456?q=synthetic',false],
  ['https://www.coupang.com/np/categories/123456?q=',false],
  ['https://www.coupang.com/np/categories/123456?sorter=unknown',false],
  ['https://www.coupang.com/np/categories/123456?page=2&page=3',false],
  ['https://www.coupang.com/np/categories/123456?page=0',false],
  ['https://www.coupang.com/np/categories/123456?page=101',false],
  ['https://www.coupang.com/np/categories/123456?submit=true',false],
  ['https://www.coupang.com/np/categories/123456#checkout',false],
  ['https://www.coupang.com/np/search?q=synthetic&q=other',false],
  ['https://www.coupang.com/np/search',false],
  ['https://example.invalid/np/categories/123456',false],
 ]){
  const script=`try{await openTab(${JSON.stringify(url)})}catch{console.log('COUPANGCTL_RESULT '+JSON.stringify({error:'synthetic'}))}`;
  const r=await runFixture(t,{script});assert.equal(r.code,0);
  assert.equal(!!JSON.parse(r.stdout.slice('COUPANGCTL_RESULT '.length)).runtime_error,allowed,url);
 }
});

test('detail and category navigation are confined to numeric product identities and option parameters',async t=>{
 for(const mode of ['inspect','category']) {
  for(const [url,allowed] of [
    ['https://www.coupang.com/vp/products/101?itemId=201&vendorItemId=301',true],
    ['https://www.coupang.com/vp/products/101',true],
    ['https://www.coupang.com/vp/products/101?itemId=201&itemId=999',false],
    ['https://www.coupang.com/vp/products/101?submit=true',false],
    ['https://www.coupang.com/vp/products/abc',false],
    ['https://www.coupang.com/vp/products/101#checkout',false],
    ['https://example.invalid/vp/products/101',false],
    ['https://mc.coupang.com/ssr/desktop/order/list',false],
  ]){
    const script=`try{await openTab(${JSON.stringify(url)})}catch{console.log('COUPANGCTL_RESULT '+JSON.stringify({error:'synthetic'}))}`;
    const r=await runFixture(t,{mode,script});assert.equal(r.code,0);
    assert.equal(!!JSON.parse(r.stdout.slice('COUPANGCTL_RESULT '.length)).runtime_error,allowed);
  }
 }
});

test('QR image capture is accessible only in explicit link mode and never logged by the runner', async t => {
  const script=`const p=await openTab('https://mc.coupang.com/ssr/desktop/order/list');let captured=false;try{const image=await p.captureQR();captured=image.approvalCode==='42'&&!!image.png;}catch{}finally{await p.close();}console.log('COUPANGCTL_RESULT '+JSON.stringify({captured}));`;
  for(const mode of ['orders','login','login_link']){
    const r=await runFixture(t,{mode,page:true,script});assert.equal(r.code,0);assert.equal(r.stderr,'');
    assert.deepEqual(JSON.parse(r.stdout.slice('COUPANGCTL_RESULT '.length)),{captured:mode==='login_link'});
    assert.equal(r.stdout.includes('synthetic-image-only'),false);
  }
});

test('account operation permits only the two exact read bootstrap pages',async t=>{
  for(const [url,allowed] of [
    ['https://loyalty.coupang.com/loyalty/management/home',true],
    ['https://cash.coupang.com/coupang-cash/home',true],
    ['https://cash.coupang.com/coupang-cash/home?page=1',false],
    ['https://cash.coupang.com/api/cash/transactions',false],
    ['https://loyalty.coupang.com/loyalty/management/cancel',false],
    ['https://user:pass@cash.coupang.com/coupang-cash/home',false],
    ['https://mc.coupang.com/ssr/desktop/order/list',false],
    ['https://example.invalid/coupang-cash/home',false],
  ]) {
    const script=`try{await openTab(${JSON.stringify(url)})}catch{console.log('COUPANGCTL_RESULT '+JSON.stringify({error:'synthetic'}))}`;
    const r=await runFixture(t,{mode:'account',script}); assert.equal(r.code,0);
    assert.equal(!!JSON.parse(r.stdout.slice('COUPANGCTL_RESULT '.length)).runtime_error,allowed);
  }
});

test('ephemeral QR events are rejected outside link mode and cannot be repeated', async t => {
  const qr="console.log('COUPANGCTL_QR_IMAGE '+JSON.stringify({png:'synthetic',approvalCode:'42'}));";
  const result="console.log('COUPANGCTL_RESULT {}');";
  for(const mode of ['search','inspect','category','account','orders','login']){
    const r=await runFixture(t,{mode,script:qr+result});assert.notEqual(r.code,0);assert.equal(r.stdout,'');
  }
  const r=await runFixture(t,{mode:'login_link',script:qr+qr+result});assert.notEqual(r.code,0);
  assert.equal(r.stdout.trim().split('\n').length,1);assert.equal(r.stdout.includes('COUPANGCTL_RESULT'),false);
});
