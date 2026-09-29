package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func okServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

func TestProvidersFor(t *testing.T) {
	auto := providersFor("")
	if len(auto) != len(defaultProviders()) {
		t.Fatalf("expected %d providers in auto mode, got %d", len(defaultProviders()), len(auto))
	}
	for _, p := range auto {
		if strings.Contains(p.url, "{ip}") {
			t.Fatalf("placeholder not resolved for %s: %s", p.name, p.url)
		}
	}
	var ipapi *provider
	for i := range auto {
		if auto[i].name == "ip-api.com" {
			ipapi = &auto[i]
		}
	}
	if ipapi == nil || !strings.Contains(ipapi.url, "/json/?fields=") {
		t.Fatalf("ip-api auto URL malformed: %v", ipapi)
	}

	explicit := providersFor("2001:db8::1")
	if len(explicit) != 1 {
		t.Fatalf("expected 1 explicit provider, got %d: %v", len(explicit), explicit)
	}
	if explicit[0].name != "ip-api.com" {
		t.Fatalf("unexpected explicit provider: %+v", explicit[0])
	}
	if !strings.Contains(explicit[0].url, "/json/2001:db8::1?") {
		t.Fatalf("explicit URL missing query IP: %s", explicit[0].url)
	}
}

func TestFetchPublicIPFallback(t *testing.T) {
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer broken.Close()
	ok := okServer(t, `{"status":"success","query":"9.9.9.9","country":"中国"}`)
	defer ok.Close()

	providers := []provider{
		{name: "broken", url: broken.URL, parse: parseIPAPI},
		{name: "ok", url: ok.URL, parse: parseIPAPI},
	}
	info, err := fetchPublicIP(providers, http.DefaultClient, time.Second, time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Source != "ok" || info.IP != "9.9.9.9" {
		t.Fatalf("expected fallback to ok provider, got: %+v", info)
	}
	wantRaw := `{"status":"success","query":"9.9.9.9","country":"中国"}`
	if info.Raw != wantRaw {
		t.Fatalf("raw body mismatch, got: %q", info.Raw)
	}
}

func TestFetchPublicIPTimeoutFallback(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.Write([]byte(`{"status":"success","query":"8.7.6.5"}`))
	}))
	defer slow.Close()
	ok := okServer(t, `{"status":"success","query":"9.9.9.9","country":"中国"}`)
	defer ok.Close()

	providers := []provider{
		{name: "slow", url: slow.URL, parse: parseIPAPI},
		{name: "ok", url: ok.URL, parse: parseIPAPI},
	}
	info, err := fetchPublicIP(providers, http.DefaultClient, 50*time.Millisecond, time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Source != "ok" {
		t.Fatalf("expected timeout fallback to ok provider, got: %+v", info)
	}
}

func TestFetchPublicIPAllFail(t *testing.T) {
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "a down", http.StatusBadGateway)
	}))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "b down", http.StatusServiceUnavailable)
	}))
	defer b.Close()

	providers := []provider{
		{name: "alpha", url: a.URL, parse: parseIPAPI},
		{name: "beta", url: b.URL, parse: parseIPAPI},
	}
	info, err := fetchPublicIP(providers, http.DefaultClient, time.Second, time.Time{})
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
	if info != nil {
		t.Fatalf("expected nil info, got: %+v", info)
	}
	for _, name := range []string{"alpha", "beta"} {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("error should mention %s, got: %v", name, err)
		}
	}
}

func TestFetchPublicIPKeepsHealthyOnOrdinaryParseError(t *testing.T) {
	original := geoAPIServiceHealthy.Load()
	defer func() { geoAPIServiceHealthy.Store(original) }()
	geoAPIServiceHealthy.Store(true)

	bad := okServer(t, `{"status":"fail","message":"reserved range"}`)
	defer bad.Close()
	providers := []provider{{name: ipAPIName, url: bad.URL, parse: parseIPAPI}}
	if _, err := fetchPublicIP(providers, http.DefaultClient, time.Second, time.Time{}); err == nil {
		t.Fatal("expected error for fail status")
	}
	if !geoAPIServiceHealthy.Load() {
		t.Fatal("ordinary parse failure should not mark the service unhealthy")
	}
}

func TestFetchPublicIPMarksUnhealthyOnHTTP429(t *testing.T) {
	original := geoAPIServiceHealthy.Load()
	defer func() { geoAPIServiceHealthy.Store(original) }()
	geoAPIServiceHealthy.Store(true)

	limited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer limited.Close()
	providers := []provider{{name: ipAPIName, url: limited.URL, parse: parseIPAPI}}
	if _, err := fetchPublicIP(providers, http.DefaultClient, time.Second, time.Time{}); err == nil {
		t.Fatal("expected error for HTTP 429")
	}
	if geoAPIServiceHealthy.Load() {
		t.Fatal("HTTP 429 should mark the service unhealthy")
	}
}

func TestFetchPublicIPBudgetAcrossProviders(t *testing.T) {
	var secondHits atomic.Int64
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte(`{"status":"success","query":"9.9.9.9"}`))
	}))
	defer slow.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHits.Add(1)
		http.Error(w, "should not run", http.StatusServiceUnavailable)
	}))
	defer second.Close()
	providers := []provider{
		{name: "slow", url: slow.URL, parse: parseIPAPI},
		{name: "second", url: second.URL, parse: parseIPAPI},
	}

	started := time.Now()
	_, err := fetchPublicIP(providers, http.DefaultClient, time.Second, time.Now().Add(120*time.Millisecond))
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("expected error when budget expires")
	}
	if elapsed > 800*time.Millisecond {
		t.Fatalf("budget should bound provider attempts, elapsed: %v", elapsed)
	}
	// 并发后所有服务都会被请求，不再有「预算耗尽就跳过后续服务」的行为；
	// 预算改为约束整体墙钟时间，由上面的 elapsed 断言守住。
	if got := secondHits.Load(); got == 0 {
		t.Fatal("concurrent fetch should have queried the second provider")
	}
}

// cancelProbeTransport 在客户端侧观测取消：慢分支必定进入 RoundTrip 并阻塞在
// req.Context() 上，因此不依赖「服务端 handler 是否来得及启动」这种时序假设。
type cancelProbeTransport struct {
	canceled chan string
}

func (t *cancelProbeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.URL.Path, "/slow") {
		<-req.Context().Done()
		t.canceled <- req.URL.Path
		return nil, req.Context().Err()
	}
	body := `{"status":"success","query":"9.9.9.9"}`
	return &http.Response{
		StatusCode:    http.StatusOK,
		Body:          io.NopCloser(strings.NewReader(body)),
		Header:        make(http.Header),
		ContentLength: int64(len(body)),
		Request:       req,
	}, nil
}

func TestFetchPublicIPCancelsRemainingOnSuccess(t *testing.T) {
	transport := &cancelProbeTransport{canceled: make(chan string, 4)}
	providers := []provider{
		{name: "slow", url: "http://example.test/slow", parse: parseIPAPI},
		{name: "ok", url: "http://example.test/ok", parse: parseIPAPI},
	}

	started := time.Now()
	info, err := fetchPublicIP(providers, &http.Client{Transport: transport}, 5*time.Second, time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Source != "ok" {
		t.Fatalf("expected the responsive provider to win, got: %+v", info)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("should not wait for the slow provider, elapsed: %v", elapsed)
	}
	select {
	case path := <-transport.canceled:
		if path != "/slow" {
			t.Fatalf("canceled request = %q, want /slow", path)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("expected the remaining in-flight request to be canceled")
	}
}

func TestFetchPublicIPEmptyProviders(t *testing.T) {
	_, err := fetchPublicIP(nil, http.DefaultClient, time.Second, time.Time{})
	if err == nil || !strings.Contains(err.Error(), "重试预算已用尽") {
		t.Fatalf("expected budget error for empty provider list, got: %v", err)
	}
}

func TestFetchPublicIPAllFailErrorKeepsDeclarationOrder(t *testing.T) {
	failing := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "down", http.StatusBadGateway)
		}))
	}
	// 首个服务故意慢一些，确保完成顺序与声明顺序不同。
	slowFirst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer slowFirst.Close()
	second := failing()
	defer second.Close()
	third := failing()
	defer third.Close()

	providers := []provider{
		{name: "alpha", url: slowFirst.URL, parse: parseIPAPI},
		{name: "beta", url: second.URL, parse: parseIPAPI},
		{name: "gamma", url: third.URL, parse: parseIPAPI},
	}
	_, err := fetchPublicIP(providers, http.DefaultClient, 2*time.Second, time.Time{})
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
	alpha, beta, gamma := strings.Index(err.Error(), "alpha"), strings.Index(err.Error(), "beta"), strings.Index(err.Error(), "gamma")
	if alpha < 0 || beta < 0 || gamma < 0 {
		t.Fatalf("error should mention every provider, got: %v", err)
	}
	if !(alpha < beta && beta < gamma) {
		t.Fatalf("error should list providers in declaration order, got: %v", err)
	}
}
