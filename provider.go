package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	userAgent   = "iplocate/1.0"
	maxBodySize = 64 << 10
	dialTimeout = 2 * time.Second
)

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
	dialer := &net.Dialer{Timeout: dialTimeout}
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
	return io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
}

func fetchPublicIP(providers []provider, client *http.Client, timeout time.Duration) (*GeoInfo, error) {
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
				info.Raw = strings.TrimSpace(string(data))
				return info, nil
			}
		}
		errs = append(errs, fmt.Sprintf("%s: %v", p.name, err))
	}
	return nil, errors.New(strings.Join(errs, "\n  "))
}
