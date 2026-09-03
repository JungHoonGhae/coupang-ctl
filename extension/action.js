export const NATIVE_HOST_NAME = "com.coupangctl.browser_bridge";

const SCHEMA_VERSION = 1;
const MAX_FRAME_BYTES = 256 << 10;
const REQUEST_ID = /^[A-Za-z0-9_-]{32,64}$/;
const ALLOWED_STATUS = new Set([
	"ok",
	"access_denied",
	"authentication_required",
	"structured_data_missing",
	"ordinary_browser_unavailable",
]);

export function createOrdinaryBrowserActionHandler(chromeApi, readPage) {
	let active = false;
	return function handleOrdinaryBrowserAction(tab) {
		if (active || !chromeApi?.runtime?.connectNative || !chromeApi?.scripting?.executeScript) {
			return Promise.resolve();
		}
		active = true;
		let port;
		try {
			port = chromeApi.runtime.connectNative(NATIVE_HOST_NAME);
		} catch {
			active = false;
			return Promise.resolve();
		}
		const selectedTab = validSelectedOrderTab(tab);
		let processing = false;
		return new Promise((resolve) => {
			let settled = false;
			const finish = () => {
				if (settled) return;
				settled = true;
				active = false;
				resolve();
			};
			port.onDisconnect.addListener(() => {
				void chromeApi.runtime.lastError;
				finish();
			});
			port.onMessage.addListener((message) => {
				if (processing || !validRequest(message)) {
					port.disconnect();
					return;
				}
				processing = true;
				void respondToRequest(chromeApi, port, selectedTab, readPage, message)
					.catch(() => port.disconnect())
					.finally(() => {
						processing = false;
					});
			});
		});
	};
}

async function respondToRequest(chromeApi, port, selectedTab, readPage, request) {
	let result = { status: "ordinary_browser_unavailable" };
	if (selectedTab) {
		try {
			const executions = await chromeApi.scripting.executeScript({
				target: { tabId: selectedTab.id, frameIds: [0] },
				world: "ISOLATED",
				func: readPage,
				args: [request.cursor ?? null],
			});
			if (executions.length === 1 && executions[0]?.frameId === 0) {
				result = executions[0].result;
			}
		} catch {
			result = { status: "ordinary_browser_unavailable" };
		}
	}
	const response = {
		schema_version: SCHEMA_VERSION,
		request_id: request.request_id,
		status: result?.status,
		...(result?.page === undefined ? {} : { page: result.page }),
	};
	if (!validResponse(response, request.request_id)) {
		throw new Error("invalid ordinary-browser result");
	}
	port.postMessage(response);
}

function validSelectedOrderTab(tab) {
	if (!Number.isInteger(tab?.id) || tab.id < 0 || typeof tab.url !== "string") return null;
	try {
		const selected = new URL(tab.url);
		if (
			selected.protocol !== "https:" ||
			selected.hostname !== "mc.coupang.com" ||
			selected.pathname !== "/ssr/desktop/order/list"
		) {
			return null;
		}
		return { id: tab.id };
	} catch {
		return null;
	}
}

function validRequest(request) {
	if (!plainObject(request)) return false;
	const keys = Object.keys(request).sort().join(",");
	if (keys !== "operation,request_id,schema_version" && keys !== "cursor,operation,request_id,schema_version") {
		return false;
	}
	if (
		request.schema_version !== SCHEMA_VERSION ||
		!REQUEST_ID.test(request.request_id ?? "") ||
		request.operation !== "read_order_document"
	) {
		return false;
	}
	if (request.cursor === undefined) return true;
	return (
		plainObject(request.cursor) &&
		Object.keys(request.cursor).sort().join(",") === "page,year" &&
		Number.isInteger(request.cursor.year) &&
		request.cursor.year >= 2000 &&
		request.cursor.year <= 2100 &&
		Number.isInteger(request.cursor.page) &&
		request.cursor.page >= 0 &&
		request.cursor.page <= 1000
	);
}

function validResponse(response, requestID) {
	if (!plainObject(response) || response.schema_version !== SCHEMA_VERSION || response.request_id !== requestID || !ALLOWED_STATUS.has(response.status)) {
		return false;
	}
	const hasPage = Object.hasOwn(response, "page");
	if (response.status === "ok") {
		if (Object.keys(response).sort().join(",") !== "page,request_id,schema_version,status" || !hasPage || !validPage(response.page)) {
			return false;
		}
	} else if (hasPage || Object.keys(response).sort().join(",") !== "request_id,schema_version,status") {
		return false;
	}
	try {
		return new TextEncoder().encode(JSON.stringify(response)).length <= MAX_FRAME_BYTES;
	} catch {
		return false;
	}
}

function validPage(page) {
	if (!plainObject(page) || !Array.isArray(page.orders) || page.orders.length > 5) return false;
	const keys = Object.keys(page).sort().join(",");
	if (keys !== "orders" && keys !== "next,orders") return false;
	if (page.next !== undefined && !validCursor(page.next)) return false;
	return page.orders.every(validOrder);
}

function validOrder(order) {
	if (!plainObject(order) || !onlyKeys(order, [
		"source_ref", "purchased_at", "purchased_at_time", "total_amount", "discount_amount",
		"shipping_fee", "currency", "fully_canceled", "receipt_available", "items",
	])) return false;
	if (
		!/^[0-9a-f]{64}$/.test(order.source_ref ?? "") ||
		!validDate(order.purchased_at) ||
		!nonnegativeInteger(order.total_amount) ||
		!nonnegativeInteger(order.discount_amount) ||
		!nonnegativeInteger(order.shipping_fee) ||
		order.currency !== "KRW" ||
		!Array.isArray(order.items) || order.items.length > 100
	) return false;
	if (order.purchased_at_time !== undefined) {
		const instant = validInstant(order.purchased_at_time);
		if (!instant || kstDate(instant) !== order.purchased_at) return false;
	}
	if (order.fully_canceled !== undefined && typeof order.fully_canceled !== "boolean") return false;
	if (order.receipt_available !== undefined && typeof order.receipt_available !== "boolean") return false;
	return order.items.every(validItem);
}

function validItem(item) {
	if (!plainObject(item) || !onlyKeys(item, [
		"product_id", "vendor_item_id", "name", "quantity", "cancelled_quantity", "returned_quantity",
		"unit_price", "paid_price", "seller_name", "brand_name", "product_type", "division_type",
		"commerce_kind", "delivery_status", "delivered_at",
	])) return false;
	const cancelled = item.cancelled_quantity ?? 0;
	const returned = item.returned_quantity ?? 0;
	if (
		!validNumericID(item.product_id ?? "") || !validNumericID(item.vendor_item_id ?? "") ||
		!validText(item.name, 2000) || item.name.length === 0 ||
		!positiveInteger(item.quantity) || !nonnegativeInteger(cancelled) || !nonnegativeInteger(returned) ||
		cancelled > item.quantity || returned > item.quantity ||
		!nonnegativeInteger(item.unit_price) || !nonnegativeInteger(item.paid_price) ||
		!validText(item.seller_name ?? "", 1000) || !validText(item.brand_name ?? "", 1000) ||
		!validText(item.product_type ?? "", 200) || !validText(item.division_type ?? "", 200) ||
		!new Set(["product_purchase", "membership_fee"]).has(item.commerce_kind) ||
		!new Set([undefined, "", "delivered", "in_transit", "cancelled", "returned", "other"]).has(item.delivery_status) ||
		(item.delivered_at !== undefined && !validInstant(item.delivered_at))
	) return false;
	return true;
}

function validCursor(cursor) {
	return plainObject(cursor) && Object.keys(cursor).sort().join(",") === "page,year" &&
		Number.isInteger(cursor.year) && cursor.year >= 2000 && cursor.year <= 2100 &&
		Number.isInteger(cursor.page) && cursor.page >= 0 && cursor.page <= 1000;
}

function validDate(value) {
	if (typeof value !== "string" || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
	const parsed = new Date(`${value}T00:00:00Z`);
	return !Number.isNaN(parsed.valueOf()) && parsed.toISOString().slice(0, 10) === value && Number(value.slice(0, 4)) >= 2000 && Number(value.slice(0, 4)) <= 2100;
}

function validInstant(value) {
	if (typeof value !== "string" || !/^\d{4}-\d{2}-\d{2}T.*(?:Z|[+-]\d{2}:?\d{2})$/.test(value)) return null;
	const parsed = new Date(value);
	return !Number.isNaN(parsed.valueOf()) && parsed.getUTCFullYear() >= 2000 && parsed.getUTCFullYear() <= 2100 ? parsed : null;
}

function kstDate(value) {
	return new Date(value.valueOf() + 9 * 60 * 60 * 1000).toISOString().slice(0, 10);
}

function onlyKeys(value, allowed) {
	const allowlist = new Set(allowed);
	return Object.keys(value).every((key) => allowlist.has(key));
}

function validNumericID(value) {
	return value === "" || (typeof value === "string" && /^\d{1,24}$/.test(value));
}

function nonnegativeInteger(value) {
	return Number.isSafeInteger(value) && value >= 0;
}

function positiveInteger(value) {
	return Number.isSafeInteger(value) && value >= 1;
}

function validText(value, maxBytes) {
	return typeof value === "string" && !value.includes("\0") && new TextEncoder().encode(value).length <= maxBytes;
}

function plainObject(value) {
	return value !== null && typeof value === "object" && !Array.isArray(value) && Object.getPrototypeOf(value) === Object.prototype;
}
