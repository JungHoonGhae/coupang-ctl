import { createOrdinaryBrowserActionHandler } from "./action.js";
import { readSelectedOrderPage } from "./page-reader.js";

const handleAction = createOrdinaryBrowserActionHandler(chrome, readSelectedOrderPage);
chrome.action.onClicked.addListener((tab) => {
	void handleAction(tab);
});
