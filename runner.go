package main

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var warnMu sync.Mutex

func warnf(format string, args ...any) {
	warnMu.Lock()
	defer warnMu.Unlock()
	fmt.Fprintf(os.Stderr, format, args...)
}

func probeLocal(res *result) bool {
	if ip4, iface, err := localIP("tcp4"); err != nil {
		warnf("警告: %v\n", err)
	} else {
		res.LocalIPv4, res.InterfaceV4 = ip4.String(), iface
	}

	v6OK := networkAvailable("tcp6")
	if !v6OK {
		warnf("提示: 未检测到可用的 IPv6 网络，已跳过 IPv6 查询\n")
		return false
	}
	if ip6, iface, err := localIP("tcp6"); err == nil {
		res.LocalIPv6, res.InterfaceV6 = ip6.String(), iface
	}
	return true
}

func collectPublic(res *result, v6OK bool, timeout time.Duration) {
	res.PublicIPv4 = fetchGeo(clientV4, timeout, "tcp4", "")
	if !v6OK || res.LocalIPv6 == "" {
		return
	}
	res.PublicIPv6 = fetchGeo(clientV6, timeout, "tcp6", "")
	if res.PublicIPv6.Error == "" {
		return
	}
	warnf("提示: 公网 IPv6 直连查询失败，改用本机地址显式查询其位置\n")
	res.PublicIPv6 = fetchGeo(clientV4, timeout, "tcp6", res.LocalIPv6)
}

func collectDNS(res *result, timeout time.Duration) {
	if dnsServers, err := configuredDNS(res.LocalIPv4, res.LocalIPv6); err != nil {
		res.DNSError = err.Error()
		warnf("警告: %v\n", err)
	} else {
		res.DNS = inspectDNS(primaryDNS(dnsServers), timeout, probeConfiguredDNS, lookupDNSOwnership)
	}
}

func exitCode(res *result) int {
	if res.PublicIPv4.Error != "" && (res.PublicIPv6 == nil || res.PublicIPv6.Error != "") {
		return 1
	}
	return 0
}

func run(jsonOut bool, timeout time.Duration) int {
	res := &result{}
	v6OK := probeLocal(res)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		collectDNS(res, timeout)
	}()
	go func() {
		defer wg.Done()
		collectPublic(res, v6OK, timeout)
	}()
	wg.Wait()

	printResult(res, jsonOut)
	return exitCode(res)
}
