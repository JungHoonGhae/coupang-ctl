import assert from "node:assert/strict";
import test from "node:test";
import { readSelectedOrderPage } from "../internal/browser/order_page_reader.js";

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

test("order reader requires explicit pagination evidence on bootstrap and continuation", async (t) => {
	const cases = [
		["partial page", { hasNext: false, partial: true }, "order_partial"],
		["partial continuation", { hasNext: true, nextYear: 2026, nextPageIndex: 2, partial: true }, "order_partial"],
		["non-partial end", { hasNext: false, partial: false }, "ok"],
		["non-partial continuation", { hasNext: true, nextYear: 2026, nextPageIndex: 2, partial: false }, "ok", { year: 2026, page: 2 }],
		...[null, "false", 0, [], {}].map(partial => ["ambiguous partial " + JSON.stringify(partial), { hasNext: false, partial }, "structured_data_missing"]),
		["missing", {}, "structured_data_missing"],
		["null flag", { hasNext: null }, "structured_data_missing"],
		["string flag", { hasNext: "false" }, "structured_data_missing"],
		["string continuation", { hasNext: "true", nextYear: 2026, nextPageIndex: 1 }, "structured_data_missing"],
		["number flag", { hasNext: 0 }, "structured_data_missing"],
		["empty nested", { orderPagination: {} }, "structured_data_missing"],
		["null nested", { hasNext: false, orderPagination: null }, "structured_data_missing"],
		["array nested", { hasNext: false, orderPagination: [] }, "structured_data_missing"],
		["missing cursor", { hasNext: true }, "structured_data_missing"],
		["explicit end", { hasNext: false }, "ok"],
		["nested end", { orderPagination: { hasNext: false } }, "ok"],
		["empty continuation", { hasNext: true, nextYear: 2026, nextPageIndex: 1 }, "ok", { year: 2026, page: 1 }],
	];
	for (const cursor of [null, { year: 2026, page: 2 }]) {
		for (const [name, pagination, status, next] of cases) {
			await t.test(`${cursor ? "continuation" : "bootstrap"}: ${name}`, async () => {
				const saved = { location: globalThis.location, document: globalThis.document, fetch: globalThis.fetch };
				let fetchCalls = 0;
				const domain = { orderList: [], ...pagination };
				globalThis.location = { origin: "https://mc.coupang.com", pathname: "/ssr/desktop/order/list" };
				globalThis.document = { querySelector: () => ({ textContent: JSON.stringify({ props: { pageProps: { domains: { desktopOrder: domain } } } }) }) };
				globalThis.fetch = async () => {
					fetchCalls++;
					return { status: 200, ok: true, text: async () => JSON.stringify(domain) };
				};
				try {
					const result = await readSelectedOrderPage(cursor);
					assert.equal(result.status, status);
					assert.equal(fetchCalls, cursor ? 1 : 0);
					if (status === "ok") {
						assert.deepEqual(result.page.orders, []);
						assert.deepEqual(result.page.next, next);
					} else {
						assert.equal(result.page, undefined, "must not relay partial data without pagination evidence");
					}
				} finally {
					Object.assign(globalThis, saved);
				}
			});
		}
	}
});
