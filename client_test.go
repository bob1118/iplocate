package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func bodyServer(t *testing.T, size int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", size)))
	}))
}

func TestHTTPGetRejectsOversizedBody(t *testing.T) {
	big := bodyServer(t, maxBodySize+1)
	defer big.Close()

	_, err := httpGet(context.Background(), big.Client(), big.URL)
	if err == nil {
		t.Fatal("expected error for oversized response")
	}
	if !strings.Contains(err.Error(), "上限") {
		t.Fatalf("expected size-limit error, got: %v", err)
	}
}

func TestHTTPGetAcceptsBodyAtLimit(t *testing.T) {
	atLimit := bodyServer(t, maxBodySize)
	defer atLimit.Close()

	body, err := httpGet(context.Background(), atLimit.Client(), atLimit.URL)
	if err != nil {
		t.Fatalf("body exactly at the limit should pass, got: %v", err)
	}
	if len(body) != maxBodySize {
		t.Fatalf("body length = %d, want %d", len(body), maxBodySize)
	}
}

func TestHTTPGetRejectsNon200(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer bad.Close()

	_, err := httpGet(context.Background(), bad.Client(), bad.URL)
	if err == nil || !strings.Contains(err.Error(), "HTTP 429") {
		t.Fatalf("expected HTTP 429 error, got: %v", err)
	}
	if respErrIsLimited(err) != true {
		t.Fatalf("HTTP 429 should be treated as a limited service, got: %v", err)
	}
}
