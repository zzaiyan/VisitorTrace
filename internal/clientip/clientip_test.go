package clientip

import (
	"net/http/httptest"
	"testing"
)

func TestResolveIgnoresUntrustedForwardingHeader(t *testing.T) {
	resolver, err := New([]string{"127.0.0.1/32"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	request.Header.Set("X-Forwarded-For", "198.51.100.20")
	got, err := resolver.Resolve(request)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.String() != "192.0.2.10" {
		t.Fatalf("Resolve() = %s", got)
	}
}

func TestResolveUsesTrustedProxyChain(t *testing.T) {
	resolver, err := New([]string{"127.0.0.1/32", "10.0.0.0/8"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("X-Forwarded-For", "198.51.100.20, 10.0.0.4")
	got, err := resolver.Resolve(request)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.String() != "198.51.100.20" {
		t.Fatalf("Resolve() = %s", got)
	}
}
