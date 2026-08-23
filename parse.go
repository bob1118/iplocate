package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

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

func lookupField(fields []field, key string) string {
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
	ipipV6Re  = regexp.MustCompile(`(?:[0-9A-Fa-f]{0,4}:){2,7}[0-9A-Fa-f]{0,4}`)
	ipipLocRe = regexp.MustCompile(`来自于\s*[：:]?\s*(.*)`)
)

func parseIPIPNet(data []byte) (*GeoInfo, error) {
	s := strings.TrimSpace(string(data))
	ip := ipipIPRe.FindString(s)
	if ip == "" {
		if c := ipipV6Re.FindString(s); c != "" && net.ParseIP(c) != nil {
			ip = c
		}
	}
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
