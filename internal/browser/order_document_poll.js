// Verify and read inside the same Camofox document. Reuse the authentication
// reader used by auth status; never launch another runtime or infer login from
// the presence of orders. Neither private source payloads nor auth material leave.
function pollOrderPage(cursor, key) {
	/* SHARED_READER */
	/* AUTH_READER */
	const authentication = JSON.parse(pollAuthenticationPage(key + '_auth'));
	if (authentication.status !== 'ok' || authentication.authenticated !== true) {
		const previous = window[key];
		if (previous) {
			previous.controller.abort();
			clearTimeout(previous.timer);
			delete window[key];
		}
		return JSON.stringify({status: authentication.status === 'ok' ? 'authentication_data_missing' : authentication.status});
	}
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
