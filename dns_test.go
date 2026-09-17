package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseResolvConfDNS(t *testing.T) {
	data := []byte("# comment\nnameserver 8.8.8.8\nnameserver 2001:4860:4860::8888\nnameserver 8.8.8.8\n")
	want := []string{"8.8.8.8", "2001:4860:4860::8888"}
	got := parseResolvConfDNS(data)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("DNS = %v, want %v", got, want)
	}
}

func TestParseIPConfigDNS(t *testing.T) {
	data := []byte("Ethernet adapter active:\r\n    Connection-specific DNS Suffix  . :\r\n    IPv4 Address. . . . . . . . . . . : 10.0.0.1\r\n    DNS Servers . . . . . . . . . . : 10.0.0.1\r\n                                      DoH: https://dns.example/dns-query\r\n                                      2001:4860:4860::8888\r\n    NetBIOS over Tcpip. . . . . . . : Enabled\r\n\r\nEthernet adapter other:\r\n    IPv4 Address. . . . . . . . . . . : 192.168.1.10\r\n    DNS Servers . . . . . . . . . . : 1.1.1.1\r\n")
	want := []string{"10.0.0.1", "2001:4860:4860::8888"}
	got := parseIPConfigDNSForIPs(data, []string{"10.0.0.1"})
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("DNS = %v, want %v", got, want)
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

func TestPrimaryDNS(t *testing.T) {
	got := primaryDNS([]dnsConfig{
		{IP: "10.0.0.1", Network: "IPv4"},
		{IP: "8.8.8.8", Network: "IPv4"},
		{IP: "2001:4860:4860::8888", Network: "IPv6"},
	})
	if len(got) != 2 || got[0].IP != "10.0.0.1" || got[1].IP != "2001:4860:4860::8888" {
		t.Fatalf("unexpected primary DNS: %+v", got)
	}
}
