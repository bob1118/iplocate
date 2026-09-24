package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

const dnsProbeDomain = "example.com"

type dnsResult struct {
	IP string `json:"ip"`
	// Network 是展示用的地址族分组，目前与 Family 相同；Family 为 JSON 兼容保留。
	Network   string     `json:"network"`
	Family    string     `json:"family"`
	Reachable bool       `json:"reachable"`
	LatencyMS int64      `json:"latency_ms,omitempty"`
	Ownership *geoResult `json:"ownership,omitempty"`
	Error     string     `json:"error,omitempty"`
}

type dnsProbe func(context.Context, string) (time.Duration, error)
type dnsOwnership func(string, time.Duration) *geoResult

func inspectDNS(servers []dnsConfig, timeout time.Duration, probe dnsProbe, ownership dnsOwnership) []dnsResult {
	results := make([]dnsResult, len(servers))
	var wg sync.WaitGroup
	for i, server := range servers {
		wg.Add(1)
		go func(i int, server dnsConfig) {
			defer wg.Done()
			results[i] = probeOneDNS(server, timeout, probe, ownership)
		}(i, server)
	}
	wg.Wait()
	return results
}

func probeOneDNS(server dnsConfig, timeout time.Duration, probe dnsProbe, ownership dnsOwnership) dnsResult {
	result := dnsResult{IP: server.IP, Network: server.Network, Family: familyOfIP(server.IP)}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	latency, err := probe(ctx, server.IP)
	cancel()
	if err != nil {
		result.Error = err.Error()
	} else {
		result.Reachable = true
		result.LatencyMS = latency.Milliseconds()
	}
	result.Ownership = ownership(server.IP, timeout)
	return result
}

func dialFamilyFor(server string) string {
	if familyOfIP(server) == "IPv6" {
		return "udp6"
	}
	return "udp4"
}

func probeConfiguredDNS(ctx context.Context, server string) (time.Duration, error) {
	started := time.Now()
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(dialContext context.Context, _, _ string) (net.Conn, error) {
			dialer := &net.Dialer{Timeout: probeTimeout}
			return dialer.DialContext(dialContext, dialFamilyFor(server), net.JoinHostPort(server, "53"))
		},
	}
	_, err := resolver.LookupHost(ctx, dnsProbeDomain)
	return time.Since(started), err
}

func lookupDNSOwnership(ip string, timeout time.Duration) *geoResult {
	if !geoAPIServiceHealthy.Load() {
		return nil
	}
	network := familyNetwork(ip)
	return ensureExplicitIP(fetchGeoProviders(providersFor(ip), ownershipClients(network), timeout, timeout, network), ip)
}

func ensureExplicitIP(res *geoResult, queryIP string) *geoResult {
	if res == nil || queryIP == "" || res.Error != "" {
		return res
	}
	want := net.ParseIP(queryIP)
	got := net.ParseIP(res.IP)
	if want == nil || got == nil || want.Equal(got) {
		return res
	}
	return &geoResult{Family: res.Family, Error: fmt.Sprintf("显式查询 IP 不一致：查询 %s，返回 %s", queryIP, res.IP)}
}

func ownershipClients(network string) []*http.Client {
	if network == "tcp6" {
		return []*http.Client{clientV6, clientV4}
	}
	return []*http.Client{clientV4}
}

func familyOfIP(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed != nil && parsed.To4() == nil {
		return "IPv6"
	}
	return "IPv4"
}

func familyNetwork(ip string) string {
	if familyOfIP(ip) == "IPv6" {
		return "tcp6"
	}
	return "tcp4"
}
