package receipts_test

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	receiptparser "github.com/JungHoonGhae/coupang-ctl/internal/coupang/receipts"
)

func TestReceiptParsersDiscardIdentifiersAndDownloadURLs(t *testing.T) {
	status, err := receiptparser.ParseStatusDocument([]byte(`{
  "cash":{"success":true,"message":"ok","data":false},
  "card":{"success":true,"message":"ok","data":true}
}`))
	if err != nil || len(status.Statuses) != 2 || status.Statuses[0].RequestInProgress || !status.Statuses[0].CanRequestNew || !status.Statuses[1].RequestInProgress {
		t.Fatalf("unexpected status: %#v %v", status, err)
	}

	history, err := receiptparser.ParseHistoryDocument([]byte(`{
  "success":true,"message":"ok","data":{"pageIndex":0,"pageSize":5,"hasNext":false,"nextPageIndex":0,"hasPrev":false,"prevPageIndex":0,
  "list":[{"requestDate":"2026.09.03","from":"2026.01.01","to":"2026.09.03","totalCount":2,"totalAmount":34000,"displayCardName":"Synthetic card","requestStatus":"COMPLETED","downloadUrlList":[{"startIndex":1,"endIndex":2,"downloadUrl":"https://mc.coupang.com/private/signed"}]}]}}
`), core.ReceiptHistoryRequest{Kind: core.ReceiptKindCard, PageSize: 5})
	if err != nil || len(history.Items) != 1 || history.Items[0].DownloadCount != 1 || history.Items[0].PaymentMethodDisplay != "Synthetic card" {
		t.Fatalf("unexpected history: %#v %v", history, err)
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(history.Items[0].Status)), "https") {
		t.Fatal("download URL leaked into typed history")
	}
}

func TestCardSummaryKeepsDisplayAggregatesButDropsCardNumbers(t *testing.T) {
	document := []byte(`{
  "kind":"card",
  "summary":{"success":true,"message":"ok","data":{"from":"2026.01.01","to":"2026.09.04","totalCount":4,"totalAmount":99000,"cardList":[{"cardId":"private-id","cardNumber":"private-number","displayCardName":"Synthetic card","displayCardNumber":"****"}]}},
  "per_card":[
    {"success":true,"message":"ok","data":{"from":"2026.01.01","to":"2026.09.03","totalCount":3,"totalAmount":74000,"displayCardName":"Synthetic card"}},
    {"success":true,"message":"ok","data":{"from":"2026.01.01","to":"2026.09.03","totalCount":1,"totalAmount":25000,"displayCardName":"Synthetic card"}}
  ]
}`)
	got, err := receiptparser.ParseSummaryDocument(document, core.ReceiptSummaryRequest{Kind: core.ReceiptKindCard, From: "2026-01-01", To: "2026-09-03"})
	if err != nil || got.From != "2026-01-01" || got.To != "2026-09-03" || len(got.Warnings) != 1 || len(got.PaymentMethods) != 1 || got.PaymentMethods[0].TotalCount != 4 || got.PaymentMethods[0].TotalAmountKRW != 99000 || got.PaymentMethods[0].Provenance != "derived_from_observed_receipt_summaries" {
		t.Fatalf("unexpected summary: %#v %v", got, err)
	}
	encoded := strings.ToLower(string(mustJSON(t, got)))
	if strings.Contains(encoded, "private-id") || strings.Contains(encoded, "private-number") || strings.Contains(encoded, "card_number") {
		t.Fatalf("private card identifiers leaked: %s", encoded)
	}
}

func TestDownloadParserRejectsUnsafeTypesAndNormalizesFilename(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("synthetic pdf"))
	download, err := receiptparser.ParseDownloadDocument([]byte(`{"filename":"../../Synthetic receipt","content_type":"application/pdf","bytes":13,"base64":"` + encoded + `"}`))
	if err != nil || download.Metadata.Filename != "Synthetic-receipt.pdf" || string(download.Content) != "synthetic pdf" {
		t.Fatalf("unexpected download: %#v %v", download.Metadata, err)
	}
	if _, err := receiptparser.ParseDownloadDocument([]byte(`{"filename":"payload","content_type":"application/x-executable","bytes":1,"base64":"eA=="}`)); err == nil {
		t.Fatal("unsafe receipt content type accepted")
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
