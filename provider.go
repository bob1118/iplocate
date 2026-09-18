package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

var errNoIP = errors.New("响应中缺少 IP")

const ipAPIName = "ip-api.com"

var geoAPIServiceHealthy atomic.Bool

func init() {
	geoAPIServiceHealthy.Store(true)
}

func markIPAPIServiceFailure() {
	geoAPIServiceHealthy.Store(false)
}

type provider struct {
	name  string
	url   string
	parse func([]byte) (*geoResult, error)
}

func defaultProviders() []provider {
	return []provider{
		{
			name:  ipAPIName,
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
			if p.name == ipAPIName && perr != nil {
				markIPAPIServiceFailure()
			}
		}
		if p.name == ipAPIName && respErrIsLimited(err) {
			markIPAPIServiceFailure()
		}
		errs = append(errs, fmt.Sprintf("%s: %v", p.name, err))
	}
	return nil, errors.New(strings.Join(errs, "\n  "))
}

func respErrIsLimited(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "HTTP 429") || strings.Contains(msg, "The requested resource requires an authentication key")
}

func fetchGeo(client *http.Client, timeout time.Duration, network, queryIP string) *geoResult {
	return fetchGeoProviders(providersFor(queryIP), []*http.Client{client}, timeout, timeout, network)
}

func fetchGeoProviders(providers []provider, clients []*http.Client, timeout, budget time.Duration, network string) *geoResult {
	family := familyOf(network)
	deadline := time.Now().Add(budget)
	var lastErr error
	for _, client := range clients {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		attempt := timeout
		if remaining < attempt {
			attempt = remaining
		}
		res, err := fetchPublicIP(providers, client, attempt)
		if err == nil {
			res.Family = family
			return res
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("重试预算已用尽")
	}
	return &geoResult{Family: family, Error: fmt.Sprintf("%s 所有服务均失败:\n  %s", family, lastErr)}
}
