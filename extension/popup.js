const CONNECT_MESSAGE = Object.freeze({
	schema_version: 1,
	type: "connect_selected_order_tab",
});

export function createPopupConnectionRequest(chromeApi) {
	return async function requestConnection() {
		if (!chromeApi?.runtime?.sendMessage) return { status: "unavailable" };
		try {
			const response = await chromeApi.runtime.sendMessage(CONNECT_MESSAGE);
			if (
				response !== null &&
				typeof response === "object" &&
				!Array.isArray(response) &&
				Object.keys(response).sort().join(",") === "status" &&
				new Set(["started", "unavailable"]).has(response.status)
			) {
				return response;
			}
		} catch {
			// The popup reports only a stable local status, never a raw runtime error.
		}
		return { status: "unavailable" };
	};
}

export function bindPopup(documentObject, chromeApi) {
	const button = documentObject?.getElementById("connect");
	const status = documentObject?.getElementById("status");
	if (!button || !status) return;
	const requestConnection = createPopupConnectionRequest(chromeApi);
	button.addEventListener("click", async () => {
		button.disabled = true;
		status.textContent = "로컬 coupangctl에 연결하는 중…";
		const result = await requestConnection();
		if (result.status === "started") {
			status.textContent = "연결 요청을 시작했습니다. 이 창을 닫아도 됩니다.";
			return;
		}
		status.textContent = "연결하지 못했습니다. CLI 명령과 선택한 주문 탭을 확인해 주세요.";
		button.disabled = false;
	});
}

if (typeof document !== "undefined" && typeof chrome !== "undefined") {
	bindPopup(document, chrome);
}
