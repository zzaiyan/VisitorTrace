package clientip

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

type Resolver struct {
	trusted []netip.Prefix
}

func New(trustedCIDRs []string) (*Resolver, error) {
	result := &Resolver{}
	for _, value := range trustedCIDRs {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy CIDR %q: %w", value, err)
		}
		result.trusted = append(result.trusted, prefix.Masked())
	}
	return result, nil
}

func (r *Resolver) Resolve(request *http.Request) (netip.Addr, error) {
	direct, err := parseRemoteAddr(request.RemoteAddr)
	if err != nil {
		return netip.Addr{}, err
	}
	if !r.isTrusted(direct) {
		return direct, nil
	}
	forwarded := strings.TrimSpace(request.Header.Get("X-Forwarded-For"))
	if forwarded == "" {
		return direct, nil
	}
	parts := strings.Split(forwarded, ",")
	chain := make([]netip.Addr, 0, len(parts)+1)
	for _, part := range parts {
		address, err := netip.ParseAddr(strings.TrimSpace(part))
		if err != nil {
			return netip.Addr{}, fmt.Errorf("invalid X-Forwarded-For address: %w", err)
		}
		chain = append(chain, address.Unmap())
	}
	chain = append(chain, direct)
	for i := len(chain) - 1; i >= 0; i-- {
		if !r.isTrusted(chain[i]) {
			return chain[i], nil
		}
	}
	return chain[0], nil
}

func (r *Resolver) IsTrustedRemote(remoteAddr string) bool {
	if r == nil {
		return false
	}
	direct, err := parseRemoteAddr(remoteAddr)
	return err == nil && r.isTrusted(direct)
}

func (r *Resolver) isTrusted(address netip.Addr) bool {
	for _, prefix := range r.trusted {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func parseRemoteAddr(value string) (netip.Addr, error) {
	host, _, err := net.SplitHostPort(value)
	if err != nil {
		host = value
	}
	address, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return netip.Addr{}, fmt.Errorf("invalid remote address %q: %w", value, err)
	}
	return address.Unmap(), nil
}
