package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode"
)

const dnsProbeDomain = "example.com"

type dnsResult struct {
	IP        string     `json:"ip"`
	Network   string     `json:"network"`
	Family    string     `json:"family"`
	Reachable bool       `json:"reachable"`
	LatencyMS int64      `json:"latency_ms,omitempty"`
	Ownership *geoResult `json:"ownership,omitempty"`
	Error     string     `json:"error,omitempty"`
}

type dnsProbe func(context.Context, string) (time.Duration, error)
type dnsOwnership func(string, time.Duration) *geoResult

type dnsConfig struct {
	IP      string
	Network string
}

func primaryDNS(servers []dnsConfig) []dnsConfig {
	seen := make(map[string]bool)
	out := make([]dnsConfig, 0, 2)
	for _, server := range servers {
		if seen[server.Network] {
			continue
		}
		seen[server.Network] = true
		out = append(out, server)
	}
	return out
}

func configuredDNS(activeIPs ...string) ([]dnsConfig, error) {
	var data []byte
	var err error
	switch runtime.GOOS {
	case "windows":
		data, err = exec.Command("ipconfig", "/all").Output()
		if err == nil {
			active := make(map[string]string, len(activeIPs))
			for _, ip := range activeIPs {
				active[ip] = familyOfIP(ip)
			}
			return parseIPConfigDNSConfigs(data, active), nil
		}
	default:
		data, err = os.ReadFile("/etc/resolv.conf")
		if err == nil {
			return parseResolvConfDNSConfigs(data), nil
		}
	}
	return nil, fmt.Errorf("读取系统 DNS 配置失败: %w", err)
}

func parseResolvConfDNS(data []byte) []string {
	return dnsConfigIPs(parseResolvConfDNSConfigs(data))
}

func parseResolvConfDNSConfigs(data []byte) []dnsConfig {
	var out []dnsConfig
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.SplitN(line, "#", 2)[0])
		if len(fields) >= 2 && fields[0] == "nameserver" {
			out = appendUniqueDNSConfig(out, dnsConfig{IP: fields[1], Network: familyOfIP(fields[1])})
		}
	}
	return out
}

func parseIPConfigDNS(data []byte) []string {
	return dnsConfigIPs(parseIPConfigDNSConfigs(data, nil))
}

func parseIPConfigDNSForIPs(data []byte, activeIPs []string) []string {
	active := make(map[string]string, len(activeIPs))
	for _, ip := range activeIPs {
		active[ip] = familyOfIP(ip)
	}
	return dnsConfigIPs(parseIPConfigDNSConfigs(data, active))
}

func parseIPConfigDNSConfigs(data []byte, active map[string]string) []dnsConfig {
	var out []dnsConfig
	var section []string
	flush := func() {
		network, ok := sectionNetwork(section, active)
		if !ok {
			return
		}
		for _, ip := range parseIPConfigDNSSection(section) {
			out = appendUniqueDNSConfig(out, dnsConfig{IP: ip, Network: network})
		}
	}
	for _, line := range strings.Split(string(data), "\n") {
		if isIPConfigSectionHeader(line) {
			flush()
			section = []string{line}
			continue
		}
		if len(section) > 0 {
			section = append(section, line)
		}
	}
	flush()
	return out
}

func isIPConfigSectionHeader(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed != "" && !unicode.IsSpace([]rune(line)[0]) && strings.HasSuffix(trimmed, ":")
}

func sectionNetwork(section []string, active map[string]string) (string, bool) {
	if len(active) == 0 {
		return "", true
	}
	for _, line := range section {
		for _, found := range ipsFromLine(line) {
			if network, ok := active[found]; ok {
				return network, true
			}
		}
	}
	return "", false
}

func parseIPConfigDNSSection(section []string) []string {
	var out []string
	inDNSBlock := false
	for _, line := range section {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		isHeader := strings.Contains(lower, "dns") && !strings.Contains(lower, "suffix") && strings.Contains(trimmed, ":")
		if isHeader {
			inDNSBlock = true
			out = appendIPsFromLine(out, trimmed)
			continue
		}
		if !inDNSBlock {
			continue
		}
		if strings.HasPrefix(lower, "doh:") {
			continue
		}
		if ips := ipsFromLine(trimmed); len(ips) > 0 {
			out = appendUniqueIPs(out, ips)
			continue
		}
		if trimmed != "" {
			inDNSBlock = false
		}
	}
	return out
}

func appendUniqueIPs(out, values []string) []string {
	for _, value := range values {
		out = appendUniqueIP(out, value)
	}
	return out
}

func appendUniqueDNSConfig(out []dnsConfig, value dnsConfig) []dnsConfig {
	ip := net.ParseIP(strings.Split(value.IP, "%")[0])
	if ip == nil {
		return out
	}
	value.IP = ip.String()
	for _, existing := range out {
		if existing.IP == value.IP && existing.Network == value.Network {
			return out
		}
	}
	return append(out, value)
}

func dnsConfigIPs(configs []dnsConfig) []string {
	out := make([]string, 0, len(configs))
	for _, config := range configs {
		out = appendUniqueIP(out, config.IP)
	}
	return out
}

func appendIPsFromLine(out []string, line string) []string {
	for _, ip := range ipsFromLine(line) {
		out = appendUniqueIP(out, ip)
	}
	return out
}

func ipsFromLine(line string) []string {
	var out []string
	for _, token := range strings.FieldsFunc(line, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("[](),;", r)
	}) {
		if strings.HasSuffix(token, ".") {
			token = strings.TrimSuffix(token, ".")
		}
		if ip := net.ParseIP(strings.Split(token, "%")[0]); ip != nil {
			out = append(out, ip.String())
		}
	}
	return out
}

func appendUniqueIP(out []string, value string) []string {
	ip := net.ParseIP(strings.Split(value, "%")[0])
	if ip == nil {
		return out
	}
	value = ip.String()
	for _, existing := range out {
		if existing == value {
			return out
		}
	}
	return append(out, value)
}

func inspectDNS(servers []dnsConfig, timeout time.Duration, probe dnsProbe, ownership dnsOwnership) []dnsResult {
	results := make([]dnsResult, 0, len(servers))
	for _, server := range servers {
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
		results = append(results, result)
	}
	return results
}

func familyOfIP(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed != nil && parsed.To4() == nil {
		return "IPv6"
	}
	return "IPv4"
}

func probeConfiguredDNS(ctx context.Context, server string) (time.Duration, error) {
	started := time.Now()
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(dialContext context.Context, _, _ string) (net.Conn, error) {
			dialer := &net.Dialer{Timeout: probeTimeout}
			return dialer.DialContext(dialContext, "udp", net.JoinHostPort(server, "53"))
		},
	}
	_, err := resolver.LookupHost(ctx, dnsProbeDomain)
	return time.Since(started), err
}

func lookupDNSOwnership(ip string, timeout time.Duration) *geoResult {
	return fetchGeo(clientV4, timeout, familyNetwork(ip), ip)
}

func familyNetwork(ip string) string {
	if familyOfIP(ip) == "IPv6" {
		return "tcp6"
	}
	return "tcp4"
}
