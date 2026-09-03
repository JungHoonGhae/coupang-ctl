// This function is passed directly to chrome.scripting.executeScript. Keep all
// executable dependencies inside its body so Chrome can serialize it without
// importing code into the selected page.
export async function readSelectedOrderPage(cursor) {
	const MAX_DOCUMENT_BYTES = 8 << 20;
	const MAX_ORDERS = 5;
	const MAX_ITEMS = 100;
	const ID_KEYS = new Set([
		"orderId",
		"orderID",
		"orderNumber",
		"productId",
		"productID",
		"vendorItemId",
		"vendorItemID",
	]);

	try {
		if (
			location.origin !== "https://mc.coupang.com" ||
			location.pathname !== "/ssr/desktop/order/list"
		) {
			return { status: "ordinary_browser_unavailable" };
		}

		let documentText;
		if (cursor === null) {
			const nextData = document.querySelector("script#__NEXT_DATA__")?.textContent;
			if (!nextData) {
				if (document.querySelector('input[type="password"]')) {
					return { status: "authentication_required" };
				}
				const body = document.body?.innerText ?? "";
				if (/access denied|접근.{0,8}(거부|제한)|비정상.{0,8}접근/i.test(body)) {
					return { status: "access_denied" };
				}
				return { status: "structured_data_missing" };
			}
			documentText = nextData;
		} else {
			if (!validCursor(cursor)) return { status: "structured_data_missing" };
			const query = new URLSearchParams({
				pageIndex: String(cursor.page),
				requestYear: String(cursor.year),
				size: "5",
			});
			const response = await fetch(`/ssr/api/myorders/model?${query}`, {
				method: "GET",
				credentials: "include",
				headers: { accept: "application/json" },
			});
			if (response.status === 401) return { status: "authentication_required" };
			if (response.status === 403) return { status: "access_denied" };
			if (!response.ok) return { status: "structured_data_missing" };
			documentText = await response.text();
		}

		if (typeof documentText !== "string" || new TextEncoder().encode(documentText).length > MAX_DOCUMENT_BYTES) {
			return { status: "structured_data_missing" };
		}
		const root = parsePreservingIdentifiers(documentText);
		const domain =
			objectAt(root, "props", "pageProps", "domains", "desktopOrder") ??
			(isObject(root) && Array.isArray(root.orderList) ? root : null);
		if (!domain || !Array.isArray(domain.orderList) || domain.orderList.length > MAX_ORDERS) {
			return { status: "structured_data_missing" };
		}
		const orders = [];
		for (const rawOrder of domain.orderList) {
			const normalized = await normalizeOrder(rawOrder);
			if (!normalized) return { status: "structured_data_missing" };
			orders.push(normalized);
		}
		const page = { orders };
		const pagination = isObject(domain.orderPagination) ? domain.orderPagination : domain;
		if (pagination.hasNext === true) {
			const year = integerValue(pagination, ["nextYear"]);
			const nextPage = integerValue(pagination, ["nextPageIndex", "nextPage"]);
			if (year === null || nextPage === null || !validCursor({ year, page: nextPage })) {
				return { status: "structured_data_missing" };
			}
			page.next = { year, page: nextPage };
		}
		return { status: "ok", page };
	} catch {
		return { status: "structured_data_missing" };
	}

	async function normalizeOrder(raw) {
		if (!isObject(raw)) return null;
		const sourceID = scalarString(raw, ["orderId", "orderID", "orderNumber"]);
		const moment = normalizeMoment(firstValue(raw, ["orderDate", "orderedAt", "purchasedAt"]));
		const total = amountValue(raw, ["totalPrice", "orderTotalPrice", "totalAmount", "paidAmount", "totalProductPrice"]);
		if (!sourceID || sourceID.length > 256 || !moment || total === null || total < 0) return null;
		const discount = amountValue(raw, ["discountAmount", "totalDiscountPrice"]) ?? 0;
		const shipping = amountValue(raw, ["shippingFee", "deliveryFee", "baseDeliveryPrice"]) ?? 0;
		if (discount < 0 || shipping < 0) return null;
		const items = collectItems(raw);
		if (items === null || items.length > MAX_ITEMS) return null;
		const currency = stringValue(raw, ["orderCurrencyType", "currency"]) || "KRW";
		if (currency !== "KRW") return null;
		return {
			source_ref: await sourceReference(sourceID),
			purchased_at: moment.date,
			...(moment.timestamp ? { purchased_at_time: moment.timestamp } : {}),
			total_amount: total,
			discount_amount: discount,
			shipping_fee: shipping,
			currency,
			...(raw.allCanceled === true ? { fully_canceled: true } : {}),
			...(isObject(raw.paymentReceiptInfo) && raw.paymentReceiptInfo.paymentReceiptVisible === true
				? { receipt_available: true }
				: {}),
			items,
		};
	}

	function collectItems(raw) {
		const keys = ["deliveryGroupList", "shipments", "deliveries", "shipmentList", "orderItems", "items"];
		for (const key of keys) {
			if (raw[key] === undefined || raw[key] === null) continue;
			const items = [];
			if (!walkItems(raw[key], {}, items) || items.length > MAX_ITEMS) return null;
			return items;
		}
		return [];
	}

	function walkItems(value, inherited, items) {
		if (Array.isArray(value)) {
			for (const child of value) {
				if (!walkItems(child, inherited, items)) return false;
			}
			return true;
		}
		if (!isObject(value)) return true;
		const current = { ...inherited };
		const status = stringValue(value, ["deliveryStatus", "shipmentStatus", "status"]);
		if (status) current.status = normalizeDeliveryStatus(status);
		else if (isObject(value.groupStatus)) current.status = normalizeDeliveryStatus(stringValue(value.groupStatus, ["status"]));
		const delivered = firstValue(value, ["deliveredAt", "deliveredDate", "deliveryCompletedAt"]);
		const deliveredAt = timeValue(delivered);
		if (deliveredAt) current.deliveredAt = deliveredAt;
		if (isObject(value.vendor)) current.sellerName = stringValue(value.vendor, ["vendorName", "sellerName"]);
		if (looksLikeItem(value)) {
			const item = normalizeItem(value, current);
			if (!item) return false;
			items.push(item);
			return items.length <= MAX_ITEMS;
		}
		for (const key of Object.keys(value).sort()) {
			if (!walkItems(value[key], current, items)) return false;
		}
		return true;
	}

	function looksLikeItem(raw) {
		const name = stringValue(raw, ["productName", "itemName", "vendorItemName"]);
		const quantity = integerValue(raw, ["quantity", "orderQuantity"]);
		return name !== "" && quantity !== null && quantity > 0;
	}

	function normalizeItem(raw, delivery) {
		const quantity = integerValue(raw, ["quantity", "orderQuantity"]);
		const cancelled = integerValue(raw, ["cancelQuantity", "cancelledQuantity"]) ?? 0;
		const returned = integerValue(raw, ["returnReceiptQuantity", "returnedQuantity"]) ?? 0;
		const unitPrice = amountValue(raw, ["unitPrice", "salesPrice", "listPrice"]) ?? 0;
		let paidPrice = amountValue(raw, ["paidPrice", "discountedPrice", "orderPrice"]);
		if (paidPrice === null) {
			const discountedUnit = amountValue(raw, ["discountedUnitPrice", "combinedUnitPrice"]);
			paidPrice = discountedUnit === null ? unitPrice * quantity : discountedUnit * quantity;
		}
		if (
			quantity === null || quantity < 1 || cancelled < 0 || returned < 0 ||
			cancelled > quantity || returned > quantity || unitPrice < 0 || paidPrice < 0 ||
			!Number.isSafeInteger(paidPrice)
		) return null;
		const productID = scalarString(raw, ["productId", "productID"]);
		const vendorItemID = scalarString(raw, ["vendorItemId", "vendorItemID"]);
		if (!validNumericID(productID) || !validNumericID(vendorItemID)) return null;
		const name = stringValue(raw, ["productName", "itemName", "vendorItemName"]);
		const seller = stringValue(raw, ["sellerName", "vendorName"]) || delivery.sellerName || "";
		const brand = isObject(raw.brandInfo) ? stringValue(raw.brandInfo, ["brandName", "officialBrandName"]) : "";
		const productType = stringValue(raw, ["productType"]);
		const divisionType = stringValue(raw, ["divisionType"]);
		if (!validText(name, 2000) || !validText(seller, 1000) || !validText(brand, 1000) || !validText(productType, 200) || !validText(divisionType, 200)) return null;
		const membershipTypes = new Set(["MEMBERSHIP", "WOW_MEMBERSHIP", "MEMBERSHIP_FEE", "SUBSCRIPTION"]);
		const commerceKind = membershipTypes.has(productType.toUpperCase()) || membershipTypes.has(divisionType.toUpperCase())
			? "membership_fee"
			: "product_purchase";
		return {
			...(productID ? { product_id: productID } : {}),
			...(vendorItemID ? { vendor_item_id: vendorItemID } : {}),
			name,
			quantity,
			...(cancelled ? { cancelled_quantity: cancelled } : {}),
			...(returned ? { returned_quantity: returned } : {}),
			unit_price: unitPrice,
			paid_price: paidPrice,
			...(seller ? { seller_name: seller } : {}),
			...(brand ? { brand_name: brand } : {}),
			...(productType ? { product_type: productType } : {}),
			...(divisionType ? { division_type: divisionType } : {}),
			commerce_kind: commerceKind,
			...(delivery.status ? { delivery_status: delivery.status } : {}),
			...(delivery.deliveredAt ? { delivered_at: delivery.deliveredAt } : {}),
		};
	}

	function parsePreservingIdentifiers(text) {
		return JSON.parse(quoteNumericIdentifierTokens(text), (key, value) => {
			if (!ID_KEYS.has(key) || typeof value !== "number") return value;
			if (Number.isSafeInteger(value) && value >= 0) return String(value);
			throw new Error("identifier precision unavailable");
		});
	}

	function quoteNumericIdentifierTokens(text) {
		let quoted = "";
		let index = 0;
		while (index < text.length) {
			if (text[index] !== '"') {
				quoted += text[index];
				index += 1;
				continue;
			}
			const end = jsonStringEnd(text, index);
			if (end === -1) return text;
			const token = text.slice(index, end);
			quoted += token;
			index = end;

			let colon = index;
			while (/\s/.test(text[colon] ?? "")) colon += 1;
			if (text[colon] !== ":") continue;
			let valueStart = colon + 1;
			while (/\s/.test(text[valueStart] ?? "")) valueStart += 1;
			let key;
			try {
				key = JSON.parse(token);
			} catch {
				return text;
			}
			if (!ID_KEYS.has(key) || !/\d/.test(text[valueStart] ?? "")) continue;
			let valueEnd = valueStart;
			while (/\d/.test(text[valueEnd] ?? "")) valueEnd += 1;
			if (!/[\s,}\]]/.test(text[valueEnd] ?? "")) continue;
			quoted += `${text.slice(index, valueStart)}"${text.slice(valueStart, valueEnd)}"`;
			index = valueEnd;
		}
		return quoted;
	}

	function jsonStringEnd(text, start) {
		let escaped = false;
		for (let index = start + 1; index < text.length; index += 1) {
			if (escaped) {
				escaped = false;
				continue;
			}
			if (text[index] === "\\") {
				escaped = true;
				continue;
			}
			if (text[index] === '"') return index + 1;
		}
		return -1;
	}

	function objectAt(root, ...path) {
		let value = root;
		for (const key of path) {
			if (!isObject(value?.[key])) return null;
			value = value[key];
		}
		return value;
	}

	function firstValue(raw, keys) {
		for (const key of keys) {
			if (raw[key] !== undefined && raw[key] !== null) return raw[key];
		}
		return undefined;
	}

	function stringValue(raw, keys) {
		const value = firstValue(raw, keys);
		return typeof value === "string" ? value.trim() : "";
	}

	function scalarString(raw, keys) {
		const value = firstValue(raw, keys);
		if (typeof value === "string") return value.trim();
		if (typeof value === "number" && Number.isSafeInteger(value)) return String(value);
		return "";
	}

	function integerValue(raw, keys) {
		return amountValue(raw, keys);
	}

	function amountValue(raw, keys) {
		const value = firstValue(raw, keys);
		if (typeof value === "number") return Number.isSafeInteger(value) ? value : null;
		if (typeof value !== "string") return null;
		const cleaned = value.replaceAll(",", "").replace("₩", "").replace("원", "").replaceAll(" ", "");
		if (!/^-?\d+$/.test(cleaned)) return null;
		const parsed = Number(cleaned);
		return Number.isSafeInteger(parsed) ? parsed : null;
	}

	function normalizeMoment(value) {
		if (typeof value === "string") {
			const trimmed = value.trim();
			const dateOnly = /^(\d{4})[-/.]\s?(\d{2})[-/.]\s?(\d{2})$/.exec(trimmed);
			if (dateOnly) {
				const date = `${dateOnly[1]}-${dateOnly[2]}-${dateOnly[3]}`;
				return validDate(date) ? { date } : null;
			}
			if (!/^\d{4}-\d{2}-\d{2}T.*(?:Z|[+-]\d{2}:?\d{2})$/.test(trimmed)) return null;
			const timestamp = timeValue(trimmed);
			return timestamp ? { date: kstDate(timestamp), timestamp } : null;
		}
		const timestamp = timeValue(value);
		return timestamp ? { date: kstDate(timestamp), timestamp } : null;
	}

	function timeValue(value) {
		let milliseconds;
		if (typeof value === "string") {
			const trimmed = value.trim();
			if (/^\d{4}[-/.]\s?\d{2}[-/.]\s?\d{2}$/.test(trimmed)) {
				const normalized = trimmed.replaceAll(". ", "-").replaceAll(".", "-").replaceAll("/", "-");
				milliseconds = Date.parse(`${normalized}T00:00:00Z`);
			} else if (/^\d{4}-\d{2}-\d{2}T.*(?:Z|[+-]\d{2}:?\d{2})$/.test(trimmed)) {
				milliseconds = Date.parse(trimmed);
			} else return null;
		} else if (typeof value === "number" && Number.isSafeInteger(value)) {
			milliseconds = value >= 1_000_000_000_000 ? value : value * 1000;
		} else return null;
		if (!Number.isFinite(milliseconds)) return null;
		const date = new Date(milliseconds);
		const year = date.getUTCFullYear();
		return year >= 2000 && year <= 2100 ? date.toISOString() : null;
	}

	function kstDate(timestamp) {
		return new Date(Date.parse(timestamp) + 9 * 60 * 60 * 1000).toISOString().slice(0, 10);
	}

	function validDate(value) {
		const parsed = new Date(`${value}T00:00:00Z`);
		return !Number.isNaN(parsed.valueOf()) && parsed.toISOString().slice(0, 10) === value && Number(value.slice(0, 4)) >= 2000 && Number(value.slice(0, 4)) <= 2100;
	}

	function normalizeDeliveryStatus(value) {
		const normalized = value.trim().toLowerCase();
		if ((normalized.includes("deliver") && normalized.includes("complete")) || normalized.includes("delivered") || normalized.includes("배송완료")) return "delivered";
		if (normalized.includes("shipping") || normalized.includes("transit") || normalized.includes("배송중")) return "in_transit";
		if (normalized.includes("cancel") || normalized.includes("취소")) return "cancelled";
		if (normalized.includes("return") || normalized.includes("반품")) return "returned";
		return normalized === "" ? "" : "other";
	}

	function validCursor(value) {
		return isObject(value) && Object.keys(value).sort().join(",") === "page,year" &&
			Number.isInteger(value.year) && value.year >= 2000 && value.year <= 2100 &&
			Number.isInteger(value.page) && value.page >= 0 && value.page <= 1000;
	}

	function validNumericID(value) {
		return value === "" || /^\d{1,24}$/.test(value);
	}

	function validText(value, maxBytes) {
		return typeof value === "string" && !value.includes("\0") && new TextEncoder().encode(value).length <= maxBytes;
	}

	function isObject(value) {
		return value !== null && typeof value === "object" && !Array.isArray(value);
	}

	async function sourceReference(sourceID) {
		const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(`coupangctl:order:${sourceID}`));
		return [...new Uint8Array(digest)].map((byte) => byte.toString(16).padStart(2, "0")).join("");
	}
}
