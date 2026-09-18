package main

import (
	"errors"
	"strings"
	"testing"
)

func TestFamilyOfIP(t *testing.T) {
	cases := map[string]string{
		"8.8.8.8":              "IPv4",
		"192.168.1.1":          "IPv4",
		"2001:4860:4860::8888": "IPv6",
		"::ffff:0.0.0.0":       "IPv4",
		"":                     "IPv4",
	}
	for ip, want := range cases {
		if got := familyOfIP(ip); got != want {
			t.Fatalf("familyOfIP(%q) = %s, want %s", ip, got, want)
		}
	}
}

func TestDisplayWidth(t *testing.T) {
	if got := displayWidth("本机 IPv6"); got != 9 {
		t.Fatalf("displayWidth(本机 IPv6) = %d, want 10", got)
	}
	if got := displayWidth("DNS IPv4:"); got != 9 {
		t.Fatalf("displayWidth(DNS IPv4:) = %d, want 9", got)
	}
}

func TestPadLabel(t *testing.T) {
	padded := padLabel("IP:")
	if len(padded) != 18 {
		t.Fatalf("expected 18 columns, got %q (%d runes)", padded, len([]rune(padded)))
	}
}

func TestPadMinWidth(t *testing.T) {
	label := strings.Repeat("本", 20)
	long := padLabel(label)
	if !strings.HasPrefix(long, label) {
		t.Fatal("padding shouldn't truncate the label")
	}
	if !strings.HasSuffix(long, " ") {
		t.Fatal("expected at least one trailing space when label exceeds column width")
	}
}

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		res  *result
		want int
	}{
		{"both public fail", &result{
			PublicIPv4: &geoResult{Error: "fail"},
			PublicIPv6: &geoResult{Error: "fail"},
		}, 1},
		{"v6 missing", &result{
			PublicIPv4: &geoResult{Error: "fail"},
		}, 1},
		{"partial success", &result{
			PublicIPv4: &geoResult{IP: "1.2.3.4"},
			PublicIPv6: &geoResult{Error: "fail"},
		}, 0},
		{"full success", &result{
			PublicIPv4: &geoResult{IP: "1.2.3.4"},
			PublicIPv6: &geoResult{IP: "::1"},
		}, 0},
	}
	for _, c := range cases {
		if got := exitCode(c.res); got != c.want {
			t.Fatalf("%s: exitCode = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestExitCodeIgnoresDNSErrors(t *testing.T) {
	res := &result{
		DNSError:   "读取系统 DNS 配置失败: 未解析到任何 DNS 服务器",
		PublicIPv4: &geoResult{IP: "1.2.3.4"},
	}
	if got := exitCode(res); got != 0 {
		t.Fatalf("DNS failure must not change exit code, got %d", got)
	}
}

func TestParseIPIPNetTimeStrings(t *testing.T) {
	data := []byte("当前 IP：240e:390:621f:7ad0:9926:714b:94c:e11a 来自于：中国 浙江 绍兴 电信 更新于 2026年9月18日 11:54:22")
	info, err := parseIPIPNet(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.IP != "240e:390:621f:7ad0:9926:714b:94c:e11a" {
		t.Fatalf("IP = %s", info.IP)
	}
	if len(info.Fields) == 0 {
		t.Fatal("expected location fields")
	}
}

func TestMarkIPAPIServiceFailure(t *testing.T) {
	original := geoAPIServiceHealthy.Load()
	defer func() { geoAPIServiceHealthy.Store(original) }()

	markIPAPIServiceFailure()
	if geoAPIServiceHealthy.Load() {
		t.Fatal("expected unhealthy after markIPAPIServiceFailure")
	}
}

func TestRespErrIsLimited(t *testing.T) {
	cases := map[string]bool{
		"HTTP 429":                            true,
		`Get "http://ip-api.com/x": HTTP 429`: true,
		"The requested resource requires an authentication key":               true,
		"Get \"http://ip-api.com/x\": dial tcp4 208.95.112.1:80: i/o timeout": false,
		"服务返回失败: private range":                                               false,
	}
	for msg, want := range cases {
		if got := respErrIsLimited(nil); got != false {
			t.Fatal("nil error should not be limited")
		}
		if got := respErrIsLimited(errors.New(msg)); got != want {
			t.Fatalf("respErrIsLimited(%q) = %v, want %v", msg, got, want)
		}
	}
}
