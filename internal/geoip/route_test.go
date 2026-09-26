package geoip

import (
	"net/netip"
	"testing"
)

func TestRouterSplitsDomesticAndForeign(t *testing.T) {
	router := NewRouter()
	cases := map[string]bool{
		// CN allocations across registries and generations.
		"58.48.27.139": true, "114.114.114.114": true, "223.104.119.73": true,
		"117.152.87.16": true, "39.144.200.217": true, "1.12.0.0": true,
		// HK / TW / MO are part of the domestic branch.
		"175.159.182.83": true, "158.132.13.222": true, "60.249.229.211": true,
		// Foreign.
		"8.8.8.8": false, "101.110.42.62": false, "85.237.206.10": false,
		"50.7.158.235": false, "218.33.111.2": false,
		// CN IPv6 vs foreign IPv6.
		"2402:4e00::1": true, "2408:8000::1": true, "2606:4700::1111": false, "2001:4860::8888": false,
	}
	for value, want := range cases {
		address := netip.MustParseAddr(value)
		if got := router.IsDomestic(address); got != want {
			t.Errorf("IsDomestic(%s) = %v, want %v", value, got, want)
		}
	}
}

func TestRouterInvalidAddressIsForeign(t *testing.T) {
	if NewRouter().IsDomestic(netip.Addr{}) {
		t.Fatal("invalid address must be foreign")
	}
}
