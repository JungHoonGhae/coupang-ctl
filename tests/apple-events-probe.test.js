import test from "node:test";
import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import {validateTarget,searchURL,validateResult} from "../research/probes/chrome-apple-events.mjs";
const profile="/tmp/coupangctl-apple-events.synthetic";
const command="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome --user-data-dir="+profile+" --no-first-run about:blank";
test("dedicated process guard rejects ordinary Chrome and debugging launches",()=>{
    assert.doesNotThrow(()=>validateTarget(123,profile,command));
    for(const invalid of [command.replace("--user-data-dir="+profile,""),command+" --remote-debugging-port=0",command+" --headless=new",command+" --load-extension=/tmp/ext",command+" --user-data-dir=/tmp/other",command.replace("Google Chrome.app","Other.app")]) assert.throws(()=>validateTarget(123,profile,invalid));
    for(const path of ["/",profile+"/../Default","/Users/synthetic/Chrome"]) assert.throws(()=>validateTarget(123,path,command));
    assert.throws(()=>validateTarget(0,profile,command));
});
test("search URL encodes input as a query rather than a navigation target",()=>{
    const url=new URL(searchURL('synthetic & q=other "'));
    assert.equal(url.origin,"https://www.coupang.com"); assert.equal(url.pathname,"/np/search");
    assert.equal(url.searchParams.size,1); assert.equal(url.searchParams.get("q"),'synthetic & q=other "');
    for(const query of [""," ","x".repeat(201)]) assert.throws(()=>searchURL(query));
});
test("success requires product identity and name, not bytes or status alone",()=>{
    const item={product_id:"123",name:"Synthetic bowl",url:"https://www.coupang.com/vp/products/123",current_amount:1200};
    assert.deepEqual(validateResult({status:"ok",search:{items:[item]}}),[item]);
    for(const result of [{status:"access_denied"},{status:"ok",search:{items:[]}},...[{name:""},{url:"https://example.test/vp/products/123"},{current_amount:-1}].map(change=>({status:"ok",search:{items:[{...item,...change}]}}))]) assert.throws(()=>validateResult(result));
});
test("transport uses PID-bound ScriptingBridge and never activates or restores",()=>{
    const source=readFileSync(new URL("../research/probes/apple-events-transport.js",import.meta.url),"utf8");
    assert.match(source,/applicationWithProcessIdentifier\(input.pid\)/);
    assert.doesNotMatch(source,/Application\(input.pid\)|\.activate\(|\.launch\(|numberWithBool\(false\)/);
    assert.match(source,/target_changed/); assert.match(source,/unrelated_tab/);
    assert.match(source,/window\.location\.assign/);
    assert.doesNotMatch(source,/tab\.setValueForKey/);
});
