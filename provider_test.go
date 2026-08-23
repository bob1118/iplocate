package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
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
	info, err := fetchPublicIP(providers, http.DefaultClient, time.Second)
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
	info, err := fetchPublicIP(providers, http.DefaultClient, 50*time.Millisecond)
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
	info, err := fetchPublicIP(providers, http.DefaultClient, time.Second)
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
