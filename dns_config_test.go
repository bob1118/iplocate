package main

import (
	"strings"
	"testing"
)

func TestIsIPConfigSectionHeader(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"Ethernet adapter Ethernet:", true},
		{"\xd2\xd4\xcc\xab\xcd\xf8\xca\xca\xc5\xe4\xc6\xf7 \xd2\xd4\xcc\xab\xcd\xf8:", true},
		{"", false},
		{"   indented but no colon", false},
		{"Windows IP Configuration", false},
		{"nosuffix\r", false},
	}
	for _, c := range cases {
		if got := isIPConfigSectionHeader(c.line); got != c.want {
			t.Fatalf("isIPConfigSectionHeader(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

func TestConfiguredDNSGBKSample(t *testing.T) {
	gbk := []byte("Windows IP \xc5\xe4\xd6\xc3\r\n\r\n" +
		"\xd2\xd4\xcc\xab\xcd\xf8\xca\xca\xc5\xe4\xc6\xf7 \xd2\xd4\xcc\xab\xcd\xf8:\r\n\r\n" +
		"   IPv6 \xb5\xd8\xd6\xb7 . . . . . . . . . . . . : 240e:390:621f:7ad0:9926:714b:94c:e11a(\xb1\xa3\xc1\xf4)\r\n" +
		"   \xc1\xd9\xca\xb1 IPv6 \xb5\xd8\xd6\xb7. . . . . . : 240e:390:621f:7ad0::5(\xb1\xa3\xc1\xf4)\r\n" +
		"   IPv4 \xb5\xd8\xd6\xb7 . . . . . . . . . . . . : 192.168.1.191(\xb1\xa3\xc1\xf4)\r\n" +
		"   DNS \xb7\xfe\xce\xf1\xc6\xf7  . . . . . . . . . . . : 240e:1c:200::1\r\n" +
		"                                       202.96.107.28\r\n\r\n" +
		"Wireless LAN adapter WLAN:\r\n\r\n" +
		"   DNS Servers . . . . . . . . . . : 1.1.1.1\r\n")
	original := readWindowsDNSConfig
	defer func() { readWindowsDNSConfig = original }()
	readWindowsDNSConfig = func() ([]byte, error) { return gbk, nil }

	originalGOOS := runtimeGOOSForTest
	defer func() { runtimeGOOSForTest = originalGOOS }()
	runtimeGOOSForTest = "windows"

	configs, err := configuredDNS("192.168.1.191", "240e:390:621f:7ad0:9926:714b:94c:e11a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]string{
		"202.96.107.28":  "IPv4",
		"240e:1c:200::1": "IPv6",
	}
	if len(configs) != 2 {
		t.Fatalf("configs = %+v, want exactly %v", configs, want)
	}
	for _, config := range configs {
		if want[config.IP] != config.Network {
			t.Fatalf("configs = %+v, network for %s should be %s", configs, config.IP, want[config.IP])
		}
	}
}

func TestConfiguredDNSEmptyWindowsOutput(t *testing.T) {
	original := readWindowsDNSConfig
	defer func() { readWindowsDNSConfig = original }()
	readWindowsDNSConfig = func() ([]byte, error) { return []byte("Ethernet adapter Ethernet:\r\n\r\n"), nil }

	originalGOOS := runtimeGOOSForTest
	defer func() { runtimeGOOSForTest = originalGOOS }()
	runtimeGOOSForTest = "windows"

	_, err := configuredDNS("192.168.1.191")
	if err == nil || !strings.Contains(err.Error(), "未解析到任何 DNS 服务器") {
		t.Fatalf("expected GBK/empty defensive error, got: %v", err)
	}
}

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
	wantConfigs := []dnsConfig{
		{IP: "10.0.0.1", Network: "IPv4"},
		{IP: "2001:4860:4860::8888", Network: "IPv6"},
	}
	gotConfigs := parseIPConfigDNSConfigs(data, map[string]string{"10.0.0.1": "IPv4"})
	if len(gotConfigs) != len(wantConfigs) {
		t.Fatalf("configs = %+v, want %+v", gotConfigs, wantConfigs)
	}
	for i, config := range wantConfigs {
		if gotConfigs[i] != config {
			t.Fatalf("configs = %+v, want %+v", gotConfigs, wantConfigs)
		}
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
