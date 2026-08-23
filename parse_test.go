package main

import (
	"reflect"
	"strings"
	"testing"
)

func fieldOf(t *testing.T, info *geoResult, key string) string {
	t.Helper()
	for _, f := range info.Fields {
		if f.Key == key {
			return f.Value
		}
	}
	t.Fatalf("missing field %q in %v", key, info.Fields)
	return ""
}

func hasField(info *geoResult, key string) bool {
	for _, f := range info.Fields {
		if f.Key == key {
			return true
		}
	}
	return false
}

func TestParseIPAPI(t *testing.T) {
	data := []byte(`{"status":"success","country":"中国","regionName":"广东省","city":"深圳","isp":"电信","query":"1.2.3.4"}`)
	info, err := parseIPAPI(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.IP != "1.2.3.4" {
		t.Fatalf("parsed IP incorrectly: %+v", info)
	}
	for key, want := range map[string]string{
		"country":    "中国",
		"regionName": "广东省",
		"city":       "深圳",
		"isp":        "电信",
	} {
		if got := fieldOf(t, info, key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
	if len(info.Fields) != 6 {
		t.Fatalf("expected all 6 returned fields, got %d: %v", len(info.Fields), info.Fields)
	}
}

func TestParseIPAPIFailure(t *testing.T) {
	data := []byte(`{"status":"fail","message":"reserved range"}`)
	info, err := parseIPAPI(data)
	if err == nil {
		t.Fatal("expected error for fail status")
	}
	if !strings.Contains(err.Error(), "reserved range") {
		t.Fatalf("error should contain message, got: %v", err)
	}
	if info != nil {
		t.Fatalf("expected nil info, got: %+v", info)
	}
}

func TestParseIPInfo(t *testing.T) {
	data := []byte(`{"ip":"8.8.8.8","city":"Mountain View","region":"California","country":"US","org":"AS15169 GOOGLE"}`)
	info, err := parseIPInfo(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.IP != "8.8.8.8" {
		t.Fatalf("parsed incorrectly: %+v", info)
	}
	for key, want := range map[string]string{
		"country": "US",
		"region":  "California",
		"city":    "Mountain View",
	} {
		if got := fieldOf(t, info, key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
	if got := fieldOf(t, info, "org"); got != "AS15169 GOOGLE" {
		t.Fatalf("org should be preserved verbatim, got %q", got)
	}
}

func TestParseIPInfoIPAPICoShape(t *testing.T) {
	data := []byte(`{"ip":"1.1.1.1","country_name":"Australia","region":"Queensland","city":"Brisbane","org":"AS13335 CLOUDFLARENET"}`)
	info, err := parseIPInfo(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.IP != "1.1.1.1" {
		t.Fatalf("parsed incorrectly: %+v", info)
	}
	if got := fieldOf(t, info, "country_name"); got != "Australia" {
		t.Fatalf("country_name = %q", got)
	}
	if got := fieldOf(t, info, "org"); got != "AS13335 CLOUDFLARENET" {
		t.Fatalf("org = %q", got)
	}
}

func TestFlattenJSON(t *testing.T) {
	data := []byte(`{"b":true,"n":1.50,"nested":{"x":"1","y":[10,null,"z"]},"a":"first","skip":null}`)
	fields, err := flattenJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []field{
		{Key: "b", Value: "true"},
		{Key: "n", Value: "1.50"},
		{Key: "nested.x", Value: "1"},
		{Key: "nested.y.0", Value: "10"},
		{Key: "nested.y.2", Value: "z"},
		{Key: "a", Value: "first"},
	}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("flatten mismatch:\n got %v\nwant %v", fields, want)
	}
}

func TestFlattenJSONErrors(t *testing.T) {
	for name, data := range map[string][]byte{
		"not-json":         []byte("nope"),
		"top-array":        []byte(`[1,2]`),
		"trailing-garbage": []byte(`{} oops`),
	} {
		if _, err := flattenJSON(data); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

func TestParseIPIPNet(t *testing.T) {
	data := []byte("当前 IP：59.40.1.1  来自于：中国 广东 深圳  电信\n")
	info, err := parseIPIPNet(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.IP != "59.40.1.1" {
		t.Fatalf("parsed incorrectly: %+v", info)
	}
	for key, want := range map[string]string{
		"country": "中国",
		"region":  "广东",
		"city":    "深圳",
		"isp":     "电信",
	} {
		if got := fieldOf(t, info, key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestParseIPIPNetShort(t *testing.T) {
	data := []byte("当前 IP：59.40.1.1　来自于：中国 广东 深圳")
	info, err := parseIPIPNet(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.IP != "59.40.1.1" {
		t.Fatalf("parsed incorrectly: %+v", info)
	}
	if got := fieldOf(t, info, "city"); got != "深圳" {
		t.Fatalf("city = %q", got)
	}
	if hasField(info, "isp") {
		t.Fatalf("isp should be absent when provider omits it: %v", info.Fields)
	}
}

func TestParseIPIPNetV6(t *testing.T) {
	data := []byte("当前 IP：2408:8240:6612:43f4:bcbe:c27b:e3d3:3e11  来自于：中国 浙江 绍兴  联通\n")
	info, err := parseIPIPNet(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.IP != "2408:8240:6612:43f4:bcbe:c27b:e3d3:3e11" {
		t.Fatalf("parsed incorrectly: %+v", info)
	}
	for key, want := range map[string]string{
		"country": "中国",
		"region":  "浙江",
		"city":    "绍兴",
		"isp":     "联通",
	} {
		if got := fieldOf(t, info, key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestParseIPIPNetV6Compressed(t *testing.T) {
	data := []byte("当前 IP：2408:8240::3e11　来自于：中国 浙江 绍兴")
	info, err := parseIPIPNet(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.IP != "2408:8240::3e11" {
		t.Fatalf("parsed incorrectly: %+v", info)
	}
}

func TestParseIPIPNetRejectsNonIP(t *testing.T) {
	data := []byte("时间 12:34:56:78 来自于： nowhere")
	if _, err := parseIPIPNet(data); err == nil {
		t.Fatal("expected error when no valid IP present")
	}
}

func TestParseInvalidData(t *testing.T) {
	for name, tc := range map[string]struct {
		parse func([]byte) (*geoResult, error)
		data  []byte
	}{
		"ipapi-invalid-json": {parseIPAPI, []byte("not json")},
		"ipinfo-empty":       {parseIPInfo, []byte(`{}`)},
		"ipapico-error":      {parseIPInfo, []byte(`{"error":true,"reason":"RateLimited"}`)},
		"ipip-no-ip":         {parseIPIPNet, []byte("no ip here")},
	} {
		info, err := tc.parse(tc.data)
		if err == nil {
			t.Fatalf("%s: expected error", name)
		}
		if info != nil {
			t.Fatalf("%s: expected nil info, got: %+v", name, info)
		}
	}
}
