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

var errNoIP = errors.New("响应中缺少 IP")

type provider struct {
	name  string
	url   string
	parse func([]byte) (*geoResult, error)
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
			url:   "http://ip-api.com/json/{ip}?fields=status,message,country,regionName,city,isp,query&lang=zh-CN",
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

func providersFor(queryIP string) []provider {
	var out []provider
	for _, p := range defaultProviders() {
		if strings.Contains(p.url, "{ip}") || queryIP == "" {
			p.url = strings.ReplaceAll(p.url, "{ip}", queryIP)
			out = append(out, p)
		}
	}
	return out
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

func fetchPublicIP(providers []provider, client *http.Client, timeout time.Duration) (*geoResult, error) {
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
				err = errNoIP
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

func fetchGeo(client *http.Client, timeout time.Duration, network, queryIP string) *geoResult {
	family := familyOf(network)
	res, err := fetchPublicIP(providersFor(queryIP), client, timeout)
	if err != nil {
		return &geoResult{Family: family, Error: fmt.Sprintf("%s 所有服务均失败:\n  %s", family, err)}
	}
	res.Family = family
	return res
}
