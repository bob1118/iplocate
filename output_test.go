package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func captureStdoutStderr(t *testing.T, fn func()) (string, string) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}
	os.Stdout, os.Stderr = outW, errW
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()

	fn()
	_ = outW.Close()
	_ = errW.Close()
	out, _ := io.ReadAll(outR)
	errOut, _ := io.ReadAll(errR)
	return string(out), string(errOut)
}

func requireContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected output to contain %q, got:\n%s", needle, haystack)
	}
}

func requireNotContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Fatalf("expected output not to contain %q, got:\n%s", needle, haystack)
	}
}

func requireOutputOrder(t *testing.T, haystack string, labels []string) {
	t.Helper()
	pos := -1
	for _, label := range labels {
		next := strings.Index(haystack, label)
		if next < 0 {
			t.Fatalf("expected output to contain %q, got:\n%s", label, haystack)
		}
		if next < pos {
			t.Fatalf("labels out of order in:\n%s", haystack)
		}
		pos = next
	}
}

func TestPrintTextIPv4Groups(t *testing.T) {
	res := &result{
		LocalIPv4:   "192.168.1.10",
		InterfaceV4: "Ethernet",
		PublicIPv4: &geoResult{
			IP:     "1.2.3.4",
			Source: "test",
			Fields: []field{{Key: "country", Value: "中国"}},
		},
	}
	out, errOut := captureStdoutStderr(t, func() { printText(res) })
	requireContains(t, out, "本机 IPv4")
	requireContains(t, out, "公网 IPv4")
	requireContains(t, out, "DNS IPv4")
	requireNotContains(t, out, "IPv6")
	if errOut != "" {
		t.Fatalf("expected empty stderr, got:\n%s", errOut)
	}
}

func TestPrintTextIPv4AndIPv6Order(t *testing.T) {
	res := &result{
		LocalIPv4:   "192.168.1.10",
		InterfaceV4: "Ethernet",
		PublicIPv4:  &geoResult{IP: "1.2.3.4", Source: "test"},
		LocalIPv6:   "2001:db8::1",
		InterfaceV6: "Ethernet",
		PublicIPv6:  &geoResult{IP: "2001:db8::2", Source: "test"},
	}
	out, _ := captureStdoutStderr(t, func() { printText(res) })
	requireOutputOrder(t, out, []string{"本机 IPv4", "公网 IPv4", "本机 IPv6", "公网 IPv6", "DNS IPv4", "DNS IPv6"})
}

func TestPrintTextSkipsIPv6WithoutLocalOrDNSError(t *testing.T) {
	res := &result{
		LocalIPv4:  "192.168.1.10",
		PublicIPv4: &geoResult{IP: "1.2.3.4", Source: "test"},
	}
	out, _ := captureStdoutStderr(t, func() { printText(res) })
	requireNotContains(t, out, "IPv6")
}

func TestPrintTextDNSFailureKeepsIPv6Section(t *testing.T) {
	res := &result{
		LocalIPv4:  "192.168.1.10",
		PublicIPv4: &geoResult{IP: "1.2.3.4", Source: "test"},
		DNSError:   "boom",
	}
	out, errOut := captureStdoutStderr(t, func() { printText(res) })
	requireContains(t, out, "DNS IPv4")
	requireContains(t, out, "DNS IPv6")
	requireContains(t, errOut, "boom")
}

func TestPrintTextHandlesNilPublicResults(t *testing.T) {
	res := &result{LocalIPv4: "192.168.1.10"}
	out, _ := captureStdoutStderr(t, func() { printText(res) })
	requireContains(t, out, "公网 IPv4:")
	requireContains(t, out, "（不可用）")
	if exitCode(res) != 1 {
		t.Fatalf("nil public IPv4 must count as a failure, got exit code %d", exitCode(res))
	}
}

func TestPrintJSONPublicIPv6Null(t *testing.T) {
	res := &result{
		LocalIPv4:  "192.168.1.10",
		PublicIPv4: &geoResult{IP: "1.2.3.4", Source: "test"},
	}
	out, _ := captureStdoutStderr(t, func() { printJSON(res) })
	requireContains(t, out, `"public_ipv6": null`)
}

func TestPrintJSONIncludesIPv6ProbeResult(t *testing.T) {
	reachable := false
	out, _ := captureStdoutStderr(t, func() {
		printJSON(&result{IPv6ProbeReachable: &reachable})
	})
	requireContains(t, out, `"ipv6_probe_reachable": false`)
}

func TestPrintTextExplainsSkippedIPv6Query(t *testing.T) {
	reachable := false
	out, _ := captureStdoutStderr(t, func() {
		printText(&result{
			LocalIPv6:          "2001:db8::10",
			IPv6ProbeReachable: &reachable,
		})
	})
	requireContains(t, out, "IPv6 TCP 探测")
	requireContains(t, out, "公网 IPv6:")
	requireContains(t, out, "连通性探测未通过")
}
