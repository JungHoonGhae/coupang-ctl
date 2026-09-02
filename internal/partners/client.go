package partners

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	deeplinkEndpoint = "https://api-gateway.coupang.com/v2/providers/affiliate_open_api/apis/openapi/v1/deeplink"
	maxLinksPerCall  = 20
	maxResponseBytes = 1 << 20
)

var (
	errUnavailable       = errors.New("Coupang Partners deeplink API unavailable")
	productPathPattern   = regexp.MustCompile(`^/vp/products/[0-9]{1,24}$`)
	subIDPattern         = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)
	allowedProductParams = map[string]bool{"itemId": true, "vendorItemId": true}
)

type Client struct {
	accessKey string
	secretKey string
	subID     string
	endpoint  string
	http      *http.Client
	now       func() time.Time

	mu    sync.RWMutex
	cache map[string]string
}

func NewFromEnvironment(getenv func(string) string) *Client {
	if getenv == nil || environmentFlag(getenv("COUPANGCTL_AFFILIATE_DISABLED")) {
		return nil
	}
	accessKey := strings.TrimSpace(getenv("COUPANG_PARTNERS_ACCESS_KEY"))
	secretKey := strings.TrimSpace(getenv("COUPANG_PARTNERS_SECRET_KEY"))
	if accessKey == "" || secretKey == "" {
		return nil
	}
	subID := strings.TrimSpace(getenv("COUPANG_PARTNERS_SUB_ID"))
	if !subIDPattern.MatchString(subID) {
		subID = ""
	}
	return &Client{
		accessKey: accessKey,
		secretKey: secretKey,
		subID:     subID,
		endpoint:  deeplinkEndpoint,
		http: &http.Client{
			Timeout: 8 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		now:   time.Now,
		cache: make(map[string]string),
	}
}

func (c *Client) Convert(ctx context.Context, productURLs []string) (map[string]string, error) {
	if c == nil || c.http == nil || c.accessKey == "" || c.secretKey == "" {
		return nil, errUnavailable
	}
	unique := make([]string, 0, len(productURLs))
	seen := make(map[string]struct{}, len(productURLs))
	result := make(map[string]string, len(productURLs))
	for _, productURL := range productURLs {
		if _, exists := seen[productURL]; exists {
			continue
		}
		if !validProductURL(productURL) {
			return nil, errors.New("invalid canonical Coupang product URL")
		}
		seen[productURL] = struct{}{}
		if cached, ok := c.cached(productURL); ok {
			result[productURL] = cached
			continue
		}
		unique = append(unique, productURL)
	}
	if len(unique) == 0 {
		return result, nil
	}
	if len(unique) > maxLinksPerCall {
		return nil, errors.New("too many Coupang product URLs")
	}
	payload := struct {
		CoupangURLs []string `json:"coupangUrls"`
		SubID       string   `json:"subId,omitempty"`
	}{CoupangURLs: unique, SubID: c.subID}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, errUnavailable
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errUnavailable
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", c.authorization(request.Method, request.URL, c.now().UTC()))

	response, err := c.http.Do(request)
	if err != nil {
		return nil, errUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errUnavailable
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(responseBody) > maxResponseBytes {
		return nil, errUnavailable
	}
	var envelope deeplinkResponse
	if err := json.Unmarshal(responseBody, &envelope); err != nil || envelope.RCode.String() != "0" {
		return nil, errUnavailable
	}
	for _, link := range envelope.Data {
		if _, requested := seen[link.OriginalURL]; !requested {
			continue
		}
		affiliateURL := link.ShortenURL
		if affiliateURL == "" {
			affiliateURL = link.LandingURL
		}
		if !validAffiliateURL(affiliateURL) {
			continue
		}
		result[link.OriginalURL] = affiliateURL
		c.store(link.OriginalURL, affiliateURL)
	}
	return result, nil
}

func (c *Client) authorization(method string, target *url.URL, now time.Time) string {
	timestamp := now.Format("060102T150405Z")
	message := timestamp + method + target.EscapedPath() + target.RawQuery
	mac := hmac.New(sha256.New, []byte(c.secretKey))
	_, _ = mac.Write([]byte(message))
	signature := hex.EncodeToString(mac.Sum(nil))
	return "CEA algorithm=HmacSHA256, access-key=" + c.accessKey + ", signed-date=" + timestamp + ", signature=" + signature
}

func (c *Client) cached(productURL string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, ok := c.cache[productURL]
	return value, ok
}

func (c *Client) store(productURL, affiliateURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[productURL] = affiliateURL
}

func validProductURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() != "www.coupang.com" || parsed.Port() != "" || parsed.User != nil || parsed.Fragment != "" || !productPathPattern.MatchString(parsed.EscapedPath()) {
		return false
	}
	for key, values := range parsed.Query() {
		if !allowedProductParams[key] || len(values) != 1 || !numeric(values[0]) {
			return false
		}
	}
	return true
}

func validAffiliateURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() == "link.coupang.com" && parsed.Port() == "" && parsed.User == nil
}

func numeric(value string) bool {
	if value == "" || len(value) > 24 {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func environmentFlag(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

type responseCode string

func (c *responseCode) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*c = responseCode(text)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return err
	}
	*c = responseCode(number.String())
	return nil
}

func (c responseCode) String() string {
	return string(c)
}

type deeplinkResponse struct {
	RCode    responseCode `json:"rCode"`
	RMessage string       `json:"rMessage"`
	Data     []struct {
		OriginalURL string `json:"originalUrl"`
		ShortenURL  string `json:"shortenUrl"`
		LandingURL  string `json:"landingUrl"`
	} `json:"data"`
}
