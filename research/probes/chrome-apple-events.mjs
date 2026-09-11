// Bounded live diagnostic. Requires an already-running dedicated normal Chrome.
// No extension needs to be installed; reuse only the existing pure DTO reader.
import {execFileSync} from "node:child_process";
import {fileURLToPath} from "node:url";
import {readSelectedSearchPage} from "../../internal/browser/search_page_reader.js";

export function validateTarget(pid, profile, command) {
    if (!Number.isSafeInteger(pid) || pid < 2 || !/^\/tmp\/coupangctl-apple-events\.[A-Za-z0-9]+$/.test(profile)) throw new Error("invalid_target");
    const prefix="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome ";
    if (!command.startsWith(prefix)) throw new Error("target_identity_mismatch");
    const args=command.slice(prefix.length).trim().split(/\s+/);
    if (args.filter(arg=>arg.startsWith("--user-data-dir=")).join() !== "--user-data-dir="+profile ||
        args.some(arg=>/^--(?:remote-debugging|headless|load-extension)/.test(arg))) throw new Error("target_identity_mismatch");
}
export function searchURL(query) {
    if (typeof query !== "string" || !query.trim() || query.length > 200) throw new Error("invalid_query");
    const url = new URL("https://www.coupang.com/np/search"); url.searchParams.set("q",query); return url.href;
}
export function validateResult(result) {
    if (result?.status !== "ok" || !Array.isArray(result.search?.items) || !result.search.items.length || result.search.items.length>60) throw new Error("missing_product_data");
    for (const item of result.search.items) {
        const u = new URL(item.url);
        if (!/^\d{1,24}$/.test(item.product_id) || typeof item.name!=="string" || !item.name.trim() || item.name.length>400 ||
            u.origin !== "https://www.coupang.com" || u.pathname !== "/vp/products/"+item.product_id || u.username || u.password || u.hash ||
            [...u.searchParams].some(([key,value])=>!["itemId","vendorItemId"].includes(key) || !/^\d{1,24}$/.test(value)) ||
            (item.current_amount !== undefined && (!Number.isSafeInteger(item.current_amount) || item.current_amount<=0 || item.current_amount>1e12))) throw new Error("invalid_product_data");
    }
    return result.search.items.slice(0,3).map(item=>({product_id:item.product_id,name:item.name,url:item.url,
        ...(item.current_amount===undefined?{}:{current_amount:item.current_amount})}));
}
export async function main(args) {
    const [operation, rawPID, profile, query] = args;
    const pid=Number(rawPID), samples=[];
    let expected;
    function call(operation, extra={}) {
        const command=execFileSync("/bin/ps",["-p",String(pid),"-o","command="],{encoding:"utf8",timeout:2000});
        validateTarget(pid,profile,command);
        const response=JSON.parse(execFileSync("/usr/bin/osascript",["-l","JavaScript",fileURLToPath(new URL("./apple-events-transport.js",import.meta.url)),
            JSON.stringify({pid,operation,...expected,...extra})],{encoding:"utf8",timeout:12000,maxBuffer:256000}));
        if(response.status!=="ok") throw new Error(response.reason || response.status);
        if(expected && (response.window_id!==expected.window_id || response.tab_id!==expected.tab_id)) throw new Error("target_changed");
        samples.push({at:new Date().toISOString(),minimized:response.minimized});
        return response;
    }
    const initial=call("status"); expected={window_id:initial.window_id,tab_id:initial.tab_id};
    if(operation==="status") return initial;
    if(operation==="permission" || operation==="minimize") return call(operation);
    if(operation!=="search") throw new Error("invalid_operation");
    const url=searchURL(query);
    if(!initial.minimized) throw new Error("window_not_minimized");
    call("permission");
    call("navigate",{url});
    const script="JSON.stringify(("+readSelectedSearchPage.toString()+")("+JSON.stringify(url)+"))";
    for(let i=0;i<20;i++) {
        await new Promise(resolve=>setTimeout(resolve,750));
        const response=call("read",{script});
        if(!response.minimized) throw new Error("window_not_minimized");
        if(response.result.status==="ok") {
            const products=validateResult(response.result);
            const final=call("status");
            if(!final.minimized) throw new Error("window_not_minimized");
            return {status:"ok",source:"apple_events_diagnostic",pid,...expected,query,products,minimized_samples:samples};
        }
        if(!["loading","structured_data_missing","ordinary_browser_unavailable"].includes(response.result.status)) throw new Error(response.result.status);
    }
    throw new Error("product_data_timeout");
}
if (process.argv[1]===fileURLToPath(import.meta.url)) {
    try { console.log(JSON.stringify(await main(process.argv.slice(2)))); }
    catch(error) {
        // exec errors may contain the script/page result; output known stage codes only.
        const reason=/^[a-z_]+$/.test(error.message)?error.message:"probe_transport_failed";
        console.log(JSON.stringify({status:"unavailable",reason})); process.exitCode=1;
    }
}
