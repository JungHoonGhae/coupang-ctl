import { createOrdinaryBrowserActionHandler } from "./action.js";
import { readSelectedOrderPage } from "./page-reader.js";

const CONNECT_MESSAGE_KEYS = "schema_version,type";

export function createServiceWorkerMessageHandler(chromeApi, readPage) {
	const handleAction = createOrdinaryBrowserActionHandler(chromeApi, readPage);
	return function handlePopupMessage(message, _sender, sendResponse) {
		if (
			message === null ||
			typeof message !== "object" ||
			Array.isArray(message) ||
			Object.keys(message).sort().join(",") !== CONNECT_MESSAGE_KEYS ||
			message.schema_version !== 1 ||
			message.type !== "connect_selected_order_tab"
		) {
			return false;
		}
		void (async () => {
			try {
				const tabs = await chromeApi.tabs.query({ active: true, lastFocusedWindow: true });
				if (!Array.isArray(tabs) || tabs.length !== 1) {
					sendResponse({ status: "unavailable" });
					return;
				}
				void handleAction(tabs[0]);
				sendResponse({ status: "started" });
			} catch {
				sendResponse({ status: "unavailable" });
			}
		})();
		return true;
	};
}

if (typeof chrome !== "undefined") {
	chrome.runtime.onMessage.addListener(createServiceWorkerMessageHandler(chrome, readSelectedOrderPage));
}
