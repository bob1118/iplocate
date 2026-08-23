package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const userAgent = "iplocate/1.0"

type field struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type GeoInfo struct {
	IP     string
	Family string
	Source string
	Raw    string
	Fields []field
}

type provider struct {
	name  string
	url   string
	parse func([]byte) (*GeoInfo, error)
}

func newFamClient(network string) *http.Client {
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
	}
	return &http.Client{Transport: transport}
}

var (
	clientV4 = newFamClient("tcp4")
	clientV6 = newFamClient("tcp6")
)

func defaultProviders() []provider {
	return []provider{
		{
			name:  "ip-api.com",
			url:   "http://ip-api.com/json/?fields=status,message,country,regionName,city,isp,query&lang=zh-CN",
			parse: parseIPAPI,
		},
		{
			name:  "ipinfo.io",
			url:   "https://ipinfo.io/json",
			parse: parseIPInfo,
		},
		{
			name:  "ipapi.co",
			url:   "https://ipapi.co/json/",
			parse: parseIPInfo,
		},
		{
			name:  "myip.ipip.net",
			url:   "https://myip.ipip.net",
			parse: parseIPIPNet,
		},
	}
}

func httpGet(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<10))
}

func fetchPublicIP(providers []provider, timeout time.Duration, network string) (*GeoInfo, error) {
	client := clientV4
	if network == "tcp6" {
		client = clientV6
	}
	var errs []string
	for _, p := range providers {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		data, err := httpGet(ctx, client, p.url)
		cancel()
		if err == nil {
			info, perr := p.parse(data)
			switch {
			case perr != nil:
				err = perr
			case info == nil || info.IP == "":
				err = fmt.Errorf("响应中缺少 IP")
			default:
				info.Source = p.name
				info.Family = familyOf(network)
				info.Raw = strings.TrimSpace(string(data))
				return info, nil
			}
		}
		errs = append(errs, fmt.Sprintf("%s: %v", p.name, err))
	}
	return nil, fmt.Errorf("%s 所有服务均失败:\n  %s", familyOf(network), strings.Join(errs, "\n  "))
}

func lookupField(fields []field, key string) string {
	for _, f := range fields {
		if f.Key == key {
			return f.Value
		}
	}
	return ""
}

func flattenJSON(data []byte) ([]field, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("无效 JSON: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("顶层必须是 JSON 对象")
	}
	var out []field
	if err := flattenObject(dec, "", &out); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("JSON 存在多余内容")
	}
	return out, nil
}

func flattenObject(dec *json.Decoder, prefix string, out *[]field) error {
	for {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		if d, ok := keyTok.(json.Delim); ok && d == '}' {
			return nil
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("意外的对象键类型 %T", keyTok)
		}
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		valTok, err := dec.Token()
		if err != nil {
			return err
		}
		if err := flattenValue(dec, valTok, path, out); err != nil {
			return err
		}
	}
}

func flattenArray(dec *json.Decoder, prefix string, out *[]field) error {
	for i := 0; ; i++ {
		valTok, err := dec.Token()
		if err != nil {
			return err
		}
		if d, ok := valTok.(json.Delim); ok && d == ']' {
			return nil
		}
		if err := flattenValue(dec, valTok, fmt.Sprintf("%s.%d", prefix, i), out); err != nil {
			return err
		}
	}
}

func flattenValue(dec *json.Decoder, tok json.Token, path string, out *[]field) error {
	switch v := tok.(type) {
	case json.Delim:
		switch v {
		case '{':
			return flattenObject(dec, path, out)
		case '[':
			return flattenArray(dec, path, out)
		default:
			return fmt.Errorf("意外分隔符 %q", v)
		}
	case nil:
		return nil
	case json.Number:
		*out = append(*out, field{Key: path, Value: v.String()})
	case string:
		*out = append(*out, field{Key: path, Value: v})
	case bool:
		*out = append(*out, field{Key: path, Value: strconv.FormatBool(v)})
	default:
		return fmt.Errorf("意外的值类型 %T", tok)
	}
	return nil
}

func parseIPAPI(data []byte) (*GeoInfo, error) {
	fields, err := flattenJSON(data)
	if err != nil {
		return nil, err
	}
	if lookupField(fields, "status") != "success" {
		msg := lookupField(fields, "message")
		if msg == "" {
			msg = "未知错误"
		}
		return nil, fmt.Errorf("服务返回失败: %s", msg)
	}
	ip := lookupField(fields, "query")
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
	ip := lookupField(fields, "ip")
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
	info := &GeoInfo{IP: ip}
	if m := ipipLocRe.FindStringSubmatch(s); m != nil {
		fields := strings.FieldsFunc(m[1], unicode.IsSpace)
		add := func(key, val string) {
			if val != "" {
				info.Fields = append(info.Fields, field{Key: key, Value: val})
			}
		}
		add("country", at(fields, 0))
		add("region", at(fields, 1))
		add("city", at(fields, 2))
		add("isp", strings.Join(fields[min(3, len(fields)):], " "))
	}
	return info, nil
}

func at(fields []string, i int) string {
	if i < len(fields) {
		return fields[i]
	}
	return ""
}
