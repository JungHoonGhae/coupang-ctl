import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import test from "node:test";

import {
	NATIVE_HOST_NAME,
	createOrdinaryBrowserActionHandler,
} from "../extension/action.js";
import { readSelectedOrderPage } from "../extension/page-reader.js";
import { createPopupConnectionRequest } from "../extension/popup.js";
import { createServiceWorkerMessageHandler } from "../extension/service-worker.js";

test("MV3 manifest has only the selected-tab bridge permissions", async () => {
	const manifest = JSON.parse(
		await readFile(new URL("../extension/manifest.json", import.meta.url), "utf8"),
	);
	assert.equal(manifest.manifest_version, 3);
	assert.deepEqual(
		[...manifest.permissions].sort(),
		["activeTab", "nativeMessaging", "scripting"],
	);
	assert.equal(manifest.host_permissions, undefined);
	assert.equal(manifest.externally_connectable, undefined);
	assert.equal(manifest.web_accessible_resources, undefined);
	assert.equal(manifest.incognito, "not_allowed");
	assert.deepEqual(manifest.background, {
		service_worker: "service-worker.js",
		type: "module",
	});
	assert.deepEqual(manifest.icons, {
		16: "icon16.png",
		48: "icon48.png",
		128: "icon128.png",
	});
	assert.deepEqual(manifest.action, {
		default_title: "이 쿠팡 주문 탭을 coupangctl에 연결",
		default_popup: "popup.html",
		default_icon: {
			16: "icon16.png",
			48: "icon48.png",
			128: "icon128.png",
		},
	});
	assert.equal(
		manifest.content_security_policy.extension_pages,
		"script-src 'self'; object-src 'none'",
	);
	const digest = createHash("sha256").update(Buffer.from(manifest.key, "base64")).digest().subarray(0, 16);
	const extensionID = [...digest]
		.flatMap((byte) => [byte >> 4, byte & 15])
		.map((nibble) => String.fromCharCode("a".charCodeAt(0) + nibble))
		.join("");
	assert.equal(extensionID, "kdpkegejlalobnlbgpjjibllolajjonf");
});

test("extension icons are exact RGBA PNG sizes required by the manifest", async () => {
	for (const size of [16, 48, 128]) {
		const image = await readFile(new URL(`../extension/icon${size}.png`, import.meta.url));
		assert.deepEqual([...image.subarray(0, 8)], [137, 80, 78, 71, 13, 10, 26, 10]);
		assert.equal(image.readUInt32BE(16), size);
		assert.equal(image.readUInt32BE(20), size);
		assert.equal(image[24], 8, `icon${size}.png must use 8-bit channels`);
		assert.equal(image[25], 6, `icon${size}.png must be RGBA`);
	}
});

test("popup discloses the exact local data flow before its affirmative button", async () => {
	const html = await readFile(new URL("../extension/popup.html", import.meta.url), "utf8");
	for (const disclosure of ["현재 선택한 쿠팡 주문목록 탭", "주문일·금액·상품·배송 상태", "내 컴퓨터의 coupangctl", "쿠키와 로그인 정보", "외부 서버로 보내지 않습니다"]) {
		assert.match(html, new RegExp(disclosure));
	}
	assert.match(html, /<button[^>]+id="connect"[^>]*>\s*이 탭 연결\s*<\/button>/);
	assert.match(html, /<script[^>]+src="popup\.js"[^>]*><\/script>/);
	assert.doesNotMatch(html, /<script(?![^>]+src=)[^>]*>/);

	const messages = [];
	const requestConnection = createPopupConnectionRequest({
		runtime: {
			async sendMessage(message) {
				messages.push(message);
				return { status: "started" };
			},
		},
	});
	assert.deepEqual(await requestConnection(), { status: "started" });
	assert.deepEqual(messages, [{ schema_version: 1, type: "connect_selected_order_tab" }]);
});

test("service worker starts the native bridge only for the closed popup message", async () => {
	const port = fakeNativePort();
	let queryCount = 0;
	const chromeApi = {
		runtime: {
			connectNative(name) {
				assert.equal(name, NATIVE_HOST_NAME);
				return port;
			},
			lastError: undefined,
		},
		tabs: {
			async query(options) {
				queryCount += 1;
				assert.deepEqual(options, { active: true, lastFocusedWindow: true });
				return [{ id: 17, url: "https://mc.coupang.com/ssr/desktop/order/list" }];
			},
		},
		scripting: { async executeScript() { return []; } },
	};
	const handler = createServiceWorkerMessageHandler(chromeApi, function syntheticReader() {});
	assert.equal(handler({ schema_version: 1, type: "ignored" }, {}, () => {}), false);
	let response;
	assert.equal(handler({ schema_version: 1, type: "connect_selected_order_tab" }, {}, (value) => { response = value; }), true);
	await eventLoopTurn();
	assert.equal(queryCount, 1);
	assert.deepEqual(response, { status: "started" });
});

test("one extension action relays validated requests through the selected top frame", async () => {
	const port = fakeNativePort();
	const executions = [];
	const chromeApi = {
		runtime: {
			connectNative(name) {
				assert.equal(name, NATIVE_HOST_NAME);
				return port;
			},
			lastError: undefined,
		},
		scripting: {
			async executeScript(options) {
				executions.push(options);
				return [{ frameId: 0, result: { status: "ok", page: { orders: [] } } }];
			},
		},
	};
	function syntheticReader() {}
	const handleAction = createOrdinaryBrowserActionHandler(chromeApi, syntheticReader);
	const completed = handleAction({
		id: 17,
		url: "https://mc.coupang.com/ssr/desktop/order/list",
	});

	port.emitMessage({
		schema_version: 1,
		request_id: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		operation: "read_order_document",
		cursor: { year: 2025, page: 2 },
	});
	await eventLoopTurn();

	assert.equal(executions.length, 1);
	assert.deepEqual(executions[0], {
		target: { tabId: 17, frameIds: [0] },
		world: "ISOLATED",
		func: syntheticReader,
		args: [{ year: 2025, page: 2 }],
	});
	assert.deepEqual(port.messages, [
		{
			schema_version: 1,
			request_id: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			status: "ok",
			page: { orders: [] },
		},
	]);

	port.emitDisconnect();
	await completed;
});

test("selected-tab reader preserves an unsafe numeric source id before hashing", async () => {
	const previousLocation = globalThis.location;
	const previousDocument = globalThis.document;
	const previousJSONParse = JSON.parse;
	globalThis.location = {
		origin: "https://mc.coupang.com",
		pathname: "/ssr/desktop/order/list",
	};
	globalThis.document = {
		querySelector(selector) {
			assert.equal(selector, "script#__NEXT_DATA__");
			return {
				textContent: `{
					"props":{"pageProps":{"domains":{"desktopOrder":{
						"orderList":[{
							"orderId":9223372036854775807,
							"orderDate":"2026-09-03T01:02:03+09:00",
							"totalPrice":12345,
							"discountAmount":1000,
							"shippingFee":0,
							"paymentReceiptInfo":{"paymentReceiptVisible":true},
							"deliveryGroupList":[{
								"deliveryStatus":"배송완료",
								"deliveredAt":"2026-09-04T04:05:06+09:00",
								"vendor":{"vendorName":"합성 판매자"},
								"orderItems":[{
									"productId":9007199254740993,
									"vendorItemId":"123456789",
									"productName":"합성 상품",
									"quantity":2,
									"unitPrice":7000,
									"paidPrice":12345,
									"productType":"GENERAL"
								}]
							}]
						}],
						"orderPagination":{"hasNext":true,"nextYear":2025,"nextPageIndex":3}
					}}}}
				}`,
			};
		},
	};
	JSON.parse = (text, reviver) => {
		if (typeof reviver !== "function") return previousJSONParse(text);
		return previousJSONParse(text, (key, value) => reviver(key, value));
	};
	try {
		const result = await readSelectedOrderPage(null);
		assert.deepEqual(result, {
			status: "ok",
			page: {
				orders: [
					{
						source_ref: "fbae1c5166e8c0592b43e28ac679f94073e23d0cd7b9ced65b497398daa3da5e",
						purchased_at: "2026-09-03",
						purchased_at_time: "2026-09-02T16:02:03.000Z",
						total_amount: 12345,
						discount_amount: 1000,
						shipping_fee: 0,
						currency: "KRW",
						receipt_available: true,
						items: [
							{
								product_id: "9007199254740993",
								vendor_item_id: "123456789",
								name: "합성 상품",
								quantity: 2,
								unit_price: 7000,
								paid_price: 12345,
								seller_name: "합성 판매자",
								product_type: "GENERAL",
								commerce_kind: "product_purchase",
								delivery_status: "delivered",
								delivered_at: "2026-09-03T19:05:06.000Z",
							},
						],
					},
				],
				next: { year: 2025, page: 3 },
			},
		});
	} finally {
		JSON.parse = previousJSONParse;
		globalThis.location = previousLocation;
		globalThis.document = previousDocument;
	}
});

test("action disconnects before relaying an invalid normalized page", async () => {
	const port = fakeNativePort();
	const chromeApi = {
		runtime: {
			connectNative() { return port; },
			lastError: undefined,
		},
		scripting: {
			async executeScript() {
				return [{ frameId: 0, result: { status: "ok", page: { orders: [{}] } } }];
			},
		},
	};
	const completed = createOrdinaryBrowserActionHandler(chromeApi, function syntheticReader() {})({
		id: 9,
		url: "https://mc.coupang.com/ssr/desktop/order/list",
	});
	port.emitMessage({
		schema_version: 1,
		request_id: "cccccccccccccccccccccccccccccccc",
		operation: "read_order_document",
	});
	await eventLoopTurn();
	assert.equal(port.disconnected, true);
	assert.deepEqual(port.messages, []);
	await completed;
});

function fakeNativePort() {
	let messageListener;
	let disconnectListener;
	return {
		messages: [],
		disconnected: false,
		onMessage: {
			addListener(listener) {
				messageListener = listener;
			},
		},
		onDisconnect: {
			addListener(listener) {
				disconnectListener = listener;
			},
		},
		postMessage(message) {
			this.messages.push(message);
		},
		disconnect() {
			this.disconnected = true;
			disconnectListener?.();
		},
		emitMessage(message) {
			messageListener(message);
		},
		emitDisconnect() {
			disconnectListener();
		},
	};
}

function eventLoopTurn() {
	return new Promise((resolve) => setImmediate(resolve));
}
