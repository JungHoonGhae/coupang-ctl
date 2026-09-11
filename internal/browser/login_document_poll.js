function pollSelectedLogin(key, activateQR) {
	/* AUTH_READER */
	const current=new URL(location.href);
	const body=document.body?.innerText??'';
	if(current.username||current.password)return JSON.stringify({status:'unexpected_destination'});
	if(/access denied|접근.{0,8}(거부|제한)|비정상.{0,8}접근|captcha|보안문자|자동입력방지/i.test(document.title+' '+body))return JSON.stringify({status:'access_denied'});
	try{
		if(performance.getEntriesByType('navigation').some(e=>e.name===current.href&&e.responseStatus===403))return JSON.stringify({status:'access_denied'});
	}catch{}
	if(current.href==='https://mc.coupang.com/ssr/desktop/order/list')return pollAuthenticationPage(key+'_auth');
	if(current.origin!=='https://login.coupang.com')return JSON.stringify({status:'loading'});
	if(document.readyState!=='complete')return JSON.stringify({status:'loading'});
	if(!activateQR)return JSON.stringify({status:'authentication_required'});
	if(/시간.{0,8}만료|QR.{0,8}만료/.test(body))return JSON.stringify({status:'qr_expired'});
	// Copy varies across login versions. Only advertise capture readiness when
	// the QR-sized image and confirmation number are actually visible. Decoding
	// and the strict Coupang link allowlist still happen before presentation.
	const visible=e=>{const r=e.getBoundingClientRect(),s=getComputedStyle(e);return r.width>0&&r.height>0&&s.display!=='none'&&s.visibility!=='hidden'&&Number(s.opacity)>0;};
	const qrImage=[...document.querySelectorAll('img,canvas')].some(e=>{const r=e.getBoundingClientRect();return visible(e)&&r.width>=120&&r.height>=120&&Math.abs(r.width-r.height)<10;});
	const confirmation=[...document.querySelectorAll('body *')].some(e=>e.children.length===0&&visible(e)&&/^\d{2}$/.test(e.textContent?.trim()??''));
	if(qrImage&&confirmation)return JSON.stringify({status:'qr_ready'});
	const state=window[key+'_qr']??(window[key+'_qr']={requested:false});
	if(!state.requested){
		const candidates=[...document.querySelectorAll('a,button,[role="tab"],li')];
		const control=candidates.find(e=>e.textContent?.trim()==='QR코드 로그인');
		if(control instanceof HTMLElement){state.requested=true;control.click();}
	}
	return JSON.stringify({status:'login_waiting'});
}
