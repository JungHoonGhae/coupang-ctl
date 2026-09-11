// Self-contained for chrome.scripting.executeScript; emits only a small DTO.
export function readSelectedSearchPage(expectedURL) {
	const unavailable = { status: "ordinary_browser_unavailable" };
	let expected, current;
	try { expected = new URL(expectedURL); current = new URL(location.href); } catch { return unavailable; }
	const searchIdentity = url => {
		if (url.origin !== 'https://www.coupang.com' || url.username || url.password || url.hash) return null;
		const category = /^\/np\/categories\/\d{1,24}$/.test(url.pathname);
		if (!category && url.pathname !== '/np/search') return null;
		for (const key of ['q','sorter','page']) if (url.searchParams.getAll(key).length > 1) return null;
		const defaultSort = category ? 'bestAsc' : 'scoreDesc';
		const query = url.searchParams.get('q') ?? '', sorter = url.searchParams.get('sorter') ?? defaultSort, page = url.searchParams.get('page') ?? '1';
		if ((category ? url.searchParams.has('q') : !query.trim() || [...query].length > 200) || ![defaultSort,'saleCountDesc','latestAsc','salePriceAsc','salePriceDesc'].includes(sorter) || !/^[1-9]\d?$|^100$/.test(page)) return null;
		const filters=[];
		for(const key of ['filter','brand','offerCondition','filterType','component','minPrice','maxPrice','rating']) {
			if(url.searchParams.getAll(key).length>1)return null;
			const value=url.searchParams.get(key)||'';
			if(value&&!(key==='rating'&&value==='0'))filters.push([key,value]);
		}
		return JSON.stringify([url.pathname,query,sorter,page,filters]);
	};
	const expectedIdentity = searchIdentity(expected);
	if (!expectedIdentity) return unavailable;
	// Classify first-party login/challenge redirects before requiring /np/search.
	if (!['https://www.coupang.com','https://login.coupang.com'].includes(current.origin) || current.username || current.password) return unavailable;
	const body = document.body?.innerText ?? "";
	if (/captcha|보안문자|자동입력방지/i.test(body)) return { status: 'access_denied' };
	if (current.origin === 'https://login.coupang.com' || document.querySelector('input[type="password"]')) return { status: "authentication_required" };
	if (/access denied|접근.{0,8}(거부|제한)|비정상.{0,8}접근/i.test(document.title + " " + body)) return { status: "access_denied" };
	const currentIdentity = searchIdentity(current);
	if (!currentIdentity) return unavailable;
	if (currentIdentity !== expectedIdentity || document.readyState !== 'complete') return {status:'loading'};
	// Bind native navigation status to this exact document, not an old page or
	// a failed subresource. A denied response is never an empty product result.
	try {
		const navigation = typeof performance !== 'undefined' ? performance.getEntriesByType('navigation') : [];
		if (navigation.some(entry=>entry.name===current.href && entry.responseStatus===403)) return {status:'access_denied'};
	} catch { /* Navigation status is unavailable in some browser versions. */ }
	const clean = value => typeof value === "string" ? value.replace(/\s+/g, " ").trim().slice(0, 400) : "";
	const imageURL = value => {
		const first=Array.isArray(value)?value[0]:value;
		const raw=typeof first==='string'?first:first?.contentUrl??first?.url;
		if(typeof raw!=='string'||raw.length>8192)return '';
		try {const u=new URL(raw,current.origin);return u.protocol==='https:'&&!u.username&&!u.password&&!u.hash&&(!u.port||u.port==='443')&&(u.hostname==='coupangcdn.com'||u.hostname.endsWith('.coupangcdn.com'))?u.href:''}catch{return ''}
	};
	const items = [], seen = new Set();
	const capturedAt = new Date().toISOString();
	const add = (rawURL, rawName, rawPrice, origin, position, rawImage) => {
		let url;
		try { url = new URL(rawURL, current.origin); } catch { return; }
		const id = url.pathname.match(/^\/vp\/products\/(\d{1,24})$/)?.[1];
		if (url.origin !== current.origin || url.username || url.password || !id || !clean(rawName)) return;
		for (const key of ['itemId','vendorItemId']) {
			const ids = url.searchParams.getAll(key);
			if (ids.length > 1 || (ids.length && !/^\d{1,24}$/.test(ids[0]))) return;
		}
		const itemID = /^\d{1,24}$/.test(url.searchParams.get("itemId") ?? "") ? url.searchParams.get("itemId") : "";
		const vendorID = /^\d{1,24}$/.test(url.searchParams.get("vendorItemId") ?? "") ? url.searchParams.get("vendorItemId") : "";
		const key = id + "/" + itemID + "/" + vendorID;
		if (seen.has(key) || items.length >= 60) return;
		seen.add(key);
		const canonical = new URL("/vp/products/" + id, current.origin);
		if (itemID) canonical.searchParams.set("itemId", itemID);
		if (vendorID) canonical.searchParams.set("vendorItemId", vendorID);
		const text = typeof rawPrice === 'string' ? rawPrice.trim().replace(/\s*원$/, '') : '';
		const price = typeof rawPrice === "number" ? rawPrice : /^(?:\d+|\d{1,3}(?:,\d{3})+)$/.test(text) ? Number(text.replace(/,/g,'')) : NaN;
		const validPrice = Number.isSafeInteger(price) && price >= 0 && price <= 1e12;
		let priceScope = 'product';
		if (itemID || vendorID) {
			priceScope = origin.source === 'dom' ? 'selected_option' : 'product';
			if (origin.offerURL) {
				try {
					const offer = new URL(origin.offerURL, current.origin);
					if (offer.origin === url.origin && offer.pathname === url.pathname && !offer.username && !offer.password &&
						offer.searchParams.getAll('itemId').length <= 1 && offer.searchParams.getAll('vendorItemId').length <= 1 &&
						(offer.searchParams.get('itemId') || '') === itemID && (offer.searchParams.get('vendorItemId') || '') === vendorID) priceScope = 'selected_option';
				} catch { /* product-level price is not option proof */ }
			}
		}
		const evidence = (field, locator, method, scope) => ({field,source:origin.source,locator,method,scope,
			provenance:method==='native_field'?'observed':'derived', captured_at:capturedAt,
			reference:{product_id:id,...(scope==='selected_option'?{item_id:itemID,vendor_item_id:vendorID}:{})}});
		const fieldEvidence = [evidence('name', origin.source==='dom'?'dom.search_card.name':'jsonld.Product.name',origin.source==='dom'?'dom_text':'native_field','product')];
		if (validPrice) fieldEvidence.push(evidence('price.current_amount',origin.source==='dom'?'dom.search_card.price':'jsonld.Product.offers.price',origin.source==='dom'||typeof rawPrice==='string'?'numeric_parse':'native_field',priceScope));
		const image=imageURL(rawImage);
		if(image)fieldEvidence.push(evidence('image_url',origin.source==='dom'?'dom.search_card.image':'jsonld.Product.image',origin.source==='dom'?'dom_text':'native_field','product'));
		items.push({product_id:id, ...(itemID ? {item_id:itemID} : {}), ...(vendorID ? {vendor_item_id:vendorID} : {}), name:clean(rawName), url:canonical.href,
			...(image?{image_url:image}:{}), ...(validPrice ? {current_amount:price,currency:origin.currency} : {}), field_evidence:fieldEvidence, observed_fields:["name", "search_position", ...(validPrice ? ["price.current_amount"] : []), ...(image?['image_url']:[])], search_position:position, rank_source:origin.source==='dom'?'dom_search_list_order':'coupang_search_order'});
	};
	// A Product elsewhere in the page is not evidence of membership in this search.
	// Bind the list's URL (or its owning SearchResultsPage URL) to q/sort/page.
	const boundLists = [];
	const listSignatures = new Set();
	const isType = (node, type) => node?.['@type'] === type || (Array.isArray(node?.['@type']) && node['@type'].includes(type));
	const matchesSearch = raw => { try { return typeof raw === 'string' && searchIdentity(new URL(raw, current.origin)) === expectedIdentity; } catch { return false; } };
	const walk = (node, depth = 0, ownerBound = false) => {
		if (!node || depth > 8) return;
		if (Array.isArray(node)) { for (const value of node.slice(0, 100)) walk(value, depth + 1, ownerBound); return; }
		if (typeof node !== "object") return;
		if (isType(node, 'ItemList')) {
			if (matchesSearch(node.url) || (ownerBound && node.url === undefined)) {
				// The live page can emit the same list twice. Deduplicate only an
				// exact structured copy, never conflicting price/order/identity data.
				const signature = JSON.stringify(node);
				if (!listSignatures.has(signature)) { listSignatures.add(signature); boundLists.push(node); }
			}
			return;
		}
		if ((isType(node, 'SearchResultsPage') || isType(node, 'CollectionPage')) && matchesSearch(node.url)) walk(node.mainEntity, depth + 1, true);
		walk(node['@graph'], depth + 1);
	};
	for (const script of document.querySelectorAll('script[type="application/ld+json"]')) {
		if ((script.textContent?.length ?? 0) > 1e6) continue;
		try { walk(JSON.parse(script.textContent)); } catch { /* missing structured data */ }
	}
	if (boundLists.length > 1) return {status:'structured_data_missing'};
	if (boundLists.length === 1) {
		const list = boundLists[0];
		if (!Array.isArray(list.itemListElement)) return {status:'structured_data_missing'};
		if (list.numberOfItems === 0) {
			// Do not turn contradictory cards or an unrelated empty widget into no_results.
			if (list.itemListElement.length || document.querySelectorAll('a[href*="/vp/products/"]').length) return {status:'structured_data_missing'};
			return {status:'ok',search:{items:[],no_results:true}};
		}
		if (!list.itemListElement.length) return {status:'structured_data_missing'};
		for (const [index, entry] of list.itemListElement.slice(0,60).entries()) {
			const product = isType(entry,'Product') ? entry : entry?.item;
			if (!isType(product,'Product')) continue;
			const offer = Array.isArray(product.offers) ? (product.offers.length === 1 ? product.offers[0] : undefined) : product.offers;
			const position = entry.position === undefined ? index+1 : entry.position;
			if (!Number.isSafeInteger(position) || position < 1 || position > 10000) continue;
			add(product.url || offer?.url,product.name,offer?.price,{source:'json_ld',currency:/^[A-Z]{3}$/.test(offer?.priceCurrency??'')?offer.priceCurrency:'',offerURL:offer?.url},position,product.image);
		}
		// A bound but damaged list must not fall back to unrelated page links.
		return items.length ? {status:'ok',search:{items}} : {status:'structured_data_missing'};
	}
	if (!items.length) {
		// Observed search-list boundary; never traverse recommendations or global links.
		// Keep the hashed class suffix out of the contract. A missing/ambiguous root
		// is unsupported data, not an invitation to scan the whole page.
		const roots = document.querySelectorAll('#product-list');
		if (roots.length !== 1) return {status:'structured_data_missing'};
		const cards = [...roots[0].children].filter(e=>e.tagName==='LI' && [...e.classList].some(c=>c.startsWith('ProductUnit_productUnit__')));
		for (const [index,card] of cards.slice(0,60).entries()) {
			const anchor = card.querySelector('a[href*="/vp/products/"]');
			if (!anchor) continue;
			const name = card.querySelector('[class*="productName"], [class*="ProductName"], .name')?.textContent || card.querySelector("img")?.alt || anchor.getAttribute("title");
			const explicitPrice = card.querySelector('.price-value, [class*="Price_price"], [class*="priceValue"]');
			const area = card.querySelector('[class*="PriceArea"], [class*="priceArea"]');
			const leaf = area ? [...area.querySelectorAll("*")].find(e => !e.children.length && e.tagName !== "DEL" && !e.closest("del") && /^\d[\d,]*\s*원$/.test(clean(e.textContent))) : null;
			const rawPrice = explicitPrice?.textContent || leaf?.textContent;
			const img=card.querySelector('img');
			add(anchor.href, name, rawPrice, {source:'dom',currency:/원\s*$/.test(rawPrice??'')?'KRW':''},index+1,img?.currentSrc||img?.getAttribute?.('src')||img?.getAttribute?.('data-src'));
		}
	}
	return items.length ? {status:"ok", search:{items}} : {status:document.readyState === "complete" ? "structured_data_missing" : "loading"};
}
