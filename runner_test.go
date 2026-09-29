package main

import (
	"net"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestProbeLocalKeepsIPv6AddressWhenTCPProbeFails(t *testing.T) {
	oldLocalIP, oldNetworkAvailable := localIPForProbe, networkAvailableForProbe
	defer func() {
		localIPForProbe = oldLocalIP
		networkAvailableForProbe = oldNetworkAvailable
	}()

	// probeLocal 并发调用该钩子，收集必须加锁，否则测试自身就有数据竞争。
	var mu sync.Mutex
	var lookedUp []string
	localIPForProbe = func(network string) (net.IP, string, error) {
		mu.Lock()
		lookedUp = append(lookedUp, network)
		mu.Unlock()
		if network == "tcp6" {
			return net.ParseIP("2001:db8::10"), "Ethernet", nil
		}
		return net.ParseIP("192.0.2.10"), "Ethernet", nil
	}
	probeCalled := false
	networkAvailableForProbe = func(network string) bool {
		probeCalled = true
		if network != "tcp6" {
			t.Fatalf("probe network = %q, want tcp6", network)
		}
		return false
	}

	res := &result{}
	_, _ = captureStdoutStderr(t, func() {
		if probeLocal(res) {
			t.Fatal("probeLocal should report failed IPv6 connectivity")
		}
	})

	if !probeCalled {
		t.Fatal("expected IPv6 connectivity probe to run")
	}
	if res.LocalIPv6 != "2001:db8::10" || res.InterfaceV6 != "Ethernet" {
		t.Fatalf("local IPv6 address should be retained despite failed TCP probe: %+v", res)
	}
	if res.IPv6ProbeReachable == nil || *res.IPv6ProbeReachable {
		t.Fatalf("expected a recorded failed IPv6 probe: %+v", res.IPv6ProbeReachable)
	}
	// 三个探测并发执行，调用顺序不再确定，只断言两个地址族都被探测到。
	mu.Lock()
	defer mu.Unlock()
	seen := map[string]bool{}
	for _, network := range lookedUp {
		seen[network] = true
	}
	if len(lookedUp) != 2 || !seen["tcp4"] || !seen["tcp6"] {
		t.Fatalf("local IP lookups = %v, want both tcp4 and tcp6", lookedUp)
	}
}

type geoCall struct {
	network string
	queryIP string
}

func stubFetchGeo(t *testing.T, answer func(network, queryIP string) *geoResult) *[]geoCall {
	t.Helper()
	original := fetchGeoForPublic
	t.Cleanup(func() { fetchGeoForPublic = original })

	var mu sync.Mutex
	calls := make([]geoCall, 0, 4)
	fetchGeoForPublic = func(client *http.Client, timeout time.Duration, network, queryIP string) *geoResult {
		mu.Lock()
		calls = append(calls, geoCall{network, queryIP})
		mu.Unlock()
		return answer(network, queryIP)
	}
	return &calls
}

func TestCollectPublicIPv6ExplicitFallback(t *testing.T) {
	calls := stubFetchGeo(t, func(network, queryIP string) *geoResult {
		switch {
		case network == "tcp4" && queryIP == "":
			return &geoResult{IP: "1.2.3.4", Family: "IPv4"}
		case network == "tcp6" && queryIP == "":
			return &geoResult{Family: "IPv6", Error: "直连失败"}
		default:
			// 显式回退轮：用 clientV4 带 queryIP 查询，Family 仍为 IPv6。
			return &geoResult{IP: queryIP, Family: "IPv6"}
		}
	})

	res := &result{LocalIPv6: "2001:db8::10"}
	_, _ = captureStdoutStderr(t, func() { collectPublic(res, true, time.Second) })

	if res.PublicIPv4 == nil || res.PublicIPv4.IP != "1.2.3.4" {
		t.Fatalf("unexpected public IPv4: %+v", res.PublicIPv4)
	}
	if res.PublicIPv6 == nil || res.PublicIPv6.IP != "2001:db8::10" || res.PublicIPv6.Family != "IPv6" {
		t.Fatalf("expected explicit-IP fallback retaining IPv6 family: %+v", res.PublicIPv6)
	}
	want := []geoCall{{"tcp4", ""}, {"tcp6", ""}, {"tcp6", "2001:db8::10"}}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("fetch calls = %+v, want %+v", *calls, want)
	}
}

func TestCollectPublicSkipsIPv6Round(t *testing.T) {
	for _, tc := range []struct {
		name      string
		v6OK      bool
		localIPv6 string
	}{
		{"probe failed", false, "2001:db8::10"},
		{"no local ipv6", true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := stubFetchGeo(t, func(network, queryIP string) *geoResult {
				return &geoResult{IP: "1.2.3.4", Family: "IPv4"}
			})
			res := &result{LocalIPv6: tc.localIPv6}
			_, _ = captureStdoutStderr(t, func() { collectPublic(res, tc.v6OK, time.Second) })
			if res.PublicIPv6 != nil {
				t.Fatalf("public IPv6 should not be queried, got: %+v", res.PublicIPv6)
			}
			if len(*calls) != 1 {
				t.Fatalf("only the IPv4 round should run, calls = %+v", *calls)
			}
		})
	}
}
