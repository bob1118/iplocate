package main

import (
	"net"
	"testing"
)

func TestProbeLocalKeepsIPv6AddressWhenTCPProbeFails(t *testing.T) {
	oldLocalIP, oldNetworkAvailable := localIPForProbe, networkAvailableForProbe
	defer func() {
		localIPForProbe = oldLocalIP
		networkAvailableForProbe = oldNetworkAvailable
	}()

	var lookedUp []string
	localIPForProbe = func(network string) (net.IP, string, error) {
		lookedUp = append(lookedUp, network)
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
	if len(lookedUp) != 2 || lookedUp[0] != "tcp4" || lookedUp[1] != "tcp6" {
		t.Fatalf("local IP lookups = %v, want tcp4 then tcp6", lookedUp)
	}
}
