// A source-native boolean establishes session authentication, not account
// identity or order availability. No order parsing, member-info request or
// cookie inspection is performed, and no private source value is returned.
function pollAuthenticationPage(key) {
  const target = 'https://mc.coupang.com/ssr/desktop/order/list';
  const endpoint = 'https://mc.coupang.com/ssr/api/member/auth';
  const missing = {status: 'authentication_data_missing'};
  const gate = () => {
    let url;
    try { url = new URL(location.href); } catch { return missing; }
    if (url.username || url.password) return missing;
    const text = document.title + ' ' + (document.body?.innerText || '');
    const status = performance.getEntriesByType('navigation').find(n => n.name === url.href)?.responseStatus;
    if ([403, 429].includes(status) || /access denied|captcha|보안문자|자동입력방지|접근.{0,8}(거부|제한)|비정상.{0,8}접근/i.test(text)) return {status: 'access_denied'};
    if (url.origin === 'https://login.coupang.com') return {status: 'authentication_required'};
    if (url.href !== target) return missing;
    if (status === 401 || document.querySelector('input[type="password"]')) return {status: 'authentication_required'};
    if (status >= 400) return missing;
    if (document.readyState !== 'complete') return {status: 'loading'};
    return null;
  };
  const blocked = gate();
  if (blocked) {
    window[key]?.controller?.abort();
    delete window[key];
    return JSON.stringify(blocked);
  }
  let state = window[key];
  if (!state) {
    state = {result: {status: 'loading'}, controller: new AbortController()};
    window[key] = state;
    const publish = result => {
      if (window[key] === state && !state.controller.signal.aborted) state.result = gate() || result;
    };
    const timer = setTimeout(() => {
      state.controller.abort();
      if (window[key] === state) state.result = missing;
    }, 10000);
    (async () => {
      const response = await fetch(endpoint, {
        method: 'GET', credentials: 'include', redirect: 'error', cache: 'no-store',
        signal: state.controller.signal, headers: {accept: 'application/json'},
      });
      try {
        if (response.url !== endpoint || response.redirected) { publish(missing); return; }
        if (response.status === 401) { publish({status: 'authentication_required'}); return; }
        if ([403, 429].includes(response.status)) { publish({status: 'access_denied'}); return; }
        if (!response.ok || !/^application\/json(?:\s*;|$)/i.test(response.headers.get('content-type') || '')) { publish(missing); return; }
        const reader = response.body.getReader();
        const decoder = new TextDecoder('utf-8', {fatal: true});
        let text = '', size = 0;
        try {
          while (true) {
            const {value, done} = await reader.read();
            if (done) break;
            size += value.byteLength;
            if (size > 1024) { publish(missing); return; }
            text += decoder.decode(value, {stream: true});
          }
          text += decoder.decode();
        } finally { await reader.cancel(); }
        const authenticated = JSON.parse(text);
        publish(authenticated === true ? {status: 'ok', authenticated: true} : authenticated === false ? {status: 'authentication_required'} : missing);
      } finally {
        if (response.body && !response.body.locked) await response.body.cancel();
      }
    })().catch(() => publish(missing)).finally(() => clearTimeout(timer));
  }
  return JSON.stringify(state.result);
}
