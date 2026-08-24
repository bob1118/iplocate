package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

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

func printResult(res *result, jsonOut bool) {
	if jsonOut {
		printJSON(res)
		return
	}
	printText(res)
}

func printJSON(res *result) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(res)
}

func printText(res *result) {
	if res.LocalIPv4 != "" {
		line(ifaceLabel("本机 IPv4", res.InterfaceV4), res.LocalIPv4)
	}
	if res.LocalIPv6 != "" {
		line(ifaceLabel("本机 IPv6", res.InterfaceV6), res.LocalIPv6)
	}
	fmt.Println()
	printGeo(res.PublicIPv4, "公网 IPv4")
	if res.PublicIPv6 != nil {
		fmt.Println()
		printGeo(res.PublicIPv6, "公网 IPv6")
	} else {
		line("公网 IPv6:", "（未检测到 IPv6 网络）")
	}
}

func ifaceLabel(base, iface string) string {
	if iface != "" {
		base += fmt.Sprintf(" (%s)", iface)
	}
	return base + ":"
}

func printGeo(g *geoResult, label string) {
	switch {
	case g == nil:
		line(label+":", "（不可用）")
	case g.Error != "":
		line(label+":", "（失败）")
		fmt.Fprintf(os.Stderr, "%s 错误详情:\n%s\n", label, g.Error)
	default:
		line(label+":", g.IP)
		line("数据源:", g.Source)
		printFields(g.Fields)
	}
}

func printFields(fields []field) {
	sep := strings.Repeat("─", 44)
	fmt.Println(sep)
	for _, f := range fields {
		name := f.Key
		if zh, ok := fieldLabels[f.Key]; ok {
			name = zh
		}
		line(name+":", f.Value)
	}
	fmt.Println(sep)
}

func line(label, value string) {
	fmt.Printf("%s%s\n", padLabel(label), value)
}

func padLabel(s string) string {
	return pad(s, 18)
}

func pad(s string, width int) string {
	pad := width - displayWidth(s)
	if pad < 1 {
		pad = 1
	}
	return s + strings.Repeat(" ", pad)
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
