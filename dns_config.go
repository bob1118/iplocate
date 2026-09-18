package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"unicode"
)

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

var runtimeGOOSForTest = runtime.GOOS

func configuredDNS(activeIPs ...string) ([]dnsConfig, error) {
	switch runtimeGOOSForTest {
	case "windows":
		data, err := readWindowsDNSConfig()
		if err == nil {
			active := make(map[string]string, len(activeIPs))
			for _, ip := range activeIPs {
				active[ip] = familyOfIP(ip)
			}
			configs := parseIPConfigDNSConfigs(data, active)
			if len(configs) == 0 {
				return nil, fmt.Errorf("读取系统 DNS 配置失败: 未解析到任何 DNS 服务器（输出可能为非 UTF-8 编码，请反馈样本）")
			}
			return configs, nil
		}
		return nil, fmt.Errorf("读取系统 DNS 配置失败: %w", err)
	default:
		data, err := os.ReadFile("/etc/resolv.conf")
		if err == nil {
			return parseResolvConfDNSConfigs(data), nil
		}
		return nil, fmt.Errorf("读取系统 DNS 配置失败: %w", err)
	}
}

var readWindowsDNSConfig = func() ([]byte, error) {
	return exec.Command("ipconfig", "/all").Output()
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
		if !sectionHasActiveIP(section, active) {
			return
		}
		for _, ip := range parseIPConfigDNSSection(section) {
			out = appendUniqueDNSConfig(out, dnsConfig{IP: ip, Network: familyOfIP(ip)})
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

func sectionHasActiveIP(section []string, active map[string]string) bool {
	if len(active) == 0 {
		return true
	}
	for _, line := range section {
		for _, found := range ipsFromLine(line) {
			if _, ok := active[found]; ok {
				return true
			}
		}
	}
	return false
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
