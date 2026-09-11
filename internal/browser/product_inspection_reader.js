// Shared read-only detail extraction. Transport owns navigation, timeout and window policy.
async function readProductInspection(request, options = {}) {
	const limit = request.document_read_limit;
	if (limit !== undefined && (!Number.isInteger(limit) || limit < 0 || limit > 3)) throw new Error('invalid_document_read_limit');
	// Transport has already navigated to the detail document. Consume the
	// remaining allowance before each explicit endpoint attempt, including failed
	// attempts. Concurrent reads must not both spend the final slot.
	let endpointReadsRemaining = (limit || 3) - 1;
	const budgetOmittedFields = [];
	const checkCancelled = () => { if (options.signal?.aborted) throw new Error('inspection_cancelled'); };
	checkCancelled();
	const initialHref = location.href;
	const current = new URL(initialHref);
	const productMatch = current.pathname.match(/^\/vp\/products\/(\d+)$/);
	if (current.origin !== 'https://www.coupang.com' || current.username || current.password ||
		!productMatch || productMatch[1] !== request.product_id) throw new Error('product_identity_mismatch');
	const productId = productMatch[1];
	const cleanText = (value, limit = 1500) => (typeof value === 'string' || typeof value === 'number' ? String(value) : '')
		.replace(/\s+/g, ' ').trim().slice(0, limit);
	const numeric = (value) => {
		if (typeof value === 'string') {
			const text = value.trim();
			if (!/^(?:\d+|\d{1,3}(?:,\d{3})+)(?:\.\d+)?$/.test(text)) return null;
			value = Number(text.replace(/,/g, ''));
		}
		return typeof value === 'number' && Number.isFinite(value) && value >= 0 && value <= Number.MAX_SAFE_INTEGER ? value : null;
	};
	const boundedNumber = (value, max = Number.MAX_SAFE_INTEGER, integer = false) => {
		if (integer && typeof value === 'string' && /[1-9]/.test(value.trim().split('.')[1] ?? '')) return null;
		const found = numeric(value);
		return found !== null && found <= max && (!integer || Number.isSafeInteger(found)) ? found : null;
	};
	const money = (value, depth = 0) => {
		if (depth > 3 || value == null) return null;
		if (typeof value === 'number') return boundedNumber(value, Number.MAX_SAFE_INTEGER, true);
		if (typeof value === 'string') return boundedNumber(value.trim().replace(/\s*원$/, ''), Number.MAX_SAFE_INTEGER, true);
		if (typeof value !== 'object') return null;
		for (const key of ['amount','value','finalPrice','salePrice','price','priceValue']) {
			if (key in value) { const found = money(value[key], depth + 1); if (found !== null) return found; }
		}
		return null;
	};
	const extraction = (value, raw, source, locator, scope) => ({
		value, source, locator, scope,
		method: source === 'quantity_info' || source === 'review_endpoint' || (raw !== null && typeof raw === 'object') ? 'alias_lookup' : source === 'dom' || typeof raw === 'string' ? 'numeric_parse' : 'native_field',
	});
	const firstAvailable = (...values) => values.find(value => value.value !== null);
	const imageURL = (raw) => {
		if (!raw) return '';
		try {
			const parsed = new URL(raw, location.origin);
			return parsed.protocol === 'https:' && /(^|\.)coupangcdn\.com$/i.test(parsed.hostname) ? parsed.href : '';
		} catch { return ''; }
	};
	const unique = (values, limit) => [...new Set(values.filter(Boolean))].slice(0, limit);
	for (let attempt = 0; attempt < 25; attempt++) {
		checkCancelled();
		const picker = document.querySelector('.option-picker-container, .option-picker-select');
		const selected = [...document.querySelectorAll('.option-picker-select .select-item.selected')];
		if (selected.some((element) => cleanText(element.textContent, 300).length > 0)) break;
		if (!picker && attempt >= 5) break;
		await new Promise((resolve) => setTimeout(resolve, 100));
	}
	const jsonLD = [];
	const structuredSnapshot = [...document.querySelectorAll('script[type="application/ld+json"]')].map((script) => script.textContent || 'null');
	for (const value of structuredSnapshot) {
		try { jsonLD.push(JSON.parse(value)); } catch {}
	}
	const objects = [];
	const walk = (value, depth = 0) => {
		if (depth > 5 || value == null || typeof value !== 'object') return;
		if (Array.isArray(value)) { for (const item of value) walk(item, depth + 1); return; }
		objects.push(value);
		for (const key of ['@graph','itemListElement','item']) if (key in value) walk(value[key], depth + 1);
	};
	for (const value of jsonLD) walk(value);
	const isType = (value, wanted) => Array.isArray(value) ? value.includes(wanted) : value === wanted;
	const productURL = (value) => {
		if (typeof value !== 'string') return null;
		try {
			const url = new URL(value, current.origin);
			return url.origin === current.origin && !url.username && !url.password && url.pathname === current.pathname ? url : null;
		} catch { return null; }
	};
	const products = objects.filter((value) => isType(value['@type'], 'Product'));
	const primaryProducts = products.filter((value) => productURL(value.url));
	if (primaryProducts.length > 1) throw new Error('product_identity_ambiguous');
	const structured = primaryProducts[0] ?? (products.length === 1 && !products[0].url ? products[0] : {});
	const structuredURL = productURL(structured.url);
	// A single displayed offer is independently bound to this product page.
	// Multiple offers are alternatives, not evidence of the selected option.
	const soleOfferURL = !Array.isArray(structured.offers) ? productURL(structured.offers?.url) : null;
	const parameter = (url, name) => {
		const values = url?.searchParams.getAll(name) ?? [];
		if (values.length > 1 || (values.length && !/^\d+$/.test(values[0]))) throw new Error('option_identity_invalid');
		return values[0] ?? '';
	};
	// A requested/navigation query ID is a constraint, not evidence of selection.
	// Independently read the primary Product URL, sole displayed offer URL,
	// and explicit selected form state.
	// Ambiguous states fail closed rather than taking the first related-item ID.
	const selectedID = (field, name, attribute) => {
		const values = [];
		const structuredID = parameter(structuredURL, name);
		if (structuredID) values.push(structuredID);
		const offerID = parameter(soleOfferURL, name);
		if (offerID) values.push(offerID);
		for (const node of document.querySelectorAll('input[name="' + name + '"], .option-picker-select .select-item.selected[' + attribute + ']')) {
			const value = node.value ?? node.getAttribute(attribute) ?? '';
			if (!/^\d+$/.test(value)) throw new Error('option_identity_invalid');
			values.push(String(value));
		}
		const unique = [...new Set(values)];
		if (unique.length > 1) throw new Error('option_identity_ambiguous');
		const observed = unique[0] ?? '';
		const navigated = parameter(current, name);
		if ((observed && navigated && observed !== navigated) ||
			(request[field] && observed !== request[field])) throw new Error('option_identity_mismatch_or_missing');
		return observed;
	};
	const itemId = selectedID('item_id', 'itemId', 'data-item-id');
	const vendorItemId = selectedID('vendor_item_id', 'vendorItemId', 'data-vendor-item-id');
	const matchesOption = (offer) => {
		const url = productURL(offer?.url);
		return url && (itemId || vendorItemId) &&
			(!itemId || parameter(url, 'itemId') === itemId) &&
			(!vendorItemId || parameter(url, 'vendorItemId') === vendorItemId);
	};
	const matchingOffers = Array.isArray(structured.offers) ? structured.offers.filter(matchesOption) : [];
	if (matchingOffers.length > 1) throw new Error('offer_identity_ambiguous');
	const singleOffer = structured.offers ?? {};
	const singleOfferURL = productURL(singleOffer.url);
	const productOnlyOffer = singleOfferURL && !itemId && !vendorItemId &&
		!parameter(singleOfferURL, 'itemId') && !parameter(singleOfferURL, 'vendorItemId');
	const offers = Array.isArray(structured.offers) ? matchingOffers[0] ?? {} :
		(singleOffer.url && !matchesOption(singleOffer) && !productOnlyOffer ? {} : singleOffer);
	const aggregate = structured.aggregateRating ?? {};
	const bodyText = cleanText(document.body?.innerText, 10000);
	const structuredName = cleanText(structured.name, 300);
	const name = structuredName || cleanText(document.querySelector('h1')?.textContent, 300);
	const description = cleanText(structured.description, 5000) || cleanText(document.querySelector('[class*="description"]')?.textContent, 5000);
	const gallery = unique([
		...(Array.isArray(structured.image) ? structured.image : [structured.image]),
		...Array.from(document.querySelectorAll('[class*="gallery"] img, [class*="thumbnail"] img, .prod-image img'))
			.map((node) => node.currentSrc || node.getAttribute('src') || node.getAttribute('data-img-src')),
	].map(imageURL), 50);
	const detailImages = unique(Array.from(document.querySelectorAll(
		'#productDetail img, .product-detail img, [class*="productDetail"] img, [id*="productDetail"] img, [class*="detail-content"] img'
	)).map((node) => imageURL(node.currentSrc || node.getAttribute('src') || node.getAttribute('data-src') || node.getAttribute('data-img-src'))), 50);
	const specifications = unique(Array.from(document.querySelectorAll(
		'#itemBrief li, #itemBrief tr, [class*="attribute"] li, [class*="spec"] tr, .prod-description-attribute'
	)).map((node) => cleanText(node.textContent, 300)).filter((value) => value.length >= 2), 50);
	const selectedOptions = unique(Array.from(document.querySelectorAll(
		'.option-picker-select .select-item.selected'
	)).map((node) => cleanText(node.textContent, 300)).filter((value) => value.length >= 1 && value.length <= 300), 20);

	// Native option rows have one selected attribute each. Their ordered valueId
	// tuple indexes a vendor-item map. Require that independent binding; a title,
	// a request URL, or an arbitrary selected=true elsewhere is insufficient.
	// Compound names/values remain intact: live labels can be misordered or have
	// different arity. Splitting them into RAM/storage would invent semantics.
	const readSelectedAttributes = () => {
		if (!itemId || !vendorItemId) return [];
		const snapshots = [...(document.scripts ?? [])].map(s => s.textContent || '').filter(t => t.includes('optionRows'));
		if (snapshots.length > 100 || snapshots.some(t => t.length > 2e6) || snapshots.reduce((n,t) => n+t.length,0) > 8e6) return [];
		const candidates = [];
		let visited = 0, decodedBytes = 0, candidatesRead = 0, scannedBytes = 0;
		const textValue = v => typeof v === 'string' && v.trim().length > 0 && v.length <= 300 && !/[\u0000-\u001f\u007f]/.test(v);
		const decode = o => {
			const rows = o?.optionRows, map = o?.attributeVendorItemMap;
			if (!Array.isArray(rows) || !rows.length || rows.length > 20 || !map || Array.isArray(map) || typeof map !== 'object') return null;
			const values = [], names = new Set(), ids = [];
			for (const row of rows) {
				const a = row?.selectedAttribute;
				if (!textValue(row?.name) || names.has(row.name.trim()) || !a || a.selected !== true || !textValue(a.name) ||
					typeof a.valueId !== 'string' || !/^\d{1,24}(?:,\d{1,24}){0,19}$/.test(a.valueId) ||
					!Array.isArray(row.attributes) || row.attributes.length > 200) return null;
				const selected = row.attributes.filter(x => x?.selected === true);
				if (selected.length !== 1 || selected[0].valueId !== a.valueId || selected[0].name !== a.name) return null;
				names.add(row.name.trim()); ids.push(a.valueId); values.push({name:row.name,value:a.name});
			}
			const key = ids.join(':');
			if (!Object.hasOwn(map,key)) return null;
			const binding = map[key];
			if (!Number.isSafeInteger(binding?.itemId) || !Number.isSafeInteger(binding?.vendorItemId) ||
				String(binding.itemId) !== itemId || String(binding.vendorItemId) !== vendorItemId) return null;
			return values;
		};
		for (const text of snapshots) {
			const queue = [{text,depth:0}];
			while (queue.length) {
				if (++visited > 80) return [];
				const {text:t,depth} = queue.shift();
				decodedBytes += t.length;
				if (decodedBytes > 16e6) return [];
				for (const match of t.matchAll(/"options"\s*:\s*\{/g)) {
					if (++candidatesRead > 40) return [];
					const start = match.index + match[0].length - 1;
					let level=0, quoted=false, escape=false;
					for (let i=start; i<t.length; i++) {
						if (++scannedBytes > 16e6) return [];
						const c=t[i];
						if (quoted) {if(escape)escape=false;else if(c==='\\')escape=true;else if(c==='"')quoted=false;continue;}
						if(c==='"')quoted=true;
						else if(c==='{'||c==='[')level++;
						else if((c==='}'||c===']')&&--level===0) {
							try {
								const candidate=JSON.parse(t.slice(start,i+1));
								const value=decode(candidate);
								if(candidate?.optionRows && !value)return [];
								if(value)candidates.push(value);
							} catch {}
							break;
						}
					}
					if(candidates.length>20)return [];
				}
				if (depth < 3) for (const m of t.matchAll(/"(?:\\.|[^"\\])*"/g)) {
					if (!m[0].includes('optionRows')) continue;
					try {const value=JSON.parse(m[0]);if(typeof value==='string')queue.push({text:value,depth:depth+1});} catch {}
					if(queue.length>80)return [];
				}
			}
		}
		const signatures = [...new Set(candidates.map(v => JSON.stringify(v)))];
		return signatures.length === 1 ? candidates[0] : [];
	};
	const selectedAttributes = readSelectedAttributes();

	const safeFetch = async (target, field) => {
		try {
			checkCancelled();
			if (endpointReadsRemaining <= 0) { budgetOmittedFields.push(field); return null; }
			endpointReadsRemaining--;
			const response = await fetch(target, { credentials: 'include', redirect: 'error', headers: { accept: 'application/json' }, signal: options.signal });
			if (!response.ok) return null;
			return await response.json();
		} catch { return null; }
	};
	const quantityTarget = /^\d+$/.test(vendorItemId)
		? '/next-api/products/quantity-info?productId=' + encodeURIComponent(productId) + '&vendorItemId=' + encodeURIComponent(vendorItemId) : '';
	const reviewTarget = '/next-api/review?productId=' + encodeURIComponent(productId) +
		'&page=1&size=' + Math.max(1, Math.min(20, request.review_limit || 5)) + '&sortBy=ORDER_SCORE_ASC&ratingSummary=true&ratings=&market=';
	const [quantityPayload, reviewPayload] = await Promise.all([
		quantityTarget ? safeFetch(quantityTarget, 'quantity_info') : Promise.resolve(null), safeFetch(reviewTarget, 'reviews'),
	]);
	checkCancelled();
	const quantity = Array.isArray(quantityPayload) ? quantityPayload[0] :
		(Array.isArray(quantityPayload?.data) ? quantityPayload.data[0] : quantityPayload?.data ?? quantityPayload ?? {});
	const reviewRoot = reviewPayload?.rData ?? reviewPayload?.data ?? reviewPayload ?? {};
	const paging = reviewRoot?.paging ?? {};
	const reviewRows = Array.isArray(paging?.contents) ? paging.contents :
		(Array.isArray(reviewRoot?.contents) ? reviewRoot.contents : Array.isArray(reviewRoot?.reviews) ? reviewRoot.reviews : []);
	const reviews = reviewRows.slice(0, Math.max(1, Math.min(20, request.review_limit || 5))).map((row) => {
		const images = Array.isArray(row?.images) ? row.images : Array.isArray(row?.attachments) ? row.attachments : [];
		const rating = boundedNumber(row?.rating ?? row?.ratingScore ?? row?.reviewRating, 5);
		const helpful = boundedNumber(row?.helpfulCount ?? row?.helpCount, Number.MAX_SAFE_INTEGER, true);
		return {
			...(rating !== null ? {rating} : {}),
			content: cleanText(row?.content ?? row?.reviewContent ?? row?.title, 1500),
			created_date: cleanText(row?.createdAt ?? row?.createdDate ?? row?.date, 40),
			...(helpful !== null ? {helpful_count: helpful} : {}),
			observed_fields: [rating !== null && 'rating', helpful !== null && 'helpful_count'].filter(Boolean),
			image_urls: unique(images.map((image) => imageURL(typeof image === 'string' ? image : image?.imageUrl ?? image?.url)), 10),
		};
	}).filter((review) => review.content || review.rating || review.image_urls.length);
	const ratingRoot = reviewRoot?.ratingSummaryTotal ?? aggregate ?? {};
	const ratingRaw = ratingRoot?.ratingAverage ?? aggregate?.ratingValue;
	const ratingAverage = boundedNumber(ratingRaw, 5);
	const countRaw = reviewRoot?.reviewTotalCount ?? ratingRoot?.ratingCount ?? aggregate?.reviewCount;
	const reviewCount = boundedNumber(countRaw, Number.MAX_SAFE_INTEGER, true);
	const endpointRatingRoot = reviewRoot?.ratingSummaryTotal != null;
	const ratingAlias = ratingRoot?.ratingAverage != null;
	const countAlias = reviewRoot?.reviewTotalCount != null || ratingRoot?.ratingCount != null;
	const ratingEvidence = extraction(ratingAverage, ratingRaw,
		endpointRatingRoot && ratingAlias ? 'review_endpoint' : 'json_ld',
		ratingAlias ? (endpointRatingRoot ? 'reviews.ratingSummaryTotal.ratingAverage' : 'jsonld.Product.aggregateRating.ratingAverage') : 'jsonld.Product.aggregateRating.ratingValue', 'product_page');
	const countEvidence = extraction(reviewCount, countRaw,
		reviewRoot?.reviewTotalCount != null || (endpointRatingRoot && ratingRoot?.ratingCount != null) ? 'review_endpoint' : 'json_ld',
		reviewRoot?.reviewTotalCount != null ? 'reviews.reviewTotalCount' : ratingRoot?.ratingCount != null ? (endpointRatingRoot ? 'reviews.ratingSummaryTotal.ratingCount' : 'jsonld.Product.aggregateRating.ratingCount') : 'jsonld.Product.aggregateRating.reviewCount', 'product_page');
	if (ratingAlias) ratingEvidence.method = 'alias_lookup';
	if (countAlias) countEvidence.method = 'alias_lookup';
	const distribution = {};
	const ratingRows = Array.isArray(ratingRoot?.ratingSummaries) ? ratingRoot.ratingSummaries : [];
	let distributionComplete = Array.isArray(ratingRoot?.ratingSummaries);
	for (const row of ratingRows) {
		const star = boundedNumber(row?.rating ?? row?.score ?? row?.star, 5, true);
		const count = boundedNumber(row?.count ?? row?.ratingCount, Number.MAX_SAFE_INTEGER, true);
		if (star === null || star < 1 || count === null || Object.hasOwn(distribution, String(star))) {
			distributionComplete = false;
			continue;
		}
		distribution[String(star)] = count;
	}
	const benefitNodes = Array.from(document.querySelectorAll('[class*="coupon"], [class*="cardBenefit"], [class*="cashback"], [class*="promotion"]'));
	const benefitTexts = unique(benefitNodes.map((node) => cleanText(node.textContent, 500))
		.filter((value) => value.length >= 2 && value.length <= 500), 20);
	const benefits = benefitTexts.map((text) => ({
		kind: /카드/.test(text) ? 'card' : /쿠폰/.test(text) ? 'coupon' : /캐시|적립/.test(text) ? 'cashback' : 'promotion',
		title: text.slice(0, 200), description: text.length > 200 ? text : '', source: 'product_page',
	}));
	const couponText = cleanText(quantity?.appliedCoupon?.title ?? quantity?.appliedCoupon?.description ?? quantity?.appliedCoupon, 300);
	if (couponText) benefits.push({ kind: 'coupon', title: couponText.slice(0, 200), description: couponText, source: 'quantity_info' });
	const cashBackText = cleanText(quantity?.cashBackSummary?.title ?? quantity?.cashBackSummary?.description ?? quantity?.cashBackSummary, 300);
	if (cashBackText) benefits.push({ kind: 'cashback', title: cashBackText.slice(0, 200), description: cashBackText, source: 'quantity_info' });
	const deliverySummary = cleanText(quantity?.delivery?.text ?? quantity?.delivery?.description ?? quantity?.delivery, 500) ||
		cleanText(document.querySelector('[class*="delivery"]')?.textContent, 500);
	const domPrice = document.querySelector('[class*="price"] strong')?.textContent;
	const domOriginal = document.querySelector('del')?.textContent;
	const offerPath = Array.isArray(structured.offers) ? 'jsonld.Product.offers[' + structured.offers.indexOf(offers) + '].price' : 'jsonld.Product.offers.price';
	const currentPriceEvidence = firstAvailable(
		extraction(money(offers?.price), offers?.price, 'json_ld', offerPath, matchesOption(offers) ? 'selected_option' : 'product'),
		extraction(money(quantity?.price), quantity?.price, 'quantity_info', 'quantity.selected_root.price', 'unknown'),
		extraction(money(quantity?.priceUnit), quantity?.priceUnit, 'quantity_info', 'quantity.selected_root.priceUnit', 'unknown'),
		extraction(money(domPrice), domPrice, 'dom', 'dom.price_strong', 'unknown'),
	);
	const originalPriceEvidence = firstAvailable(
		extraction(money(quantity?.originalPrice), quantity?.originalPrice, 'quantity_info', 'quantity.selected_root.originalPrice', 'unknown'),
		extraction(money(quantity?.priceList), quantity?.priceList, 'quantity_info', 'quantity.selected_root.priceList', 'unknown'),
		extraction(money(domOriginal), domOriginal, 'dom', 'dom.del', 'unknown'),
	);
	const currentAmount = currentPriceEvidence?.value ?? null;
	// A site name is not currency evidence. Keep unverified endpoint currencies unknown.
	const currency = currentPriceEvidence?.source === 'json_ld' && /^[A-Z]{3}$/.test(offers?.priceCurrency ?? '') ? offers.priceCurrency :
		currentPriceEvidence?.source === 'dom' && /원\s*$/.test(domPrice ?? '') ? 'KRW' : '';
	const originalAmount = originalPriceEvidence?.value ?? null;
	const discountRaw = quantity?.discountRate ?? document.querySelector('[class*="discountRate"]')?.textContent;
	const discountRate = boundedNumber(typeof discountRaw === 'string' ? discountRaw.trim().replace(/%$/, '') : discountRaw, 100, true);
	const canonical = new URL('/vp/products/' + productId, location.origin);
	if (/^\d+$/.test(itemId)) canonical.searchParams.set('itemId', itemId);
	if (/^\d+$/.test(vendorItemId)) canonical.searchParams.set('vendorItemId', vendorItemId);
	const observed = ['name'];
	if (currentAmount !== null) observed.push('price.current_amount');
	if (originalAmount !== null) observed.push('price.original_amount');
	if (discountRate !== null) observed.push('price.discount_rate');
	if (ratingAverage !== null) observed.push('rating');
	if (reviewCount !== null) observed.push('review_count');
	const capturedAt = new Date().toISOString();
	const evidence = (field, value) => ({
		field, source: value.source, locator: value.locator, method: value.method, scope: value.scope,
		provenance: value.method === 'native_field' ? 'observed' : value.method === 'alias_lookup' ? 'inferred' : 'derived',
		reference: {product_id: productId, ...(value.scope === 'selected_option' ? {item_id: itemId, vendor_item_id: vendorItemId} : {})},
		captured_at: capturedAt,
	});
	const fieldEvidence = [];
	if (name) fieldEvidence.push(evidence('name', {source: structuredName ? 'json_ld' : 'dom', locator: structuredName ? 'jsonld.Product.name' : 'dom.h1', method: structuredName ? 'native_field' : 'dom_text', scope: 'product'}));
	for (const [field, value] of [['price.current_amount', currentPriceEvidence], ['price.original_amount', originalPriceEvidence], ['rating', ratingEvidence], ['review_count', countEvidence]]) {
		if (value && value.value !== null) fieldEvidence.push(evidence(field, value));
	}
	if (discountRate !== null) fieldEvidence.push(evidence('price.discount_rate', extraction(discountRate, discountRaw, quantity?.discountRate != null ? 'quantity_info' : 'dom', quantity?.discountRate != null ? 'quantity.selected_root.discountRate' : 'dom.discount_rate', 'unknown')));
	const rocket = /로켓|rocket/i.test(deliverySummary || bodyText);
	const freeShipping = /무료\s*배송/.test(deliverySummary || bodyText);
	const warnings = [];
	if (!quantityPayload) warnings.push('current quantity and promotion endpoint was unavailable');
	if (!reviewPayload) warnings.push('review endpoint was unavailable');
	if (!benefits.some((benefit) => benefit.kind === 'card')) warnings.push('no structured card benefit was observed for this item');
	const finalStructured = [...document.querySelectorAll('script[type="application/ld+json"]')].map((script) => script.textContent || 'null');
	if (location.href !== initialHref || finalStructured.length !== structuredSnapshot.length ||
		finalStructured.some((value, index) => value !== structuredSnapshot[index]) ||
		JSON.stringify(readSelectedAttributes()) !== JSON.stringify(selectedAttributes) ||
		selectedID('item_id', 'itemId', 'data-item-id') !== itemId ||
		selectedID('vendor_item_id', 'vendorItemId', 'data-vendor-item-id') !== vendorItemId) throw new Error('product_identity_changed_during_read');
	return JSON.stringify({
		product: {
			product_id: productId, item_id: /^\d+$/.test(itemId) ? itemId : '', vendor_item_id: /^\d+$/.test(vendorItemId) ? vendorItemId : '',
			name, url: canonical.href, image_url: gallery[0] || '',
			...(currentAmount !== null ? {current_amount: currentAmount, currency} : {}),
			...(originalAmount !== null ? {original_amount: originalAmount} : {}),
			...(discountRate !== null ? {discount_rate: discountRate} : {}),
			...(ratingAverage !== null ? {rating: ratingAverage} : {}),
			...(reviewCount !== null ? {review_count: reviewCount} : {}),
			review_scope: ratingAverage !== null || reviewCount !== null ? 'product_page_observed' : '',
			rocket, free_shipping: freeShipping, coupon: benefits.some((benefit) => benefit.kind === 'coupon'),
			sponsored: false, observed_fields: observed, field_evidence: fieldEvidence,
		},
		field_evidence: [
			...(selectedOptions.length ? [evidence('selected_options', {source: 'dom', locator: 'dom.option_picker.selected.text', method: 'selected_option_text', scope: itemId || vendorItemId ? 'selected_option' : 'unknown'})] : []),
			...(selectedAttributes.length ? [evidence('selected_attributes', {source:'product_options',locator:'options.optionRows.selectedAttribute',method:'native_field',scope:'selected_option'})] : []),
		],
		selected_attributes: selectedAttributes,
		selected_options: selectedOptions, description, specifications, gallery_images: gallery, detail_images: detailImages,
		delivery: { summary: deliverySummary, free_shipping: freeShipping, rocket }, benefits,
		rating: {
			...(ratingAverage !== null ? {average: ratingAverage} : {}),
			...(reviewCount !== null ? {count: reviewCount} : {}), distribution,
		},
		reviews,
		coverage: {
			budget_omitted_fields: budgetOmittedFields,
			source: 'coupang_product_document_and_read_endpoints',
			observed_fields: unique(['identity','name', selectedOptions.length && 'selected_options', selectedAttributes.length && 'selected_attributes', description && 'description', gallery.length && 'gallery_images', detailImages.length && 'detail_images',
				currentAmount !== null && 'price', ratingAverage !== null && 'rating.average', reviewCount !== null && 'rating.count', distributionComplete && 'rating.distribution',
				deliverySummary && 'delivery', benefits.length && 'benefits', reviews.length && 'reviews'].filter(Boolean), 30),
			unavailable_fields: unique([!selectedAttributes.length && 'selected_attributes', !quantityPayload && 'quantity_info', !reviewPayload && 'reviews', !benefits.some((benefit) => benefit.kind === 'card') && 'card_benefit',
				currentAmount === null && 'price.current_amount', ratingAverage === null && 'rating.average', reviewCount === null && 'rating.count', !distributionComplete && 'rating.distribution'].filter(Boolean), 20),
		}, warnings,
	});
}
