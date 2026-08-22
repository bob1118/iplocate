package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const userAgent = "iplocate/1.0"

const maxBodySize = 64 << 10

type field struct {
	Key   string
	Value string
}

type GeoInfo struct {
	IP     string
	Source string
	Fields []field
}

type provider struct {
	name  string
	url   string
	parse func([]byte) (*GeoInfo, error)
}

var httpClient = &http.Client{}

func defaultProviders() []provider {
	return []provider{
		{name: "ip-api.com", url: "http://ip-api.com/json/?lang=zh-CN", parse: parseIPAPI},
		{name: "ipinfo.io", url: "https://ipinfo.io/json", parse: parseIPInfo},
		{name: "ipapi.co", url: "https://ipapi.co/json/", parse: parseIPInfo},
		{name: "myip.ipip.net", url: "https://myip.ipip.net", parse: parseIPIPNet},
	}
}

func httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
}

func fetchPublicIP(providers []provider, timeout time.Duration) (*GeoInfo, error) {
	var errs []string
	for _, p := range providers {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		data, err := httpGet(ctx, p.url)
		cancel()
		if err == nil {
			var info *GeoInfo
			info, err = p.parse(data)
			if err == nil && info.IP == "" {
				err = fmt.Errorf("响应中缺少 IP")
			}
			if err == nil {
				info.Source = p.name
				return info, nil
			}
		}
		errs = append(errs, fmt.Sprintf("%s: %v", p.name, err))
	}
	return nil, fmt.Errorf("所有公网 IP 服务均失败:\n  %s", strings.Join(errs, "\n  "))
}

func flattenJSON(data []byte) ([]field, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("响应不是 JSON 对象")
	}
	var out []field
	if err := flatten(dec, "", '{', &out); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("响应中存在多余内容")
	}
	return out, nil
}

func flatten(dec *json.Decoder, prefix string, open json.Delim, out *[]field) error {
	for i := 0; dec.More(); i++ {
		var name string
		if open == '{' {
			keyTok, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := keyTok.(string)
			if !ok {
				return fmt.Errorf("意外的键类型 %T", keyTok)
			}
			name = joinKey(prefix, key)
		} else {
			name = joinKey(prefix, strconv.Itoa(i))
		}
		valTok, err := dec.Token()
		if err != nil {
			return err
		}
		switch v := valTok.(type) {
		case json.Delim:
			if v != '{' && v != '[' {
				return fmt.Errorf("意外的分隔符 %v", v)
			}
			if err := flatten(dec, name, v, out); err != nil {
				return err
			}
		case string:
			*out = append(*out, field{name, v})
		case json.Number:
			*out = append(*out, field{name, v.String()})
		case bool:
			*out = append(*out, field{name, strconv.FormatBool(v)})
		}
	}
	_, err := dec.Token()
	return err
}

func joinKey(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

func findField(fields []field, key string) string {
	for _, f := range fields {
		if f.Key == key {
			return f.Value
		}
	}
	return ""
}

func parseIPAPI(data []byte) (*GeoInfo, error) {
	fields, err := flattenJSON(data)
	if err != nil {
		return nil, err
	}
	if s := findField(fields, "status"); s != "" && s != "success" {
		msg := findField(fields, "message")
		if msg == "" {
			msg = "未知错误"
		}
		return nil, fmt.Errorf("服务返回失败: %s", msg)
	}
	ip := findField(fields, "query")
	if ip == "" {
		return nil, fmt.Errorf("响应中缺少 IP")
	}
	return &GeoInfo{IP: ip, Fields: fields}, nil
}

func parseIPInfo(data []byte) (*GeoInfo, error) {
	fields, err := flattenJSON(data)
	if err != nil {
		return nil, err
	}
	ip := findField(fields, "ip")
	if ip == "" {
		return nil, fmt.Errorf("响应中缺少 IP")
	}
	return &GeoInfo{IP: ip, Fields: fields}, nil
}

var (
	ipipIPRe  = regexp.MustCompile(`\d{1,3}(?:\.\d{1,3}){3}`)
	ipipLocRe = regexp.MustCompile(`来自于\s*[：:]?\s*(.*)`)
)

func parseIPIPNet(data []byte) (*GeoInfo, error) {
	s := strings.TrimSpace(string(data))
	ip := ipipIPRe.FindString(s)
	if ip == "" {
		return nil, fmt.Errorf("响应中未找到 IP")
	}
	info := &GeoInfo{IP: ip, Fields: []field{{Key: "ip", Value: ip}}}
	if m := ipipLocRe.FindStringSubmatch(s); m != nil {
		parts := strings.FieldsFunc(m[1], unicode.IsSpace)
		get := func(i int) string {
			if i < len(parts) {
				return parts[i]
			}
			return ""
		}
		for _, f := range [4]field{
			{Key: "country", Value: get(0)},
			{Key: "region", Value: get(1)},
			{Key: "city", Value: get(2)},
			{Key: "isp", Value: get(3)},
		} {
			if f.Value != "" {
				info.Fields = append(info.Fields, f)
			}
		}
	}
	return info, nil
}
