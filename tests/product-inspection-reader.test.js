import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import vm from "node:vm";
import test from "node:test";

const source = readFileSync(new URL("../internal/browser/product_inspection_reader.js", import.meta.url), "utf8");
function reader({href="https://www.coupang.com/vp/products/101", productURL, selected={}, products, onFetch, responses={}, nodes={}, scripts=[]}={}) {
    const requests=[];
    const product={"@type":"Product",name:"Synthetic bowl",offers:{price:1200}, ...(productURL?{url:productURL}:{})};
    const location=new URL(href);
    const document={scripts,body:{innerText:"Synthetic product"},querySelector(selector){return nodes[selector]??null;},querySelectorAll(selector){
        if(selector.includes("ld+json")) return [{textContent:JSON.stringify(products ?? product)}];
        if(selector.startsWith('input[name="itemId"]')) return (selected.item_id??[]).map(value=>({value,getAttribute:()=>null}));
        if(selector.startsWith('input[name="vendorItemId"]')) return (selected.vendor_item_id??[]).map(value=>({value,getAttribute:()=>null}));
        return [];
    }};
    const context=vm.createContext({document,location,URL,setTimeout:fn=>fn(),fetch:async url=>{requests.push(url);onFetch?.({location,setProducts(value){products=value;}});return {ok:true,json:async()=>url.includes("quantity-info")?responses.quantity??{}:responses.reviews??{}};}});
    const read=vm.runInContext(`(${source})`,context);
    return {requests,async run(request={product_id:"101"}){return JSON.parse(await read(request));}};
}

function optionDocument(rows=[['RAM용량','32GB'],['저장용량','1TB']]) {
    const optionRows=rows.map(([name,value],i)=>{
        const selectedAttribute={valueId:String(i+1),name:value,selected:true};
        return {name,attributes:[selectedAttribute],selectedAttribute};
    });
    return {optionRows,attributeVendorItemMap:{[optionRows.map(r=>r.selectedAttribute.valueId).join(':')]:{itemId:201,vendorItemId:301}}};
}
function nativeOptionsReader(options, extra={}) {
    const scripts=[{textContent:'self.__next_f.push([1,'+JSON.stringify('0:'+JSON.stringify({options}))+'])'}];
    return reader({selected:{item_id:['201'],vendor_item_id:['301']},scripts,...extra});
}

test('native attributes require a selected tuple mapped to the exact item and seller option',async()=>{
    const h=nativeOptionsReader(optionDocument());
    const r=await h.run({product_id:'101',document_read_limit:1});
    assert.deepEqual(r.selected_attributes,[{name:'RAM용량',value:'32GB'},{name:'저장용량',value:'1TB'}]);
    assert.equal(h.requests.length,0);
    const e=r.field_evidence.find(e=>e.field==='selected_attributes');
    assert.equal(e.source,'product_options');assert.equal(e.provenance,'observed');
    assert.equal(e.scope,'selected_option');
    assert.deepEqual(e.reference,{product_id:'101',item_id:'201',vendor_item_id:'301'});
    assert.ok(r.coverage.observed_fields.includes('selected_attributes'));
});

test('compound option labels and mismatched value counts remain unsplit source statements',async()=>{
    for(const value of ['WIN10 × 4GB × 128GB × 검정','WIN10 × 8GB × 256GB']) {
        const name='운영체제 × 저장용량 × RAM용량 × 색상';
        const r=await nativeOptionsReader(optionDocument([[name,value]])).run();
        assert.deepEqual(r.selected_attributes,[{name,value}]);
    }
});

test('unbound, conflicting, malformed and excessive native option records are unavailable',async()=>{
    const cases=[
        o=>{o.attributeVendorItemMap['1:2'].vendorItemId=999;},
        o=>{o.optionRows[0].attributes.push({...o.optionRows[0].selectedAttribute,valueId:'3'});},
        o=>{o.optionRows[0].selectedAttribute={...o.optionRows[0].selectedAttribute,name:'64GB'};},
        o=>{o.optionRows[0].selectedAttribute.selected='true';},
        o=>{o.optionRows[0].name='';},
        o=>{o.optionRows[0].name='x'.repeat(301);},
        o=>{o.attributeVendorItemMap['1:2'].itemId=Number.MAX_SAFE_INTEGER+1;},
        o=>{o.optionRows[1].name=o.optionRows[0].name;},
    ];
    for(const mutate of cases){
        const o=optionDocument();mutate(o);
        const r=await nativeOptionsReader(o).run();
        assert.deepEqual(r.selected_attributes,[]);
        assert.ok(r.coverage.unavailable_fields.includes('selected_attributes'));
    }
    const r=await nativeOptionsReader(optionDocument(),{selected:{}}).run();
    assert.deepEqual(r.selected_attributes,[]);
});

test('option changes during auxiliary reads fail instead of mixing old specs and new prices',async()=>{
    const scripts=[{textContent:JSON.stringify({options:optionDocument()})}];
    const h=nativeOptionsReader(optionDocument(),{scripts,onFetch(){
        scripts[0].textContent=JSON.stringify({options:optionDocument([['RAM용량','64GB']])});
    }});
    await assert.rejects(()=>h.run(),/identity_changed/);
});

test('conflicting or invalid additional option objects cannot be voted away',async()=>{
    const valid=optionDocument();
    const changed=optionDocument([['RAM용량','64GB'],['저장용량','1TB']]);
    const invalid=optionDocument();invalid.optionRows[0].selectedAttribute.selected=false;
    for(const other of [changed,invalid]){
        const scripts=[{textContent:JSON.stringify({a:{options:valid},b:{options:other}})}];
        const r=await nativeOptionsReader(valid,{scripts}).run();
        assert.deepEqual(r.selected_attributes,[]);
    }
    const r=await nativeOptionsReader(valid,{scripts:[{textContent:JSON.stringify({options:valid})},{textContent:JSON.stringify({options:valid})}]}).run();
    assert.equal(r.selected_attributes.length,2);
});

test("field evidence distinguishes native JSON, numeric text and uncertain endpoint aliases", async () => {
    const native = await reader().run();
    const price = native.product.field_evidence.find(v=>v.field==='price.current_amount');
    assert.equal(price.provenance, 'observed'); assert.equal(price.source, 'json_ld');
    assert.equal(price.scope, 'product'); assert.deepEqual(price.reference, {product_id:'101'});
    assert.ok(Number.isFinite(Date.parse(price.captured_at)));
    const dom = await reader({products:{'@type':'Product',name:'Synthetic'}, nodes:{'[class*="price"] strong':{textContent:'1,200원'}}}).run();
    const parsed = dom.product.field_evidence.find(v=>v.field==='price.current_amount');
    assert.equal(parsed.provenance,'derived'); assert.equal(parsed.source,'dom'); assert.equal(parsed.method,'numeric_parse'); assert.equal(parsed.scope,'unknown');
    const alias = await reader({products:{'@type':'Product',name:'Synthetic'},selected:{vendor_item_id:['301']}, responses:{quantity:{price:{amount:1200}}}}).run();
    const guessed = alias.product.field_evidence.find(v=>v.field==='price.current_amount');
    assert.equal(guessed.provenance,'inferred'); assert.equal(guessed.scope,'unknown'); assert.equal(guessed.source,'quantity_info');
});

test("explicit document allowance includes the already navigated detail and caps endpoint reads", async () => {
    for (const limit of [1, 2, 3]) {
        const h = reader({selected:{vendor_item_id:['301']}});
        const result = await h.run({product_id:'101',document_read_limit:limit});
        assert.equal(h.requests.length, limit - 1);
        assert.equal(result.product.name, 'Synthetic bowl');
        assert.deepEqual(result.coverage.budget_omitted_fields,
            limit===1?['quantity_info','reviews']:limit===2?['reviews']:[]);
        for (const field of result.coverage.budget_omitted_fields) assert.ok(result.coverage.unavailable_fields.includes(field));
    }
});

test("absent quantity target leaves the remaining document allowance for reviews", async () => {
    const h = reader();
    const result = await h.run({product_id:'101',document_read_limit:2});
    assert.equal(h.requests.length,1);
    assert.ok(h.requests[0].includes('/review?'));
    assert.deepEqual(result.coverage.budget_omitted_fields,[]);
});

test("invalid document allowances fail before any endpoint reads", async () => {
    for (const limit of [-1,4,1.5,'2',null]) {
        const h = reader();
        await assert.rejects(()=>h.run({product_id:'101',document_read_limit:limit}),/document_read_limit/);
        assert.equal(h.requests.length,0);
    }
});

test("a failed endpoint consumes its allowance and does not enable another request", async () => {
    const h = reader({selected:{vendor_item_id:['301']}, onFetch(){throw new Error('synthetic network failure');}});
    const result = await h.run({product_id:'101',document_read_limit:2});
    assert.equal(h.requests.length,1);
    assert.ok(h.requests[0].includes('quantity-info'));
    assert.deepEqual(result.coverage.budget_omitted_fields,['reviews']);
    assert.ok(result.coverage.unavailable_fields.includes('quantity_info'));
    assert.ok(result.coverage.unavailable_fields.includes('reviews'));
});

test("fallback evidence names the source actually used", async () => {
    const result = await reader({products:{'@type':'Product',name:'   ',aggregateRating:{ratingAverage:4,ratingCount:12}},nodes:{h1:{textContent:'Synthetic DOM name'}}}).run();
    const fields = Object.fromEntries(result.product.field_evidence.map(v=>[v.field,v]));
    assert.equal(fields.name.source,'dom'); assert.equal(fields.name.locator,'dom.h1');
    assert.equal(fields.rating.source,'json_ld'); assert.equal(fields.rating.locator,'jsonld.Product.aggregateRating.ratingAverage');
    assert.equal(fields.rating.provenance,'inferred');
    assert.equal(fields.review_count.source,'json_ld'); assert.equal(fields.review_count.locator,'jsonld.Product.aggregateRating.ratingCount');
    assert.equal(fields.review_count.provenance,'inferred');
});

test("option price evidence binds only an explicitly matching offer and preserves zero", async () => {
    const result = await reader({selected:{item_id:['201'],vendor_item_id:['301']}, products:{'@type':'Product',name:'Synthetic',offers:{url:'https://www.coupang.com/vp/products/101?itemId=201&vendorItemId=301',price:0}}}).run();
    const price=result.product.field_evidence.find(v=>v.field==='price.current_amount');
    assert.equal(result.product.current_amount,0);assert.equal(price.scope,'selected_option');
    assert.deepEqual(price.reference,{product_id:'101',item_id:'201',vendor_item_id:'301'});
});

test("detail rejects a different observed product before any endpoint read",async()=>{
    const h=reader({href:"https://www.coupang.com/vp/products/999"});
    await assert.rejects(()=>h.run({product_id:"101"}),/identity/);
    assert.equal(h.requests.length,0);
});

test("request and location query alone do not prove the selected option",async()=>{
    for(const href of ["https://www.coupang.com/vp/products/101","https://www.coupang.com/vp/products/101?itemId=201&vendorItemId=301"]){
        const h=reader({href});
        await assert.rejects(()=>h.run({product_id:"101",item_id:"201",vendor_item_id:"301"}),/identity/);
        assert.equal(h.requests.length,0);
    }
});

test("selected form identifiers agree with the request and drive quantity reads",async()=>{
    const h=reader({selected:{item_id:["201"],vendor_item_id:["301"]}});
    const result=await h.run({product_id:"101",item_id:"201",vendor_item_id:"301"});
    assert.equal(result.product.item_id,"201");assert.equal(result.product.vendor_item_id,"301");
    assert.ok(h.requests.some(url=>url.includes("productId=101&vendorItemId=301")));
});

test("sole offer URL independently proves displayed option when Product URL and form IDs are absent",async()=>{
    const href='https://www.coupang.com/vp/products/101?itemId=201&vendorItemId=301';
    const h=reader({href,products:{'@type':'Product',name:'Synthetic',offers:{url:href,price:0,priceCurrency:'KRW'}}});
    const result=await h.run({product_id:'101',item_id:'201',vendor_item_id:'301'});
    assert.equal(result.product.item_id,'201');
    assert.equal(result.product.vendor_item_id,'301');
    const price=result.product.field_evidence.find(v=>v.field==='price.current_amount');
    assert.equal(result.product.current_amount,0);
    assert.equal(price.scope,'selected_option');
    assert.ok(h.requests.some(url=>url.includes('vendorItemId=301')));
});

test("sole offer evidence cannot override conflicting form or Product URL and cannot select among offers",async()=>{
    const href='https://www.coupang.com/vp/products/101?itemId=201&vendorItemId=301';
    const offer={url:href,price:1200};
    const cases=[
        {selected:{item_id:['202']},products:{'@type':'Product',name:'Synthetic',offers:offer}},
        {products:{'@type':'Product',name:'Synthetic',url:href.replace('201','202'),offers:offer}},
        {products:{'@type':'Product',name:'Synthetic',offers:[offer,{...offer,url:href.replace('201','202')}]}},
        {products:{'@type':'Product',name:'Synthetic',offers:{...offer,url:href.replace('/101?','/999?')}}},
    ];
    for(const input of cases){
        const h=reader({href,...input});
        await assert.rejects(()=>h.run({product_id:'101',item_id:'201',vendor_item_id:'301'}),/identity/);
        assert.equal(h.requests.length,0);
    }
});

test("contradictory, malformed or ambiguous selected identifiers fail before reads",async()=>{
    for(const selected of [{item_id:["202"],vendor_item_id:["301"]},{item_id:["201"],vendor_item_id:["302"]},{item_id:["201","202"],vendor_item_id:["301"]},{item_id:["201x"],vendor_item_id:["301"]}]){
        const h=reader({selected});
        await assert.rejects(()=>h.run({product_id:"101",item_id:"201",vendor_item_id:"301"}),/identity/);
        assert.equal(h.requests.length,0);
    }
});

test("product-only read leaves unavailable options empty instead of copying URL parameters",async()=>{
    const h=reader({href:"https://www.coupang.com/vp/products/101?itemId=201&vendorItemId=301"});
    const result=await h.run();
    assert.equal(result.product.item_id,"");assert.equal(result.product.vendor_item_id,"");
    assert.equal(result.product.url,"https://www.coupang.com/vp/products/101");
    assert.equal(h.requests.some(url=>url.includes("quantity-info")),false);
});

test("primary structured Product URL supplies option evidence, not unrelated JSON-LD",async()=>{
    const h=reader({products:[
        {"@type":"Product",name:"Synthetic unrelated",url:"https://www.coupang.com/vp/products/999?itemId=900&vendorItemId=901"},
        {"@type":"Product",name:"Synthetic primary",url:"https://www.coupang.com/vp/products/101?itemId=201&vendorItemId=301"}
    ]});
    const result=await h.run({product_id:"101",item_id:"201",vendor_item_id:"301"});
    assert.equal(result.product.name,"Synthetic primary");
    assert.equal(result.product.item_id,"201");assert.equal(result.product.vendor_item_id,"301");
});

test("structured URL and selected form conflicts are not resolved by source preference",async()=>{
    const h=reader({productURL:"https://www.coupang.com/vp/products/101?itemId=201",selected:{item_id:["202"]}});
    await assert.rejects(()=>h.run(),/identity/);assert.equal(h.requests.length,0);
});

test("ambiguous primary products and malformed navigation options fail before reads",async()=>{
    for(const options of [
        {products:[{"@type":"Product",url:"https://www.coupang.com/vp/products/101"},{"@type":"Product",url:"https://www.coupang.com/vp/products/101"}]},
        {href:"https://www.coupang.com/vp/products/101?itemId=201&itemId=201"},
        {href:"https://www.coupang.com/vp/products/101?vendorItemId="},
        {href:"https://www.coupang.com/vp/products/101?itemId=201",selected:{item_id:["202"]}},
        {href:"https://user:pass@www.coupang.com/vp/products/101"},
        {href:"https://www.coupang.com.evil.test/vp/products/101"}
    ]){
        const h=reader(options);await assert.rejects(()=>h.run(),/identity/);assert.equal(h.requests.length,0);
    }
});

test("identity changes during read cannot return mixed-page evidence",async()=>{
    for(const mode of ["navigation","selection","structured"]){
        const selected={item_id:["201"],vendor_item_id:["301"]};
        const h=reader({selected,onFetch({location,setProducts}){
            if(mode==="navigation") location.href="https://www.coupang.com/vp/products/999";
            if(mode==="selection") selected.item_id=["202"];
            if(mode==="structured") setProducts({"@type":"Product",name:"Synthetic changed"});
        }});
        await assert.rejects(()=>h.run({product_id:"101",item_id:"201",vendor_item_id:"301"}),/identity/);
    }
});

test("multi-offer price belongs to the observed option, not the first offer",async()=>{
    const h=reader({products:{"@type":"Product",name:"Synthetic options",url:"https://www.coupang.com/vp/products/101?itemId=201&vendorItemId=301",offers:[
        {url:"https://www.coupang.com/vp/products/101?itemId=202&vendorItemId=302",price:9900},
        {url:"https://www.coupang.com/vp/products/101?itemId=201&vendorItemId=301",price:1200}
    ]}});
    const result=await h.run({product_id:"101",item_id:"201",vendor_item_id:"301"});
    assert.equal(result.product.current_amount,1200);
});

test("unidentified or conflicting offer prices do not become the selected price",async()=>{
    for(const offers of [[{price:9900}],{url:"https://www.coupang.com/vp/products/101?itemId=202&vendorItemId=302",price:9900}]){
        const h=reader({products:{"@type":"Product",name:"Synthetic options",url:"https://www.coupang.com/vp/products/101?itemId=201&vendorItemId=301",offers}});
        if(!Array.isArray(offers)){
            // A sole offer is now identity evidence too: contradictory identity
            // rejects the document, rather than merely hiding its price.
            await assert.rejects(()=>h.run({product_id:"101",item_id:"201",vendor_item_id:"301"}),/identity_ambiguous/);
            assert.equal(h.requests.length,0);
            continue;
        }
        const result=await h.run({product_id:"101",item_id:"201",vendor_item_id:"301"});
        assert.equal(result.product.observed_fields.includes("price.current_amount"),false);
    }
});

test("single product-level offer remains available for a product-only inspection",async()=>{
    const h=reader({products:{"@type":"Product",name:"Synthetic generic",url:"https://www.coupang.com/vp/products/101",offers:{url:"https://www.coupang.com/vp/products/101",price:1200}}});
    const result=await h.run();
    assert.equal(result.product.current_amount,1200);
    assert.equal(result.product.item_id,"");assert.equal(result.product.vendor_item_id,"");
});

test("known zero values survive fallback selection and granular coverage",async()=>{
    for(const zero of [0,"0"]){
        const h=reader({selected:{item_id:["201"],vendor_item_id:["301"]},products:{"@type":"Product",name:"Synthetic zero",offers:{price:zero},aggregateRating:{ratingValue:zero,reviewCount:zero}},responses:{quantity:{price:1200,originalPrice:zero,discountRate:zero}}});
        const result=await h.run({product_id:"101",item_id:"201",vendor_item_id:"301"});
        assert.equal(result.product.current_amount,0);
        for(const field of ["price.current_amount","price.original_amount","price.discount_rate","rating","review_count"]) assert.ok(result.product.observed_fields.includes(field),field);
        assert.ok(result.coverage.observed_fields.includes("rating.average"));
        assert.ok(result.coverage.observed_fields.includes("rating.count"));
    }
});

test("missing or malformed numbers are not zero, positive amounts or clamped ratings",async()=>{
    for(const price of [undefined,null,"",-1,"-1","12abc","1,2",true,"9007199254740990.9"]){
        const h=reader({products:{"@type":"Product",name:"Synthetic unknown",offers:{price},aggregateRating:{ratingValue:6,reviewCount:1.5}},responses:{reviews:{contents:[{content:"Synthetic review"}]}}});
        const result=await h.run();
        for(const field of ["current_amount","rating","review_count"]) assert.equal(Object.hasOwn(result.product,field),false,field);
        assert.equal(Object.hasOwn(result.reviews[0],"rating"),false);
        assert.equal(Object.hasOwn(result.reviews[0],"helpful_count"),false);
    }
});

test("review zero and rating buckets preserve availability without inventing missing counts",async()=>{
    const h=reader({responses:{reviews:{contents:[{content:"Synthetic review",rating:0,helpfulCount:0}],ratingSummaryTotal:{ratingSummaries:[{rating:5,count:0},{rating:4}]}}}});
    const result=await h.run();
    assert.equal(result.reviews[0].rating,0);assert.equal(result.reviews[0].helpful_count,0);
    assert.deepEqual(result.reviews[0].observed_fields,["rating","helpful_count"]);
    assert.equal(result.rating.distribution?.["4"],undefined);
    assert.equal(result.rating.distribution?.["5"],0);
});
