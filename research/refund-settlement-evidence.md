# Refund settlement evidence

Last live verification: 2026-09-03 (Asia/Seoul).

This note records only redacted response paths, JSON types, aggregate sample
counts, and provenance. It contains no order values, identifiers, dates,
product text, cookies, or raw payloads.

## Observed order-list model

A bounded headed read of one authenticated order-model page covered five order
entries, including one entry explicitly marked fully canceled. The following
source-native shapes were observed:

| Redacted path | Shape | Observation |
| --- | --- | --- |
| `order.allCanceled` | boolean | present on all five entries; true on the fully canceled sample |
| `order.deliveryCancelBundleList` | array | non-empty only on the fully canceled sample |
| `...productList[].cancelQuantity` | number | positive on the fully canceled sample |
| `...productList[].cancelReceiptQuantity` | number | positive on the fully canceled sample |
| `...productList[].cancelReturnStatus` | string/null | populated on the fully canceled sample |
| `...productList[].combinedUnitPrice` | number | positive inside the cancellation bundle |
| `...productList[].discountedUnitPrice` | number | positive inside the cancellation bundle |
| `...productList[].unitPrice` | number | positive inside the cancellation bundle |
| `order.bundleReceiptList[].importTax.refundedPriceEx` | null | null on all five samples |
| `order.bundleReceiptList[].importTax.overCollectedRefundPriceEx` | null | null on all five samples |

The `importTax` paths are scoped to import-tax over-collection and are not
evidence of a general order refund. Cancellation quantities and item prices can
describe a canceled line, but they do not prove the settled refund after
discount allocation, points, shipping, fees, or later adjustments. They must
not be labeled exact post-refund net spend.

## Adoption decision

- Keep fully canceled orders and observed canceled/returned unit counts in the
  existing analytics.
- Do not derive settled refund amounts by multiplying item prices and canceled
  quantities.
- Do not adopt either import-tax refund field as a general refund amount.
- Require a non-null, source-native settlement amount with status and semantic
  agreement across multiple redacted canceled and returned samples before
  adding exact post-refund net spend.

## Vendor-receipt follow-up

The payment-receipt page exposed a `vendor` domain and a static
`GET /payment-receipt/vendor-receipts/{orderId}` contract. Five order samples
returned HTTP 200 from the corresponding same-origin `/ssr/api` read and shared
the same key/type shape. Relevant observed fields included:

- vendor and product `originalPaymentPrice` / `originalPaymentCancelPrice`;
- Coupang Cash and cashable Coupang Cash price/cancel-price pairs;
- coupon-discount and card-instant-discount price/cancel-price pairs;
- product `quantity`, `canceledQuantity`, `unitPrice`, and
  `combinedUnitPrice`;
- vendor payment type/name/description and issued/product/delivery totals.

This is sufficient to adopt a private-local, single-order vendor-receipt read.
A subsequent bounded cross-state check covered four ordinary orders and one
fully canceled order. All five same-origin reads returned HTTP 200. The four
ordinary samples had zero `originalPaymentCancelPrice`, zero coupon/card/cash
cancel components, and zero `canceledQuantity`. The fully canceled sample had
positive `originalPaymentCancelPrice`, positive coupon cancel amount, and
positive `canceledQuantity`; its other observed cancel components were zero.

This comparison supports labeling these values as source-native cancellation
components. It is still not sufficient to call any cancel-price field a
completed refund: the response contained no verified refund-settlement status
or processor completion timestamp, and the sample did not cover a partial
return or every payment method.

The first subsequent full-history attempt was stopped by
`browser_access_denied` at its initial read. No bypass or repeated retry was
performed. The paced research probe remains available at
`research/probes/order-refund-metadata` for a later verification window.
