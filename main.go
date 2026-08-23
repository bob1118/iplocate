package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	jsonOut := flag.Bool("json", false, "以 JSON 格式输出")
	timeout := flag.Duration("timeout", 3*time.Second, "单个公网 IP 服务请求超时时间")
	flag.Parse()

	os.Exit(run(*jsonOut, *timeout))
}

func run(jsonOut bool, timeout time.Duration) int {
	res := &result{}

	if ip4, iface, err := localIP("tcp4"); err != nil {
		fmt.Fprintf(os.Stderr, "警告: %v\n", err)
	} else {
		res.LocalIPv4, res.InterfaceV4 = ip4.String(), iface
	}

	v6OK := networkAvailable("tcp6")
	if v6OK {
		if ip6, iface, err := localIP("tcp6"); err == nil {
			res.LocalIPv6, res.InterfaceV6 = ip6.String(), iface
		}
	} else {
		fmt.Fprintln(os.Stderr, "提示: 未检测到可用的 IPv6 网络，已跳过 IPv6 查询")
	}

	res.PublicIPv4 = fetchGeo(clientV4, timeout, "tcp4")
	if v6OK {
		res.PublicIPv6 = fetchGeo(clientV6, timeout, "tcp6")
	}

	printResult(res, jsonOut)

	if res.PublicIPv4.Error != "" && (res.PublicIPv6 == nil || res.PublicIPv6.Error != "") {
		return 1
	}
	return 0
}

func fetchGeo(client *http.Client, timeout time.Duration, network string) *geoResult {
	family := familyOf(network)
	info, err := fetchPublicIP(defaultProviders(), client, timeout)
	if err != nil {
		return &geoResult{Family: family, Error: fmt.Sprintf("%s 所有服务均失败:\n  %s", family, err)}
	}
	return &geoResult{
		IP:     info.IP,
		Family: family,
		Source: info.Source,
		Raw:    info.Raw,
		Fields: info.Fields,
	}
}
