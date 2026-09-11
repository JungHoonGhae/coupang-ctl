import assert from "node:assert/strict";
import test from "node:test";
import { readSelectedSearchPage } from "../internal/browser/search_page_reader.js";

const target = "https://www.coupang.com/np/search?q=synthetic";
const searchList = (...items) => ({"@type": "ItemList", url: target, itemListElement: items});

test('search keeps only the bound product image from the Coupang CDN', () => {
	const previous={document:globalThis.document,location:globalThis.location};
	globalThis.location={href:target};
	const image='https://thumbnail.coupangcdn.com/synthetic.jpg';
	try {
		for(const raw of [image,[image],{'@type':'ImageObject',contentUrl:image}]) {
			const product={'@type':'Product',name:'Synthetic',url:'https://www.coupang.com/vp/products/123',image:raw};
			globalThis.document={title:'Synthetic',body:{innerText:''},readyState:'complete',querySelector:()=>null,querySelectorAll:s=>s.includes('ld+json')?[{textContent:JSON.stringify(searchList(product))}]:[]};
			const item=readSelectedSearchPage(target).search.items[0];
			assert.equal(item.image_url,image);assert.ok(item.observed_fields.includes('image_url'));
			assert.equal(item.field_evidence.find(e=>e.field==='image_url').scope,'product');
		}
		for(const raw of ['https://coupangcdn.com.evil.test/x','http://thumbnail.coupangcdn.com/x','https://u:p@thumbnail.coupangcdn.com/x','data:image/png;base64,x','https://thumbnail.coupangcdn.com:444/x']) {
			globalThis.document.querySelectorAll=s=>s.includes('ld+json')?[{textContent:JSON.stringify(searchList({'@type':'Product',name:'Synthetic',url:'https://www.coupang.com/vp/products/123',image:raw}))}]:[];
			const item=readSelectedSearchPage(target).search.items[0];assert.equal(item.image_url,undefined);assert.equal(item.observed_fields.includes('image_url'),false);
		}
	}finally{globalThis.document=previous.document;globalThis.location=previous.location}
});

test("search price metadata preserves currency, zero and option scope", () => {
	const previous={document:globalThis.document,location:globalThis.location};
	const url='https://www.coupang.com/vp/products/123?itemId=456';
	globalThis.location={href:target};
	try {
		for (const [price,currency,offerURL,scope,provenance] of [[0,'KRW',url,'selected_option','observed'],['1,200','KRW',url,'selected_option','derived'],[12,'USD',undefined,'product','observed'],[1200,undefined,undefined,'product','observed']]) {
			const product={'@type':'Product',url,name:'Synthetic',offers:{price,priceCurrency:currency,url:offerURL}};
			globalThis.document={title:'Synthetic',body:{innerText:''},readyState:'complete',querySelector:()=>null,querySelectorAll:s=>s.includes('ld+json')?[{textContent:JSON.stringify(searchList(product))}]:[]};
			const card=readSelectedSearchPage(target).search.items[0], e=card.field_evidence.find(x=>x.field==='price.current_amount');
			assert.equal(card.current_amount,typeof price==='string'?1200:price);assert.equal(card.currency,currency??'');
			assert.equal(e.scope,scope);assert.equal(e.provenance,provenance);assert.ok(Number.isFinite(Date.parse(e.captured_at)));
			assert.deepEqual(e.reference,scope==='selected_option'?{product_id:'123',item_id:'456',vendor_item_id:''}:{product_id:'123'});
		}
		for (const price of ['',null,undefined,-1,'-100','12abc','1,2',1.5]) {
			globalThis.document.querySelectorAll=s=>s.includes('ld+json')?[{textContent:JSON.stringify(searchList({'@type':'Product',url,name:'Synthetic',offers:{price}}))}]:[];
			const card=readSelectedSearchPage(target).search.items[0];
			assert.equal(Object.hasOwn(card,'current_amount'),false);
			assert.equal(card.field_evidence.some(x=>x.field==='price.current_amount'),false);
		}
	} finally {globalThis.document=previous.document;globalThis.location=previous.location;}
});

test("search DOM parsing is derived and does not guess a currency", () => {
	const previous={document:globalThis.document,location:globalThis.location};
	globalThis.location={href:target};
	try {
		for(const text of ['0원','1,200원','1200']) {
			const card={tagName:'LI',classList:['ProductUnit_productUnit__synthetic'],querySelector:s=>s.startsWith('a[')?anchor:s.includes('productName')?{textContent:'Synthetic DOM'}:s.includes('.price-value')?{textContent:text}:null};
			const anchor={href:'https://www.coupang.com/vp/products/123?itemId=456',closest:()=>card};
			globalThis.document={title:'Synthetic',body:{innerText:''},readyState:'complete',querySelector:()=>null,querySelectorAll:s=>s==='#product-list'?[{children:[card]}]:[]};
			const result=readSelectedSearchPage(target).search.items[0],e=result.field_evidence.find(x=>x.field==='price.current_amount');
			assert.equal(e.source,'dom');assert.equal(e.provenance,'derived');assert.equal(e.scope,'selected_option');
			assert.equal(result.currency,text.endsWith('원')?'KRW':'');
			assert.equal(result.rank_source,'dom_search_list_order');
		}
	} finally {globalThis.document=previous.document;globalThis.location=previous.location;}
});

test("reader emits only structured product fields and rejects error pages", () => {
	const previous = {document:globalThis.document,location:globalThis.location};
	globalThis.location = {href:target};
	globalThis.document = {title:"Synthetic results",body:{innerText:"Synthetic results"},readyState:"complete",querySelector(){return null;},querySelectorAll(selector){return selector.includes("ld+json") ? [{textContent:JSON.stringify(searchList({item:{"@type":"Product",name:"Synthetic bowl",url:"https://www.coupang.com/vp/products/123?itemId=456&tracking=drop",offers:{price:1200,priceCurrency:"KRW"},secret:"must not leave page"}}))}] : [];} };
	try {
		const response=readSelectedSearchPage(target);
		assert.equal(response.status,"ok");
		assert.equal(response.search.items[0].current_amount,1200);
		assert.equal(response.search.items[0].url,"https://www.coupang.com/vp/products/123?itemId=456");
		assert.equal(JSON.stringify(response).includes("must not leave page"),false);
		globalThis.document.title="Access Denied";
		assert.deepEqual(readSelectedSearchPage(target),{status:"access_denied"});
	} finally {globalThis.document=previous.document;globalThis.location=previous.location;}
});
