import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';

const source=readFileSync(new URL('../internal/browser/search_page_reader.js',import.meta.url),'utf8').replace('export function','function');
const target='https://www.coupang.com/np/search?q=synthetic';
const product=id=>({'@type':'Product',url:`https://www.coupang.com/vp/products/${id}`,name:'Synthetic',offers:{price:0,priceCurrency:'KRW'}});
const list=(items,extra={})=>({'@type':'ItemList',url:target,itemListElement:items,...extra});
function read({scripts=[],href=target,expected=target,ready='complete',body='',title='Synthetic',password=false,anchors=[],roots=[],navigation=[]}={}) {
 const document={title,body:{innerText:body},readyState:ready,querySelector:s=>s.includes('password')&&password?{}:null,querySelectorAll:s=>s.includes('ld+json')?scripts.map(x=>({textContent:JSON.stringify(x)})):s==='#product-list'?roots:anchors};
 const context=vm.createContext({URL,location:{href},document,performance:{getEntriesByType:type=>type==='navigation'?navigation:[]}});
 vm.runInContext(source,context);
 return JSON.parse(JSON.stringify(context.readSelectedSearchPage(expected)));
}

test('unfiltered structured lists cannot masquerade as filtered results',()=>{
 const filtered=target+'&filter=synthetic-filter';
 assert.equal(read({scripts:[list([product(111)])],href:filtered,expected:filtered}).status,'structured_data_missing');
 assert.equal(read({scripts:[list([product(111)],{url:filtered})],href:filtered,expected:filtered}).status,'ok');
});

test('structured products need a request-bound search list, not mere page presence',()=>{
 for(const scripts of [[product(111)],[list([product(111)],{url:undefined})],[list([product(111)],{url:target.replace('synthetic','other')})],[list([product(111)],{url:target+'&sorter=salePriceAsc'})],[list([product(111)],{url:target+'&page=2'})]]) {
  assert.equal(read({scripts}).status,'structured_data_missing');
 }
 const result=read({scripts:[product(999),list([product(111)])]});
 assert.deepEqual(result.search.items.map(x=>x.product_id),['111']);
});

test('navigation 403 cannot become missing data or explicit no-results',()=>{
 for(const scripts of [[],[list([],{numberOfItems:0})],[list([product(111)])]]) {
  assert.equal(read({scripts,navigation:[{name:target,responseStatus:403}]}).status,'access_denied');
 }
 for(const entry of [{name:target+'&page=2',responseStatus:403},{name:target,responseStatus:0},{name:target,responseStatus:200}]) {
  assert.equal(read({scripts:[list([product(111)])],navigation:[entry]}).status,'ok');
 }
});

test('SearchResultsPage binds only its mainEntity and never overrides a conflicting list URL',()=>{
 const page={'@type':'SearchResultsPage',url:target,mainEntity:{'@type':'ItemList',itemListElement:[product(111)]}};
 assert.equal(read({scripts:[{'@graph':[page,product(999)]}]}).search.items[0].product_id,'111');
 page.mainEntity.url=target+'&page=2';
 assert.equal(read({scripts:[page]}).status,'structured_data_missing');
});

test('observed CollectionPage shape binds the primary list, but never another sort or page',()=>{
 const page={'@type':'CollectionPage',url:target,mainEntity:{'@type':'ItemList',itemListElement:[{position:1,item:product(111)}]}};
 assert.equal(read({scripts:[page]}).search.items[0].product_id,'111');
 for(const href of [target+'&page=2',target+'&sorter=salePriceAsc']) {
  assert.equal(read({scripts:[page],href,expected:href}).status,'structured_data_missing');
 }
});

test('exact duplicated structured lists are accepted but conflicting copies are not merged',()=>{
 const first=list([product(111)]), copy=JSON.parse(JSON.stringify(first));
 assert.equal(read({scripts:[first,copy]}).search.items.length,1);
 copy.itemListElement[0].offers.price=100;
 assert.equal(read({scripts:[first,copy]}).status,'structured_data_missing');
});

test('DOM fallback requires one primary list and reads each direct card once',()=>{
 const card=id=>({tagName:'LI',classList:['ProductUnit_productUnit__synthetic'],querySelector:s=>s.startsWith('a[')?{href:`https://www.coupang.com/vp/products/${id}`} : s.includes('productName')?{textContent:'Synthetic'}:null});
 const roots=[{children:[card(111),{tagName:'DIV',classList:[]},card(222)]}];
 const anchors=[{href:'https://www.coupang.com/vp/products/999'}];
 assert.equal(read({anchors}).status,'structured_data_missing');
 assert.equal(read({roots:[...roots,...roots],anchors}).status,'structured_data_missing');
 const result=read({roots,anchors});
 assert.deepEqual(result.search.items.map(x=>[x.product_id,x.search_position,x.rank_source]),[['111',1,'dom_search_list_order'],['222',2,'dom_search_list_order']]);
});

test('empty/loading/malformed/ambiguous lists are not explicit no-results',()=>{
 assert.equal(read().status,'structured_data_missing');
 assert.equal(read({scripts:[list([])]}).status,'structured_data_missing');
 assert.equal(read({scripts:[list([],{numberOfItems:'0'})]}).status,'structured_data_missing');
 assert.equal(read({scripts:[list([],{numberOfItems:0})],ready:'interactive'}).status,'loading');
 assert.deepEqual(read({scripts:[list([],{numberOfItems:0})]}),{status:'ok',search:{items:[],no_results:true}});
 for(const scripts of [[list([product(111)],{numberOfItems:0})],[list([],{numberOfItems:0}),list([product(111)])],[list([{}])],[list([product(111)]),list([product(222)])]]) {
  assert.equal(read({scripts}).status,'structured_data_missing');
 }
 assert.equal(read({scripts:[list([],{numberOfItems:0})],anchors:[{}]}).status,'structured_data_missing');
});

test('bound list preserves positions after invalid and duplicate entries',()=>{
 const result=read({scripts:[list([{item:product(111),position:61},{item:product(222),position:-1},{item:product(111),position:63},{item:product(333),position:64}])]});
 assert.deepEqual(result.search.items.map(x=>[x.product_id,x.search_position]),[['111',61],['333',64]]);
 assert.equal(read({scripts:[list([null,product(333)])]}).search.items[0].search_position,2);
});

test('unsafe or ambiguous request identities cannot return products',()=>{
 for(const url of [target+'&q=other',target+'&q=synthetic',target+'&sorter=scoreDesc&sorter=saleCountDesc',target+'&page=01',target+'&page=0',target+'&page=101',target+'&sorter=',target+'#fragment',target.replace('www.coupang.com','user:pass@www.coupang.com'),target.replace('www.coupang.com','www.coupang.com.evil.test')]) {
  assert.equal(read({scripts:[list([product(111)])],expected:url}).status,'ordinary_browser_unavailable',url);
  assert.equal(read({scripts:[list([product(111)])],href:url}).status,'ordinary_browser_unavailable',url);
 }
 assert.equal(read({href:target+'&page=2',scripts:[list([product(111)])]}).status,'loading');
 assert.equal(read({href:target+'&sorter=scoreDesc&page=1&channel=recent',scripts:[list([product(111)])]}).status,'ok');
});

test('first-party login and challenge redirects have explicit outcomes',()=>{
 assert.equal(read({href:'https://login.coupang.com/login/login.pang'}).status,'authentication_required');
 assert.equal(read({href:'https://www.coupang.com/challenge',body:'CAPTCHA'}).status,'access_denied');
 assert.equal(read({href:'https://login.coupang.com/login/login.pang',body:'CAPTCHA'}).status,'access_denied');
 assert.equal(read({password:true,title:'Access denied'}).status,'authentication_required');
 assert.equal(read({href:'https://unrelated.test/login',password:true}).status,'ordinary_browser_unavailable');
});

test('category lists bind category identity, sort, page and filters without a query',()=>{
 const category='https://www.coupang.com/np/categories/123456';
 const filtered=category+'?sorter=salePriceAsc&page=2&filter=synthetic';
 const page=url=>({'@type':'CollectionPage',url,mainEntity:{'@type':'ItemList',itemListElement:[product(111)]}});
 for(const url of [category,category+'?sorter=bestAsc',filtered]) {
  const r=read({href:url,expected:url,scripts:[page(url)]});
  assert.equal(r.status,'ok');assert.equal(r.search.items[0].product_id,'111');
 }
 assert.equal(read({href:category+'?sorter=bestAsc',expected:category,scripts:[page(category)]}).status,'ok');
 for(const wrong of [category.replace('123456','654321'),category+'?page=2',category+'?sorter=saleCountDesc',target]) {
  assert.equal(read({href:category,expected:category,scripts:[page(wrong)]}).status,'structured_data_missing');
 }
 assert.equal(read({href:category.replace('123456','654321'),expected:category,scripts:[page(category)]}).status,'loading');
 for(const wrong of [category+'?q=desktop',category+'?q=',category+'/',category.replace('123456','abc'),category.replace('123456','1'.repeat(25))]) {
  assert.equal(read({href:wrong,expected:wrong,scripts:[page(wrong)]}).status,'ordinary_browser_unavailable');
 }
 assert.equal(read({href:'https://login.coupang.com/login/login.pang',expected:category}).status,'authentication_required');
 assert.equal(read({href:category,expected:category,title:'Access Denied'}).status,'access_denied');
});
