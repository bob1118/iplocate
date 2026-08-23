package main

import (
	"flag"
	"fmt"
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
	v6Addr := ""
	if v6OK {
		if ip6, iface, err := localIP("tcp6"); err == nil {
			res.LocalIPv6, res.InterfaceV6 = ip6.String(), iface
			v6Addr = ip6.String()
		}
	} else {
		fmt.Fprintln(os.Stderr, "提示: 未检测到可用的 IPv6 网络，已跳过 IPv6 查询")
	}

	res.PublicIPv4 = fetchGeo(clientV4, timeout, "tcp4", "")
	if v6OK {
		res.PublicIPv6 = fetchGeo(clientV6, timeout, "tcp6", "")
		if res.PublicIPv6.Error != "" && v6Addr != "" {
			fmt.Fprintln(os.Stderr, "提示: 公网 IPv6 直连查询失败，改用本机地址显式查询其位置")
			res.PublicIPv6 = fetchGeo(clientV4, timeout, "tcp6", v6Addr)
		}
	}

	printResult(res, jsonOut)

	if res.PublicIPv4.Error != "" && (res.PublicIPv6 == nil || res.PublicIPv6.Error != "") {
		return 1
	}
	return 0
}
