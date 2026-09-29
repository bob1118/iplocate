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

var errBudgetExhausted = errors.New("重试预算已用尽")

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

// fetchPublicIP 并发请求所有服务，首个成功的结果即胜出，并取消其余仍在飞的请求。
//
// 隐私取舍：这会让**全部**服务都收到出口 IP（见 README）。收益是任一快速服务
// 立刻决定结果，不必等最慢的服务耗尽单次预算。
//
// 失败路径会排空所有结果，并按 providers 声明顺序拼接错误，保证报错可复现；
// 成功路径不等待其余服务，因此返回哪个服务取决于响应速度。
func fetchPublicIP(providers []provider, client *http.Client, timeout time.Duration, deadline time.Time) (*geoResult, error) {
	if len(providers) == 0 {
		return nil, errBudgetExhausted
	}

	base, cancelAll := context.WithCancel(context.Background())
	defer cancelAll()

	type outcome struct {
		index int
		info  *geoResult
		err   error
	}
	// 容量等于服务数：成功路径提前返回后，剩余 goroutine 仍能无阻塞写入，不会泄漏。
	ch := make(chan outcome, len(providers))
	for i, p := range providers {
		go func(i int, p provider) {
			info, err := fetchOneProvider(base, client, p, timeout, deadline)
			ch <- outcome{i, info, err}
		}(i, p)
	}

	failures := make([]outcome, len(providers))
	for range providers {
		got := <-ch
		if got.info != nil {
			cancelAll()
			return got.info, nil
		}
		failures[got.index] = got
	}
	errs := make([]string, 0, len(failures))
	for _, f := range failures {
		errs = append(errs, fmt.Sprintf("%s: %v", providers[f.index].name, f.err))
	}
	return nil, errors.New(strings.Join(errs, "\n  "))
}

// fetchOneProvider 在单次尝试的预算内抓取并解析单个服务，失败时返回原因。
func fetchOneProvider(parent context.Context, client *http.Client, p provider, timeout time.Duration, deadline time.Time) (*geoResult, error) {
	attempt := timeout
	if !deadline.IsZero() {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, errBudgetExhausted
		}
		if remaining < attempt {
			attempt = remaining
		}
	}
	ctx, cancel := context.WithTimeout(parent, attempt)
	defer cancel()

	data, err := httpGet(ctx, client, p.url)
	if err != nil {
		if p.name == ipAPIName && respErrIsLimited(err) {
			markIPAPIServiceFailure()
		}
		return nil, err
	}
	info, perr := p.parse(data)
	switch {
	case perr != nil:
		if p.name == ipAPIName && respErrIsLimited(perr) {
			markIPAPIServiceFailure()
		}
		return nil, perr
	case info == nil || info.IP == "":
		return nil, errNoIP
	}
	info.Source = p.name
	info.Raw = strings.TrimSpace(string(data))
	return info, nil
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
		res, err := fetchPublicIP(providers, client, attempt, deadline)
		if err == nil {
			res.Family = family
			return res
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errBudgetExhausted
	}
	return &geoResult{Family: family, Error: fmt.Sprintf("%s 所有服务均失败:\n  %s", family, lastErr)}
}
