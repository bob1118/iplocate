package main

import (
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

var warnMu sync.Mutex

var (
	localIPForProbe          = localIP
	networkAvailableForProbe = networkAvailable
	fetchGeoForPublic        = fetchGeo
)

func warnf(format string, args ...any) {
	warnMu.Lock()
	defer warnMu.Unlock()
	fmt.Fprintf(os.Stderr, format, args...)
}

func probeLocal(res *result) bool {
	// 三个探测互不依赖，并发执行以免最坏情况下串行累加探测超时。
	// 实测 UDP 源地址探测近乎瞬时，收益主要体现在 TCP 连通性探测较慢的网络。
	// 探测本身不告警：结果先落到局部变量，join 之后再按固定顺序回填，
	// 保证 stderr 的提示顺序与串行版本完全一致。
	var (
		ip4, ip6    net.IP
		iface4, if6 string
		err4, err6  error
		v6Reachable bool
		wg          sync.WaitGroup
	)
	wg.Add(3)
	go func() {
		defer wg.Done()
		ip4, iface4, err4 = localIPForProbe("tcp4")
	}()
	go func() {
		defer wg.Done()
		ip6, if6, err6 = localIPForProbe("tcp6")
	}()
	go func() {
		defer wg.Done()
		v6Reachable = networkAvailableForProbe("tcp6")
	}()
	wg.Wait()

	if err4 != nil {
		warnf("警告: %v\n", err4)
	} else {
		res.LocalIPv4, res.InterfaceV4 = ip4.String(), iface4
	}

	// Obtain the route-selected local IPv6 independently of the TCP connectivity
	// probe: a blocked probe target must not hide an otherwise usable local route.
	if err6 != nil {
		warnf("警告: %v\n", err6)
	} else {
		res.LocalIPv6, res.InterfaceV6 = ip6.String(), if6
	}

	res.IPv6ProbeReachable = &v6Reachable
	if !v6Reachable {
		warnf("提示: IPv6 TCP 连通性探测未通过，公网 IPv6 查询已跳过\n")
	}
	return v6Reachable
}

func collectPublic(res *result, v6OK bool, timeout time.Duration) {
	res.PublicIPv4 = fetchGeoForPublic(clientV4, timeout, "tcp4", "")
	if !v6OK || res.LocalIPv6 == "" {
		return
	}
	res.PublicIPv6 = fetchGeoForPublic(clientV6, timeout, "tcp6", "")
	if res.PublicIPv6.Error == "" {
		return
	}
	// 直连失败时用 IPv4 客户端显式查询本机 IPv6 的位置，Family 仍标记为 IPv6。
	warnf("提示: 公网 IPv6 直连查询失败，改用本机地址显式查询其位置\n")
	res.PublicIPv6 = fetchGeoForPublic(clientV4, timeout, "tcp6", res.LocalIPv6)
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
	// 拿不到公网 IPv4 结果即视为失败，包括结果为 nil 的情况。
	if res.PublicIPv4 == nil || res.PublicIPv4.Error != "" {
		if res.PublicIPv6 == nil || res.PublicIPv6.Error != "" {
			return 1
		}
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
