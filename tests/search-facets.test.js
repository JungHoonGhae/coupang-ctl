import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';
const source=readFileSync(new URL('../internal/browser/search_facets.js',import.meta.url),'utf8');
function read({name='Memory',label='32 GB',selected=false,disabled=false,selection=null,query='desktop',expected='https://www.coupang.com/np/search?q='+query,href='https://www.coupang.com/np/search?q=desktop',duplicate=false,breadcrumbs=[],transition=false,readyState='complete'}={}){
 const classes={contains:c=>c==='selected'?selected:c==='disabled'?disabled:false};
 const option={textContent:label,classList:classes,getAttribute:()=>null,parentElement:{classList:{contains:()=>false}}};
 const group={querySelectorAll:()=>duplicate?[option,option]:[option],querySelector:()=>null};
 const heading={textContent:name,parentElement:group};
 const root={querySelectorAll:s=>s==='label'?[option]:[heading]};
 const native={componentAttributeFilter:[{id:'1-attr_1',name,children:[{id:'2',name:label}]}]};
 const text='self.__next_f.push('+JSON.stringify([1,JSON.stringify(native)])+')';
 const context=vm.createContext({URL,location:{href},document:{readyState,querySelectorAll:s=>s.includes('ld+json')?breadcrumbs.map(x=>({textContent:JSON.stringify(x)})):[root],scripts:[{textContent:text}]}});
 vm.runInContext(source,context);
 return JSON.parse(JSON.stringify(context.readSearchFacets(expected,selection,transition)));
}
test('native ids and observed UI selection survive discovery',()=>{
 const r=read({selected:true});assert.equal(r.status,'ok');assert.equal(r.facets[0].id,'1-attr_1');assert.equal(r.facets[0].options[0].id,'2');assert.equal(r.facets[0].options[0].selected,true);
});

const categoryLeaf={'@type':'ListItem',position:2,name:'Synthetic child',item:'https://www.coupang.com/np/categories/654321'};
const categoryHome={'@type':'ListItem',position:1,name:'Synthetic parent',item:'https://www.coupang.com/np/categories/123456'};
const categoryBase={name:'카테고리',label:'Synthetic child',selected:true,selection:{name:'카테고리',label:'Synthetic child'},expected:'https://www.coupang.com/np/categories/123456',href:'https://www.coupang.com/np/categories/654321?sorter=bestAsc',transition:true};
test('explicit category clicks accept the observed nested breadcrumb shape',()=>{
 const breadcrumb={'@type':'BreadcrumbList',itemListElement:[categoryHome,[categoryLeaf]]};
 const result=read({...categoryBase,breadcrumbs:[breadcrumb]});
 assert.equal(result.status,'ok');assert.equal(result.category_id,'654321');
 assert.equal(read({...categoryBase,breadcrumbs:[{...breadcrumb,itemListElement:[categoryHome,categoryLeaf]}]}).status,'ok');
});
test('category transitions require selected unique choice, matching breadcrumb and stable sort',()=>{
 const breadcrumb={'@type':'BreadcrumbList',itemListElement:[categoryHome,categoryLeaf]};
 for(const change of [{transition:false},{selected:false},{disabled:true},{duplicate:true},{readyState:'loading'},{breadcrumbs:[]},
  {breadcrumbs:[{...breadcrumb,itemListElement:[categoryHome,{...categoryLeaf,name:'Other'}]}]},
  {breadcrumbs:[{...breadcrumb,itemListElement:[categoryHome,{...categoryLeaf,item:'https://example.com/np/categories/654321'}]}]},
  {breadcrumbs:[{...breadcrumb,itemListElement:[categoryHome,{...categoryLeaf,position:1}]}]},
  {breadcrumbs:[breadcrumb,{...breadcrumb,itemListElement:[categoryHome,{...categoryLeaf,name:'Conflicting'}]}]},
  {href:categoryBase.href+'&page=2'}, {href:categoryBase.href+'&sorter=bestAsc'},
  {href:categoryBase.href.replace('bestAsc','salePriceAsc')}, {href:categoryBase.href+'&q=synthetic'},
  {href:categoryBase.href.replace('www.coupang.com','elsewhere.test')},
  {expected:'https://www.coupang.com/np/search?q=synthetic'},
 ])assert.equal(read({...categoryBase,breadcrumbs:[breadcrumb],...change}).status,'filter_unavailable');
});
test('different product groups do not require a hardcoded taxonomy',()=>{
 assert.equal(read({name:'Dishwasher capacity',label:'6 settings'}).facets[0].name,'Dishwasher capacity');
});
test('unknown, disabled and ambiguous choices fail closed',()=>{
 assert.equal(read({selection:{name:'CPU',label:'invented'}}).status,'filter_unavailable');
 assert.equal(read({disabled:true,selection:{name:'Memory',label:'32 GB'}}).status,'filter_unavailable');
 assert.equal(read({duplicate:true,selection:{name:'Memory',label:'32 GB'}}).status,'filter_unavailable');
});
test('wrong query or host cannot supply filters',()=>{
 assert.equal(read({query:'other'}).status,'filter_unavailable');
 assert.equal(read({href:'https://example.com/np/search?q=desktop'}).status,'filter_unavailable');
});

test('category sidebars must match the requested category, sort and page',()=>{
 const category='https://www.coupang.com/np/categories/123456';
 assert.equal(read({expected:category,href:category+'?sorter=bestAsc&filter=synthetic'}).status,'ok');
 const expected='https://www.coupang.com/np/categories/123456?sorter=salePriceAsc';
 const href=expected+'&filter=synthetic';
 const r=read({expected,href,selected:true});
 assert.equal(r.status,'ok');assert.equal(r.facets[0].options[0].selected,true);
 for(const wrong of [href.replace('123456','654321'),href.replace('salePriceAsc','saleCountDesc'),href+'&page=2',href+'&q=desktop',href+'&sorter=salePriceAsc']) {
  assert.equal(read({expected,href:wrong}).status,'filter_unavailable');
 }
});
