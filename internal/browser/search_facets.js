// Reviewed search-sidebar adapter. No evaluation of Next.js script contents.
function readSearchFacets(expectedURL, selection, allowCategoryTransition = false) {
 const clean=x=>typeof x==='string'?x.replace(/\s+/g,' ').trim():'';
 let u,expected;
 try{u=new URL(location.href);expected=new URL(expectedURL);}catch{return {status:'filter_unavailable'};}
 if(u.origin==='https://login.coupang.com')return {status:'authentication_required'};
 if(u.origin!=='https://www.coupang.com'||expected.origin!==u.origin||u.username||u.password||u.hash||expected.username||expected.password||expected.hash)return {status:'filter_unavailable'};
 if(/access denied|captcha|보안문자|자동입력방지/i.test((document.title||'')+' '+(document.body?.innerText||'')))return {status:'access_denied'};
 const category=/^\/np\/categories\/\d{1,24}$/.test(expected.pathname);
 const changedCategory=u.pathname!==expected.pathname;
 if((!category&&expected.pathname!=='/np/search')||
    (changedCategory&&(!allowCategoryTransition||!category||selection?.name!=='카테고리'||!/^\/np\/categories\/\d{1,24}$/.test(u.pathname)))||
    (category?(u.searchParams.has('q')||expected.searchParams.has('q')):!expected.searchParams.get('q')?.trim()))return {status:'filter_unavailable'};
 for(const key of ['q','sorter','page']){
  const fallback=key==='sorter'?(category?'bestAsc':'scoreDesc'):key==='page'?'1':'';
  if(u.searchParams.getAll(key).length>1||expected.searchParams.getAll(key).length>1||
     (u.searchParams.get(key)??fallback)!==(expected.searchParams.get(key)??fallback))return {status:'filter_unavailable'};
 }
 const roots=document.querySelectorAll('.filter-function-bar');
 if(roots.length!==1)return {status:'filter_unavailable'};
 const root=roots[0], allLabels=[...root.querySelectorAll('label')];
 // Decode bounded nested JSON strings to find native attribute definitions.
 const native=[];
 for(const script of document.scripts) {
  const text=script.textContent||'';
  if(text.length>2000000||!text.includes('componentAttributeFilter'))continue;
  const queue=[{text,depth:0}];let visited=0;
  while(queue.length&&visited++<80) {
   const {text:t,depth}=queue.shift();
   const match=/"componentAttributeFilter"\s*:\s*(\[)/.exec(t);
   if(match) {
    const start=match.index+match[0].length-1;let level=0,quoted=false,escape=false;
    for(let i=start;i<t.length;i++) {
     const c=t[i];if(quoted){if(escape)escape=false;else if(c==='\\')escape=true;else if(c==='"')quoted=false;continue;}
     if(c==='"')quoted=true;else if(c==='[')level++;else if(c===']'&&--level===0){try{native.push(...JSON.parse(t.slice(start,i+1)))}catch{}break;}
    }
   }
   if(depth<3)for(const m of t.matchAll(/"(?:\\.|[^"\\])*"/g)) {
    if(!m[0].includes('componentAttributeFilter'))continue;
    try{const s=JSON.parse(m[0]);if(typeof s==='string')queue.push({text:s,depth:depth+1})}catch{}
   }
  }
 }
 const facets=[];let target=null;
 for(const heading of root.querySelectorAll('h5')) {
  const name=clean(heading.textContent), group=heading.parentElement;
  if(!name||name.length>200)continue;
  const definitions=native.filter(f=>f.name===name);
  const definition=definitions[0];
  const options=[];
  for(const label of group.querySelectorAll('.filter-function-bar-list label')) {
   const text=clean(label.textContent);if(!text||text.length>400)continue;
   const disabled=label.classList.contains('disabled')||label.getAttribute('aria-disabled')==='true';
   const selected=label.classList.contains('selected')||label.parentElement.classList.contains('selected')||label.getAttribute('aria-checked')==='true';
   const n=definition?.children?.find(o=>o.name===text);
   options.push({label:text,selected,disabled,...(n?{id:String(n.id)}:{})});
   if(selection&&selection.name===name&&selection.label===text) {
    if(target)return {status:'filter_unavailable'};
    target={index:allLabels.indexOf(label),selected,disabled};
   }
  }
  if(options.length)facets.push({name,options:options.slice(0,100),source:definition?'coupang_structured_filter_and_dom':'coupang_sidebar_dom',...(definition?{id:String(definition.id)}:{}),more_available:!!group.querySelector('.filter-function-bar-list-btn')});
 }
 if(selection&&(!target||target.disabled))return {status:'filter_unavailable',facets};
 if(changedCategory){
  // Only an explicit sidebar click may change scope. A URL change or a label
  // alone is insufficient: bind the selected category to the native breadcrumb.
  if(!target?.selected||document.readyState!=='complete')return {status:'filter_unavailable'};
  let matched=0,conflict=false,visited=0;
  const walk=(node,depth=0)=>{
   if(!node||depth>5||visited++>=300)return;
   if(Array.isArray(node)){for(const n of node.slice(0,50))walk(n,depth+1);return;}
   if(typeof node!=='object')return;
   if(node['@type']==='BreadcrumbList'){
    const raw=node.itemListElement;
    if(!Array.isArray(raw)||!raw.length||raw.length>20||raw.some(r=>Array.isArray(r)&&r.length>20))return;
    // Category pages currently emit [home, [category, ..., leaf]]. Product
    // pages also emit flat lists. Do not recursively flatten arbitrary trees.
    const rows=raw.flat(1);
    if(!rows.length||rows.length>20)return;
    const leaf=rows[rows.length-1];let url;
    try{url=new URL(leaf.item);}catch{return;}
    if(url.origin!==u.origin||url.pathname!==u.pathname||url.username||url.password||url.search||url.hash)return;
    if(rows.some((r,i)=>r?.['@type']!=='ListItem'||r.position!==i+1)||clean(leaf.name)!==selection.label){conflict=true;return;}
    matched++;
   }
   walk(node['@graph'],depth+1);
  };
  const scripts=[...document.querySelectorAll('script[type="application/ld+json"]')];
  if(scripts.length>20)return {status:'filter_unavailable'};
  let bytes=0;
  for(const script of scripts){
   bytes+=script.textContent?.length??0;
   if(bytes>2e6)return {status:'filter_unavailable'};
   try{walk(JSON.parse(script.textContent));}catch{}
  }
  if(!matched||conflict)return {status:'filter_unavailable'};
 }
 return {status:'ok',facets:facets.slice(0,50),target,...(changedCategory?{category_id:u.pathname.split('/').pop()}:{})};
}
