package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"time"
)

const (
	labelWidth = 16
	labelPad   = "                        "
)

var fieldLabels = map[string]string{
	"ip":            "公网 IP",
	"query":         "公网 IP",
	"status":        "状态",
	"message":       "消息",
	"continent":     "大洲",
	"country":       "国家",
	"country_name":  "国家",
	"countryCode":   "国家代码",
	"country_code":  "国家代码",
	"regionName":    "省份",
	"region":        "地区",
	"city":          "城市",
	"district":      "区县",
	"zip":           "邮编",
	"postal":        "邮编",
	"lat":           "纬度",
	"latitude":      "纬度",
	"lon":           "经度",
	"longitude":     "经度",
	"timezone":      "时区",
	"offset":        "时区偏移",
	"utc_offset":    "时区偏移",
	"currency":      "货币",
	"currency_name": "货币",
	"isp":           "运营商",
	"org":           "组织",
	"as":            "自治域",
	"asn":           "自治域",
	"hostname":      "主机名",
	"mobile":        "移动网络",
	"proxy":         "代理",
	"hosting":       "托管主机",
}

type result struct {
	LocalV4 net.IP
	IfaceV4 string
	LocalV6 net.IP
	IfaceV6 string
	Geo     *GeoInfo
}

func main() {
	jsonOut := flag.Bool("json", false, "以 JSON 格式输出")
	timeout := flag.Duration("timeout", 3*time.Second, "单个公网 IP 服务请求超时时间")
	flag.Parse()

	res := &result{}
	if v4, v6, err := localIPs(); err != nil {
		fmt.Fprintf(os.Stderr, "警告: %v\n", err)
	} else {
		res.LocalV4, res.IfaceV4 = v4, ifaceNameFor(v4)
		res.LocalV6, res.IfaceV6 = v6, ifaceNameFor(v6)
	}

	geo, err := fetchPublicIP(defaultProviders(), *timeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
	res.Geo = geo

	printResult(res, *jsonOut)
}

func printResult(res *result, jsonOut bool) {
	if jsonOut {
		writeJSON(res)
		return
	}
	printLocal(res)
	printGeo(res.Geo)
}

func printLocal(res *result) {
	if res.LocalV4 == nil && res.LocalV6 == nil {
		return
	}
	if res.LocalV4 != nil {
		printLine(localLabel("本机 IP", res.IfaceV4), res.LocalV4.String())
	}
	if res.LocalV6 != nil {
		printLine(localLabel("本机 IPv6", res.IfaceV6), res.LocalV6.String())
	}
}

func printGeo(g *GeoInfo) {
	if g == nil {
		return
	}
	printLine("公网 IP:", g.IP)
	printLine("数据源:", g.Source)
	for _, f := range g.Fields {
		if f.Key == "ip" || f.Key == "query" {
			continue
		}
		printLine(fieldLabel(f.Key)+":", f.Value)
	}
}

func writeJSON(res *result) {
	out := make(map[string]any)
	setLocal := func(ipKey, ifaceKey string, ip net.IP, iface string) {
		if ip == nil {
			return
		}
		out[ipKey] = ip.String()
		if iface != "" {
			out[ifaceKey] = iface
		}
	}
	setLocal("local_ipv4", "interface_v4", res.LocalV4, res.IfaceV4)
	setLocal("local_ipv6", "interface_v6", res.LocalV6, res.IfaceV6)
	if res.Geo != nil {
		for _, f := range res.Geo.Fields {
			out[f.Key] = f.Value
		}
		out["source"] = res.Geo.Source
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}

func fieldLabel(key string) string {
	if zh, ok := fieldLabels[key]; ok {
		return zh
	}
	return key
}

func localLabel(base, iface string) string {
	if iface != "" {
		base += fmt.Sprintf(" (%s)", iface)
	}
	return base + ":"
}

func printLine(labelText, value string) {
	fmt.Printf("%s%s\n", padLabel(labelText), value)
}

func padLabel(s string) string {
	pad := labelWidth - displayWidth(s)
	if pad < 1 {
		pad = 1
	}
	return s + labelPad[:pad]
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
