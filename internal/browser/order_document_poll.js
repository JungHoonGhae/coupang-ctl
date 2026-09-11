// Apple Events returns synchronously. Poll one bounded, cancellable read of the
// selected protected page; never return the source payload or account details.
function pollOrderPage(cursor, key) {
	/* SHARED_READER */
	const current = new URL(location.href);
	const body = document.body?.innerText ?? '';
	if (current.username || current.password) return JSON.stringify({status:'order_data_missing'});
	if (/captcha|보안문자|자동입력방지/i.test(body)) return JSON.stringify({status:'access_denied'});
	if (current.origin === 'https://login.coupang.com') return JSON.stringify({status:'authentication_required'});
	if (current.href !== 'https://mc.coupang.com/ssr/desktop/order/list') return JSON.stringify({status:'loading'});
	if (/access denied|접근.{0,8}(거부|제한)|비정상.{0,8}접근/i.test(document.title+' '+body)) return JSON.stringify({status:'access_denied'});
	try {
		for (const entry of performance.getEntriesByType('navigation')) {
			if (entry.name !== current.href) continue;
			if (entry.responseStatus === 403) return JSON.stringify({status:'access_denied'});
			if (entry.responseStatus === 401) return JSON.stringify({status:'authentication_required'});
		}
	} catch { /* Response status is not available in all Chrome versions. */ }
	if (document.querySelector('input[type="password"]')) return JSON.stringify({status:'authentication_required'});
	if (document.readyState !== 'complete') return JSON.stringify({status:'loading'});
	let state = window[key];
	if (!state) {
		state = {status:'loading', controller:new AbortController()};
		window[key] = state;
		state.timer = setTimeout(()=>{
			state.controller.abort();
			if (window[key] === state) {state.status='order_data_missing';delete state.page;}
		}, 10000);
		readSelectedOrderPage(cursor, {signal:state.controller.signal}).then(result=>{
			if (window[key] !== state || state.controller.signal.aborted) return;
			clearTimeout(state.timer);
			if (result?.status === 'ok' && result.page && Array.isArray(result.page.orders)) {
				if (JSON.stringify(result).length > 128000) {state.status='order_data_missing';return;}
				state.page=result.page;state.status='ok';
			} else {
				state.status=['access_denied','authentication_required','order_partial'].includes(result?.status) ? result.status : 'order_data_missing';
			}
		}).catch(()=>{
			if (window[key] === state) {state.status='order_data_missing';clearTimeout(state.timer);}
		});
	}
	return JSON.stringify({status:state.status,...(state.status==='ok'?{page:state.page}:{})});
}
