package partners

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const syntheticProductURL = "https://www.coupang.com/vp/products/101?itemId=201&vendorItemId=301"

func TestConvertSignsOfficialShapeAndCachesValidatedLink(t *testing.T) {
	fixed := time.Date(2026, 9, 2, 1, 2, 3, 0, time.UTC)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodPost || request.URL.Path != "/v2/providers/affiliate_open_api/apis/openapi/v1/deeplink" {
			t.Fatalf("unexpected request target: %s %s", request.Method, request.URL.RequestURI())
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		var payload struct {
			CoupangURLs []string `json:"coupangUrls"`
			SubID       string   `json:"subId"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.CoupangURLs) != 1 || payload.CoupangURLs[0] != syntheticProductURL || payload.SubID != "oss_test" {
			t.Fatalf("unexpected request body: %s", body)
		}
		message := "260902T010203Z" + http.MethodPost + request.URL.Path
		mac := hmac.New(sha256.New, []byte("synthetic-secret"))
		_, _ = mac.Write([]byte(message))
		expected := "CEA algorithm=HmacSHA256, access-key=synthetic-access, signed-date=260902T010203Z, signature=" + hex.EncodeToString(mac.Sum(nil))
		if request.Header.Get("Authorization") != expected {
			t.Fatal("unexpected authorization signature")
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"rCode":"0","rMessage":"","data":[{"originalUrl":"` + syntheticProductURL + `","shortenUrl":"https://link.coupang.com/a/synthetic"}]}`))
	}))
	defer server.Close()

	client := testClient(server.Client(), server.URL+"/v2/providers/affiliate_open_api/apis/openapi/v1/deeplink", fixed)
	links, err := client.Convert(context.Background(), []string{syntheticProductURL})
	if err != nil {
		t.Fatal(err)
	}
	if links[syntheticProductURL] != "https://link.coupang.com/a/synthetic" {
		t.Fatalf("unexpected converted links: %#v", links)
	}
	if _, err := client.Convert(context.Background(), []string{syntheticProductURL}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("cached link caused %d network requests", calls.Load())
	}
}

func TestConvertRejectsUntrustedInputsAndOutputs(t *testing.T) {
	client := testClient(http.DefaultClient, deeplinkEndpoint, time.Now())
	for _, raw := range []string{
		"http://www.coupang.com/vp/products/101",
		"https://evil.example/vp/products/101",
		"https://www.coupang.com/vp/products/101?redirect=https://evil.example",
		"https://www.coupang.com/vp/products/not-numeric",
	} {
		if _, err := client.Convert(context.Background(), []string{raw}); err == nil {
			t.Fatalf("accepted untrusted product URL %q", raw)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"rCode":0,"data":[{"originalUrl":"` + syntheticProductURL + `","shortenUrl":"https://evil.example/affiliate"}]}`))
	}))
	defer server.Close()
	client = testClient(server.Client(), server.URL, time.Now())
	links, err := client.Convert(context.Background(), []string{syntheticProductURL})
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Fatalf("accepted untrusted affiliate response: %#v", links)
	}
}

func TestConvertNeverReturnsRemoteErrorDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusUnauthorized)
		_, _ = response.Write([]byte(`{"message":"sensitive remote diagnostic"}`))
	}))
	defer server.Close()
	client := testClient(server.Client(), server.URL, time.Now())
	_, err := client.Convert(context.Background(), []string{syntheticProductURL})
	if err == nil || strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestNewFromEnvironmentRequiresBothKeysAndHonorsOptOut(t *testing.T) {
	values := map[string]string{
		"COUPANG_PARTNERS_ACCESS_KEY": "synthetic-access",
		"COUPANG_PARTNERS_SECRET_KEY": "synthetic-secret",
		"COUPANG_PARTNERS_SUB_ID":     "bad sub id",
	}
	getenv := func(key string) string { return values[key] }
	client := NewFromEnvironment(getenv)
	if client == nil || client.subID != "" {
		t.Fatalf("unexpected environment client: %#v", client)
	}
	values["COUPANGCTL_AFFILIATE_DISABLED"] = "true"
	if NewFromEnvironment(getenv) != nil {
		t.Fatal("global affiliate opt-out was ignored")
	}
	delete(values, "COUPANGCTL_AFFILIATE_DISABLED")
	delete(values, "COUPANG_PARTNERS_SECRET_KEY")
	if NewFromEnvironment(getenv) != nil {
		t.Fatal("client was created with an incomplete credential pair")
	}
}

func testClient(httpClient *http.Client, endpoint string, now time.Time) *Client {
	return &Client{
		accessKey: "synthetic-access",
		secretKey: "synthetic-secret",
		subID:     "oss_test",
		endpoint:  endpoint,
		http:      httpClient,
		now:       func() time.Time { return now },
		cache:     make(map[string]string),
	}
}
