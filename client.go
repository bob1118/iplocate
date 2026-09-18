package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const (
	userAgent   = "iplocate/1.0"
	maxBodySize = 64 << 10
	dialTimeout = 2 * time.Second
)

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
