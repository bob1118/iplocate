package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

type geoResult struct {
	IP     string  `json:"ip,omitempty"`
	Family string  `json:"family,omitempty"`
	Source string  `json:"source,omitempty"`
	Error  string  `json:"error,omitempty"`
	Raw    string  `json:"raw,omitempty"`
	Fields []field `json:"fields,omitempty"`
}

type result struct {
	LocalIPv4  string     `json:"local_ipv4,omitempty"`
	LocalIPv6  string     `json:"local_ipv6,omitempty"`
	Interface  string     `json:"interface,omitempty"`
	PublicIPv4 *geoResult `json:"public_ipv4"`
	PublicIPv6 *geoResult `json:"public_ipv6"`
}

var fieldLabels = map[string]string{
	"status":       "状态",
	"message":      "消息",
	"query":        "查询 IP",
	"ip":           "IP 地址",
	"country":      "国家/地区",
	"country_name": "国家/地区",
	"countryCode":  "国家代码",
	"regionName":   "省/州",
	"region":       "省/州",
	"region_code":  "区域代码",
	"city":         "城市",
	"isp":          "运营商",
	"org":          "组织",
	"as":           "AS 号",
	"hostname":     "主机名",
	"loc":          "经纬度",
	"latitude":     "纬度",
	"longitude":    "经度",
	"timezone":     "时区",
	"utc_offset":   "UTC 偏移",
	"postal":       "邮编",
}

func main() {
	jsonOut := flag.Bool("json", false, "以 JSON 格式输出")
	timeout := flag.Duration("timeout", 3*time.Second, "单个公网 IP 服务请求超时时间")
	flag.Parse()

	os.Exit(run(*jsonOut, *timeout))
}

func run(jsonOut bool, timeout time.Duration) int {
	v6OK := networkAvailable("tcp6")

	res := &result{}
	if ip4, iface, err := localIP("tcp4"); err != nil {
		fmt.Fprintf(os.Stderr, "警告: %v\n", err)
	} else {
		res.LocalIPv4 = ip4.String()
		res.Interface = iface
	}
	if v6OK {
		if ip6, iface, err := localIP("tcp6"); err == nil {
			res.LocalIPv6 = ip6.String()
			if res.Interface == "" {
				res.Interface = iface
			}
		}
	}

	res.PublicIPv4 = fetchGeo(timeout, "tcp4")
	if v6OK {
		res.PublicIPv6 = fetchGeo(timeout, "tcp6")
	} else {
		fmt.Fprintln(os.Stderr, "提示: 未检测到可用的 IPv6 网络，已跳过 IPv6 查询")
	}

	printResult(res, jsonOut)

	if res.PublicIPv4.Error != "" && (res.PublicIPv6 == nil || res.PublicIPv6.Error != "") {
		return 1
	}
	return 0
}

func fetchGeo(timeout time.Duration, network string) *geoResult {
	info, err := fetchPublicIP(defaultProviders(), timeout, network)
	if err != nil {
		return &geoResult{Family: familyOf(network), Error: err.Error()}
	}
	return &geoResult{
		IP:     info.IP,
		Family: info.Family,
		Source: info.Source,
		Raw:    info.Raw,
		Fields: info.Fields,
	}
}

func printResult(res *result, jsonOut bool) {
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return
	}

	locLabel := "本机 IPv4"
	if res.Interface != "" {
		locLabel += fmt.Sprintf(" (%s)", res.Interface)
	}
	if res.LocalIPv4 != "" {
		fmt.Printf("%s%s\n", padLabel(locLabel+":"), res.LocalIPv4)
	}
	if res.LocalIPv6 != "" {
		fmt.Printf("%s%s\n", padLabel("本机 IPv6 ("+res.Interface+"):"), res.LocalIPv6)
	}
	fmt.Println()
	printGeo(res.PublicIPv4, "公网 IPv4")
	if res.PublicIPv6 != nil {
		fmt.Println()
		printGeo(res.PublicIPv6, "公网 IPv6")
	} else {
		fmt.Printf("%s%s\n", padLabel("公网 IPv6:"), "（未检测到 IPv6 网络）")
	}
}

func printGeo(g *geoResult, label string) {
	if g == nil {
		fmt.Printf("%s%s\n", padLabel(label+":"), "（不可用）")
		return
	}
	if g.Error != "" {
		fmt.Printf("%s%s\n", padLabel(label+":"), "（失败）")
		fmt.Fprintf(os.Stderr, "%s 错误详情:\n%s\n", label, g.Error)
		return
	}
	fmt.Printf("%s%s\n", padLabel(label+":"), g.IP)
	fmt.Printf("%s%s\n", padLabel("数据源:"), g.Source)
	printFields(g.Fields)
}

func printFields(fields []field) {
	fmt.Println(strings.Repeat("─", 44))
	for _, f := range fields {
		name := f.Key
		if zh, ok := fieldLabels[f.Key]; ok {
			name = zh
		}
		fmt.Printf("%s%s\n", pad(name+":", 22), f.Value)
	}
	fmt.Println(strings.Repeat("─", 44))
}

func pad(s string, width int) string {
	pad := width - displayWidth(s)
	if pad < 1 {
		pad = 1
	}
	return s + strings.Repeat(" ", pad)
}

func padLabel(s string) string {
	return pad(s, 18)
}

func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		switch {
		case r >= 0x1100 && r <= 0x115F,
			r >= 0x2E80 && r <= 0xA4CF && r != 0x303F,
			r >= 0xAC00 && r <= 0xD7A3,
			r >= 0xF900 && r <= 0xFAFF,
			r >= 0xFE30 && r <= 0xFE6F,
			r >= 0xFF00 && r <= 0xFF60,
			r >= 0xFFE0 && r <= 0xFFE6:
			w += 2
		default:
			w++
		}
	}
	return w
}
