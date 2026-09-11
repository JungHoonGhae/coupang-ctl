// Private transport for the bundled search program; not an arbitrary-JS CLI.
// Each request owns its server/browser and closes them without opening a window.
import {spawn} from 'node:child_process';
import {randomBytes} from 'node:crypto';
import {mkdir, stat} from 'node:fs/promises';
import {dirname, join} from 'node:path';
import net from 'node:net';
import {setTimeout as delay} from 'node:timers/promises';

process.umask(0o077);

// One bounded configuration document; there is no phone/OTP input channel.
let initial;
try{
 let chunks=[],bytes=0;
 for await(const chunk of process.stdin){bytes+=chunk.length;if(bytes>1<<20)throw Error('invalid_input');chunks.push(chunk);}
 initial=JSON.parse(Buffer.concat(chunks).toString('utf8'));
}catch{process.exit(1);}
const {config,state_dir:stateDir,script,mode='search'}=initial;
initial=undefined;
const key=randomBytes(32).toString('hex');
let server,stopped,stopping=false,cancelled=false,shutdownFailed=false,persistenceLoaded=false;
const controller=new AbortController();
const stop=()=>{
 stopping=true;
 // Cancellation may arrive before spawn; do not cache an empty cleanup.
 if(!server)return;
 return stopped??=(async()=>{
 if(server.exitCode!==null||server.signalCode!==null){shutdownFailed||=server.exitCode!==0;return;}
 server.kill('SIGTERM');
 await Promise.race([new Promise(r=>server.once('close',r)),delay(8000,undefined,{ref:false})]);
 if(server.exitCode===null&&server.signalCode===null){shutdownFailed=true;server.kill('SIGKILL');await new Promise(r=>server.once('close',r));}
 if(server.exitCode!==0)shutdownFailed=true;
 })();
};
for(const signal of ['SIGINT','SIGTERM','SIGHUP'])process.on(signal,()=>{
 cancelled=true;controller.abort();void stop();
});
const parentWatch=setInterval(()=>{if(process.ppid===1){cancelled=true;controller.abort();void stop();}},1000);
parentWatch.unref();
try {
 if(!['search','inspect','category','account','orders','login','login_link'].includes(mode))throw Error('invalid_operation');
 // An installed, complete engine is required. Never download on a search.
 for(const f of [config.server,join(config.engine_dir,'version.json')])if(!(await stat(f)).isFile())throw Error('runtime_missing');
 for(const d of ['profiles','cookies','uploads','traces','tmp'])await mkdir(join(stateDir,d),{recursive:true,mode:0o700});
 const port=await new Promise(resolve=>{const s=net.createServer();s.listen(0,'127.0.0.1',()=>{const p=s.address().port;s.close(()=>resolve(p));});});
 const env=Object.fromEntries(['HOME','PATH','LANG','USER','LOGNAME'].filter(k=>process.env[k]).map(k=>[k,process.env[k]]));
 Object.assign(env,{
  NODE_ENV:'production',CAMOFOX_PORT:String(port),CAMOFOX_BIND_HOST:'127.0.0.1',CAMOFOX_ACCESS_KEY:key,
  CAMOFOX_CRASH_REPORT_ENABLED:'false',CAMOFOX_INTERACTIVE:mode==='login'?'desktop':'off',CAMOFOX_DISABLE_DEFAULT_ADDONS:'1',
  CAMOUFOX_INSTALL_DIR:config.engine_dir,CAMOFOX_PROFILE_DIR:join(stateDir,'profiles'),
  CAMOFOX_COOKIES_DIR:join(stateDir,'cookies'),CAMOFOX_UPLOADS_DIR:join(stateDir,'uploads'),
  CAMOFOX_TRACES_DIR:join(stateDir,'traces'),TMPDIR:join(stateDir,'tmp'),
  MAX_SESSIONS:'1',MAX_TABS_PER_SESSION:'1',MAX_TABS_GLOBAL:'1',MAX_CONCURRENT_PER_USER:'1',
 });
 // No asynchronous boundary may separate this check from owning the child.
 if(cancelled)throw Error('cancelled');
 server=spawn(config.node,[config.server],{cwd:dirname(config.server),env,stdio:['ignore','pipe','pipe']});
 // Reduce upstream diagnostics to lifecycle booleans in memory. Never forward
 // their fields (which can contain authentication URLs or profile contents).
 for(const stream of [server.stdout,server.stderr]){
  let pending='';
  stream.on('data',b=>{
   pending+=b.toString('utf8');if(pending.length>65536){pending='';shutdownFailed=true;return;}
   let i;while((i=pending.indexOf('\n'))>=0){const line=pending.slice(0,i);pending=pending.slice(i+1);
    try{const e=JSON.parse(line);
     if(e.msg==='plugin loaded'&&e.plugin==='persistence')persistenceLoaded=true;
     if(e.msg==='plugin load failed'&&e.plugin==='persistence')shutdownFailed=true;
     if(e.msg==='failed to persist storage state')shutdownFailed=true;
    }catch{}
   }
  });
 }
 let spawnFailed=false;server.on('error',()=>{spawnFailed=true;});
 let apiFailure;
 const api=async(method,path,body)=>{
  if(cancelled||stopping)throw Error('cancelled');
  const response=await fetch('http://127.0.0.1:'+port+path,{method,headers:{'content-type':'application/json',authorization:'Bearer '+key},body:body===undefined?undefined:JSON.stringify(body),signal:AbortSignal.any([controller.signal,AbortSignal.timeout(35000)])});
  let chunks=[],bytes=0;for await(const b of response.body){bytes+=b.length;if(bytes>1<<20)throw Error('response_limit');chunks.push(b);}
  const data=JSON.parse(Buffer.concat(chunks).toString('utf8'));
  if(!response.ok){
   const error=String(data.error||'');
   apiFailure={status:response.status,reason:/execution context.*destroyed|cannot find context|context.*not found/i.test(error)?'document_changed':/timeout|timed out/i.test(error)?'timeout':'request_failed'};
   throw Error('transport_failed');
  }
  return data;
 };
 let ready=false;for(let i=0;i<100;i++){
  if(cancelled||spawnFailed||server.exitCode!==null||server.signalCode!==null)throw Error('runtime_unavailable');
  try{await api('GET','/health');ready=true;break;}catch{}
  await delay(100);
 }
 if(!ready||!persistenceLoaded||shutdownFailed)throw Error('runtime_unavailable');
 const validateURL=url=>{
  const u=new URL(url);
  const detail=u.origin==='https://www.coupang.com'&&/^\/vp\/products\/\d{1,24}$/.test(u.pathname)&&
   [...u.searchParams.keys()].every(k=>['itemId','vendorItemId'].includes(k)&&u.searchParams.getAll(k).length===1&&/^\d{1,24}$/.test(u.searchParams.get(k)));
  const account=['https://loyalty.coupang.com/loyalty/management/home','https://cash.coupang.com/coupang-cash/home'].includes(u.href);
  const categoryList=/^\/np\/categories\/\d{1,24}$/.test(u.pathname);
  const search=u.origin==='https://www.coupang.com'&&(u.pathname==='/np/search'||categoryList)&&
   (categoryList?!u.searchParams.has('q'):!!u.searchParams.get('q')?.trim()&&[...u.searchParams.get('q')].length<=200)&&
   [...u.searchParams.keys()].every(k=>['q','sorter','page'].includes(k)&&u.searchParams.getAll(k).length===1)&&
   (!u.searchParams.has('sorter')||[categoryList?'bestAsc':'scoreDesc','saleCountDesc','latestAsc','salePriceAsc','salePriceDesc'].includes(u.searchParams.get('sorter')))&&
   (!u.searchParams.has('page')||/^[1-9]\d?$|^100$/.test(u.searchParams.get('page')));
  const allowed=mode==='search'?search:['inspect','category'].includes(mode)?detail:mode==='account'?account:u.href==='https://mc.coupang.com/ssr/desktop/order/list';
  if(!allowed||u.username||u.password||u.hash)throw Error('invalid_document_url');
 };
 const openTab=async url=>{
  validateURL(url);
  const t=await api('POST','/tabs',{userId:config.user_id,sessionKey:'product-search',url,trace:false});
  const evaluate=async expression=>(await api('POST','/tabs/'+t.tabId+'/evaluate',{userId:config.user_id,expression})).result;
  // A 200 response can still be the initial empty render. Wait for document
  // evidence, not a successful HTTP status; final classification stays in Go's
  // shared reader. No refresh or challenge action is attempted here.
  for(let i=0;mode==='search'&&i<24;i++){
   const loaded=await evaluate("!!document.querySelector('.filter-function-bar,input[type=password]')||/access denied|captcha|자동입력방지/i.test(document.title+' '+(document.body?.innerText||''))");
   if(loaded)break;await delay(250);
  }
  return {
   evaluate,
   captureQR:async()=>{
    if(mode!=='login_link')throw Error('qr_capture_not_requested');
    const qr=await evaluate(`(()=>{
     if(location.origin!=='https://login.coupang.com')return null;
     const visible=e=>{
      const r=e.getBoundingClientRect(),s=getComputedStyle(e);
      return r.width>0&&r.height>0&&s.display!=='none'&&s.visibility!=='hidden'&&Number(s.opacity)>0;
     };
     const approvalCode=[...document.querySelectorAll('body *')].filter(e=>e.children.length===0&&visible(e)).map(e=>e.textContent?.trim()??'').find(v=>/^\\d{2}$/.test(v))??'';
     const image=[...document.querySelectorAll('img')].find(e=>{
      const r=e.getBoundingClientRect();return visible(e)&&e.complete&&e.naturalWidth>=120&&r.width>=120&&Math.abs(r.width-r.height)<10&&e.src.startsWith('data:image/png;base64,');
     });
     let png=image?image.src.slice('data:image/png;base64,'.length):'';
     const canvas=[...document.querySelectorAll('canvas')].find(e=>{
      const r=e.getBoundingClientRect();return visible(e)&&r.width>=120&&Math.abs(r.width-r.height)<10&&e.width>=120&&e.width===e.height&&e.width<=2048;
     });
     // Decode the original QR pixels, not the CSS-resized viewport image.
     // No screenshot of the rest of the login page is captured.
     if(!png&&canvas){try{png=canvas.toDataURL('image/png').slice('data:image/png;base64,'.length);}catch{}}
     return {approvalCode,png};
    })()`);
    const approvalCode=qr?.approvalCode;
    if(!/^\d{2}$/.test(approvalCode))return null;
    if(qr.png){if(qr.png.length>700000)throw Error('qr_capture_limit');return qr;}
    return null;
   },
   goto:async url=>{
    if(!['login','login_link'].includes(mode))throw Error('navigation_not_allowed');
    validateURL(url);await api('POST','/tabs/'+t.tabId+'/navigate',{userId:config.user_id,url});
   },
   locator:selector=>({nth:index=>({click:async()=>{
    if(selector!=='.filter-function-bar label'||!Number.isInteger(index)||index<0||index>5000)throw Error('invalid_filter');
    await api('POST','/tabs/'+t.tabId+'/click',{userId:config.user_id,selector:selector+' >> nth='+index});
   }})}),
   close:async()=>{await api('DELETE','/tabs/'+t.tabId+'?userId='+encodeURIComponent(config.user_id));},
  };
 };
 // Only the Go adapter's generated program reaches this private boundary.
 // Buffer its sole result until graceful shutdown/persistence succeeds.
 let result,qrSent=false;
 const output={log:line=>{
  if(mode==='login_link'&&typeof line==='string'&&(line.startsWith('COUPANGCTL_QR ')||line.startsWith('COUPANGCTL_QR_IMAGE '))){
   if(qrSent||result!==undefined||Buffer.byteLength(line)>720000)throw Error('invalid_qr_result');
   qrSent=true;process.stdout.write(line+'\n');return;
  }
  if(result!==undefined||typeof line!=='string'||!line.startsWith('COUPANGCTL_RESULT ')||Buffer.byteLength(line)>1<<20)throw Error('invalid_result');
  const value=JSON.parse(line.slice('COUPANGCTL_RESULT '.length));
  if(value.error&&apiFailure)value.runtime_error=apiFailure;
  result='COUPANGCTL_RESULT '+JSON.stringify(value);
 }};
 const AsyncFunction=Object.getPrototypeOf(async function(){}).constructor;
 await new AsyncFunction('openTab','console',script)(openTab,output);
 await stop();
 if(cancelled||shutdownFailed||result===undefined)throw Error('cancelled_or_missing_result');
 process.stdout.write(result+'\n');
} catch {
 // Never expose page URLs, response payloads, credentials, or upstream logs.
 process.exitCode=1;
} finally {
 clearInterval(parentWatch);await stop();
}
