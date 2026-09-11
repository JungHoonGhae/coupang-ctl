// Bounded document polling shared by browser transports; no window ownership here.
function pollProductInspection(request, key) {
	/* SHARED_READER */
	const current = new URL(location.href);
	const body = document.body?.innerText ?? '';
	if (!['https://www.coupang.com','https://login.coupang.com'].includes(current.origin) || current.username || current.password) return JSON.stringify({status:'inspection_failed'});
	if (/captcha|보안문자|자동입력방지/i.test(body)) return JSON.stringify({status:'access_denied'});
	if (current.origin==='https://login.coupang.com' || document.querySelector('input[type="password"]')) return JSON.stringify({status:'authentication_required'});
	if (/access denied|접근.{0,8}(거부|제한)|비정상.{0,8}접근/i.test(document.title+' '+body)) return JSON.stringify({status:'access_denied'});
	if (document.readyState!=='complete' || current.pathname!=='/vp/products/'+request.product_id) return JSON.stringify({status:'loading'});
	for(const [field,param]of [['item_id','itemId'],['vendor_item_id','vendorItemId']]) {
		if(current.searchParams.getAll(param).length>1 || (request[field] && current.searchParams.get(param)!==request[field])) return JSON.stringify({status:'inspection_failed'});
	}
	// Error wording changes; use the browser's status for this exact document
	// when available. Other navigation/resource entries are not its response.
	try {
		const navigation = typeof performance !== 'undefined' ? performance.getEntriesByType('navigation') : [];
		if (navigation.some(entry=>entry.name===current.href && entry.responseStatus===403)) return JSON.stringify({status:'access_denied'});
	} catch { /* Older engines may not expose navigation response status. */ }
	let state=window[key];
	if(!state) {
		state={status:'loading',controller:new AbortController()};
		window[key]=state;
		state.timer=setTimeout(()=>{state.controller.abort();if(window[key]===state){state.status='inspection_failed';delete state.inspection;}},10000);
		readProductInspection(request,{signal:state.controller.signal}).then(document=>{
			if(window[key]!==state || state.controller.signal.aborted)return;
			if(typeof document!=='string' || document.length>128000)throw new Error('invalid_result');
			state.inspection=JSON.parse(document);state.status='ok';clearTimeout(state.timer);
		}).catch(()=>{
			if(window[key]===state){state.status='inspection_failed';clearTimeout(state.timer);}
		});
	}
	return JSON.stringify({status:state.status,...(state.status==='ok'?{inspection:state.inspection}:{})});
}
