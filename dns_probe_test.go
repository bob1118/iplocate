package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type failTransport struct {
	err error
}

func (f failTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, f.err
}

func TestFetchGeoProvidersClientFallback(t *testing.T) {
	ok := okServer(t, `{"status":"success","query":"240e:1c:200::1","country":"中国"}`)
	defer ok.Close()
	providers := []provider{{name: "ok", url: ok.URL, parse: parseIPAPI}}
	bad := failTransport{err: errors.New("forced dial failure")}
	res := fetchGeoProviders(providers, []*http.Client{
		{Transport: bad},
		http.DefaultClient,
	}, time.Second, time.Second, "tcp6")
	if res.Error != "" || res.IP != "240e:1c:200::1" {
		t.Fatalf("expected client fallback to succeed, got: %+v", res)
	}

	broken := fetchGeoProviders(providers, []*http.Client{
		{Transport: bad},
		{Transport: bad},
	}, time.Second, time.Second, "tcp6")
	if !strings.Contains(broken.Error, "所有服务均失败") {
		t.Fatalf("expected all-clients failure, got: %+v", broken)
	}
}

func TestInspectDNS(t *testing.T) {
	servers := []dnsConfig{{IP: "8.8.8.8", Network: "IPv4"}, {IP: "2001:4860:4860::8888", Network: "IPv6"}}
	probe := func(_ context.Context, server string) (time.Duration, error) {
		if server == servers[1].IP {
			return 0, errors.New("timeout")
		}
		return 25 * time.Millisecond, nil
	}
	ownership := func(server string, _ time.Duration) *geoResult {
		return &geoResult{IP: server, Source: "test"}
	}
	got := inspectDNS(servers, time.Second, probe, ownership)
	if len(got) != 2 || !got[0].Reachable || got[1].Reachable {
		t.Fatalf("unexpected DNS results: %+v", got)
	}
	if got[0].LatencyMS != 25 || got[1].Error != "timeout" {
		t.Fatalf("unexpected probe details: %+v", got)
	}
	if got[0].Ownership.Source != "test" {
		t.Fatalf("missing ownership result: %+v", got[0])
	}
	if got[0].Network != "IPv4" || got[1].Network != "IPv6" {
		t.Fatalf("unexpected network scope: %+v", got)
	}
}

func TestDialFamilyFor(t *testing.T) {
	if got := dialFamilyFor("8.8.8.8"); got != "udp4" {
		t.Fatalf("dialFamilyFor(8.8.8.8) = %s, want udp4", got)
	}
	if got := dialFamilyFor("2001:4860:4860::8888"); got != "udp6" {
		t.Fatalf("dialFamilyFor(2001:4860:4860::8888) = %s, want udp6", got)
	}
}

func TestLookupDNSOwnershipHealthyGate(t *testing.T) {
	original := geoAPIServiceHealthy.Load()
	defer func() { geoAPIServiceHealthy.Store(original) }()

	geoAPIServiceHealthy.Store(false)
	if res := lookupDNSOwnership("240e:1c:200::1", time.Second); res != nil {
		t.Fatalf("expected nil ownership when ip-api marked unhealthy, got: %+v", res)
	}
}

func TestFetchGeoProvidersRetryBudget(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.Write([]byte(`{}`))
	}))
	defer slow.Close()
	providers := []provider{{name: "slow", url: slow.URL, parse: parseIPAPI}}
	bad := failTransport{err: errors.New("forced failure")}

	started := time.Now()
	res := fetchGeoProviders(providers, []*http.Client{
		{Transport: bad, Timeout: 50 * time.Millisecond},
		{Transport: bad, Timeout: 50 * time.Millisecond},
	}, 50*time.Millisecond, 60*time.Millisecond, "tcp4")
	elapsed := time.Since(started)
	if !strings.Contains(res.Error, "所有服务均失败") {
		t.Fatalf("expected failure result, got: %+v", res)
	}
	if elapsed > 300*time.Millisecond {
		t.Fatalf("retry budget should cap total attempts, elapsed: %v", elapsed)
	}
}

func TestFetchGeoProvidersRetryFallsThroughInBudget(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Write([]byte(`{"status":"success","query":"9.9.9.9"}`))
	}))
	defer slow.Close()
	ok := okServer(t, `{"status":"success","query":"9.9.9.9"}`)
	defer ok.Close()
	providers := []provider{
		{name: "slow", url: slow.URL, parse: parseIPAPI},
		{name: "ok", url: ok.URL, parse: parseIPAPI},
	}
	bad := failTransport{err: errors.New("forced")}
	res := fetchGeoProviders(providers, []*http.Client{
		{Transport: bad},
		http.DefaultClient,
	}, time.Second, time.Second, "tcp4")
	if res.Error != "" || res.IP != "9.9.9.9" {
		t.Fatalf("expected retry within budget to reach ok provider, got: %+v", res)
	}
}
