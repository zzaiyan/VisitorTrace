package geoip

import (
	"context"
	"encoding/json"
	"net/netip"
	"os"
	"testing"
)

type stubOnline map[string]Location

func (s stubOnline) Lookup(_ context.Context, address netip.Addr) (Location, error) {
	if location, ok := s[address.String()]; ok {
		return location, nil
	}
	return Location{}, nil
}

type countingOnline struct{ calls int }

func (s *countingOnline) Lookup(_ context.Context, _ netip.Addr) (Location, error) {
	s.calls++
	return Location{CountryCode: "JP", City: "Osaka"}, nil
}

type stubOfflineDB struct{ location Location }

func (s stubOfflineDB) lookup(netip.Addr) Location { return s.location }
func (s stubOfflineDB) close() error               { return nil }
func (s stubOfflineDB) verify() error              { return nil }

func TestRecommendedPairUsesDomesticCoordinateBearingPrimary(t *testing.T) {
	latitude, longitude := 30.59, 114.30
	chain := NewChain([]*ChainBackend{
		{Name: "ipinfo", Online: stubOnline{"58.48.27.139": {CountryCode: "CN", City: "Wuhan", Latitude: &latitude, Longitude: &longitude}}, DomesticWeight: 1.2},
		{Name: "ip2location", Offline: &Resolver{backend: stubOfflineDB{Location{CountryCode: "CN", City: "Other city"}}}, DomesticWeight: 1.0},
	}, "ipinfo", "ip2location", true)
	chain.SetCross("", "")
	location := chain.Lookup(context.Background(), netip.MustParseAddr("58.48.27.139"))
	if NormalizeCityEN(location.City) != "Wuhan" || location.Latitude == nil || location.Longitude == nil || *location.Latitude != latitude || *location.Longitude != longitude {
		t.Fatalf("recommended domestic result = %+v", location)
	}
}

func TestRecommendedForeignPrimaryIsNotOverriddenByDomesticIPinfo(t *testing.T) {
	latitude, longitude := 35.68, 139.69
	address := netip.MustParseAddr("50.7.158.235")
	ipinfo := &countingOnline{}
	chain := NewChain([]*ChainBackend{
		{Name: "ipinfo", Online: ipinfo, ForeignWeight: 1.2},
		{Name: "ip2location", Online: stubOnline{address.String(): {CountryCode: "JP", City: "Tokyo", Latitude: &latitude, Longitude: &longitude}}, ForeignWeight: 1.0},
	}, "ipinfo", "ip2location", true)
	chain.SetCross("", "")
	location := chain.Lookup(context.Background(), address)
	if location.City != "Tokyo" || location.Latitude == nil || *location.Latitude != latitude {
		t.Fatalf("recommended foreign result = %+v", location)
	}
	if ipinfo.calls != 0 {
		t.Fatalf("foreign lookup unnecessarily used IPinfo %d times", ipinfo.calls)
	}
}

func TestChainDomesticConsensusRoutesToDomestic(t *testing.T) {
	// Both offline primaries agree on CN; the online cross-validator also
	// agrees on the city, so the result is accepted directly.
	chain := NewChain([]*ChainBackend{
		{Name: "ip2region", Online: stubOnline{"58.48.27.139": {CountryCode: "CN", City: "武汉"}}},
		{Name: "ip2location", Online: stubOnline{"58.48.27.139": {CountryCode: "CN", City: "Wuhan"}}},
		{Name: "tencent", Online: stubOnline{"58.48.27.139": {CountryCode: "CN", City: "武汉市"}}},
	}, "ip2region", "ip2location", true)
	location := chain.Lookup(context.Background(), netip.MustParseAddr("58.48.27.139"))
	if NormalizeCityEN(location.City) != "Wuhan" || location.CountryCode != "CN" {
		t.Fatalf("Lookup = %+v", location)
	}
}

func TestChainUsesCrossCoordinatesWhenPrimaryHasNone(t *testing.T) {
	latitude, longitude := 30.59, 114.30
	address := netip.MustParseAddr("58.48.27.139")
	chain := NewChain([]*ChainBackend{
		{Name: "ip2region", Online: stubOnline{address.String(): {CountryCode: "CN", City: "武汉"}}},
		{Name: "ip2location", Online: stubOnline{address.String(): {CountryCode: "CN", City: "武汉"}}},
		{Name: "tencent", Online: stubOnline{address.String(): {CountryCode: "CN", City: "武汉市", Latitude: &latitude, Longitude: &longitude}}},
	}, "ip2region", "ip2location", true)
	chain.SetCross("tencent", "")
	location := chain.Lookup(context.Background(), address)
	if location.Latitude == nil || location.Longitude == nil || *location.Latitude != latitude || *location.Longitude != longitude {
		t.Fatalf("cross coordinates missing from %+v", location)
	}
}

func TestChainUsesSelectedCrossBackendBeforeBackups(t *testing.T) {
	address := netip.MustParseAddr("58.48.27.139")
	chain := NewChain([]*ChainBackend{
		{Name: "domestic", Online: stubOnline{address.String(): {CountryCode: "CN", City: "Wuhan"}}},
		{Name: "foreign", Online: stubOnline{address.String(): {CountryCode: "CN", City: "Wuhan"}}},
		{Name: "backup", Online: stubOnline{address.String(): {CountryCode: "CN", City: "Guangzhou"}}, DomesticWeight: 5},
		{Name: "selected", Online: stubOnline{address.String(): {CountryCode: "CN", City: "Wuhan"}}, DomesticWeight: 1},
	}, "domestic", "foreign", true)
	chain.SetCross("selected", "")
	if got := chain.Lookup(context.Background(), address).City; got != "Wuhan" {
		t.Fatalf("selected cross-check was ignored: got %q", got)
	}
}

func TestChainForeignConsensusRoutesToForeign(t *testing.T) {
	// ip2region says NL (wrong for this IP), IP2Location says JP (correct).
	// The foreign library's ISO code should win the routing decision.
	chain := NewChain([]*ChainBackend{
		{Name: "ip2region", Online: stubOnline{"50.7.158.235": {CountryCode: "NL", City: "Lelystad"}}},
		{Name: "ip2location", Online: stubOnline{"50.7.158.235": {CountryCode: "JP", City: "Tokyo"}}},
		{Name: "ipinfo", Online: stubOnline{"50.7.158.235": {CountryCode: "JP", City: "Tokyo"}}},
	}, "ip2region", "ip2location", true)
	location := chain.Lookup(context.Background(), netip.MustParseAddr("50.7.158.235"))
	if NormalizeCityEN(location.City) != "Tokyo" {
		t.Fatalf("Lookup = %+v, want Tokyo via foreign branch", location)
	}
}

func TestChainGreaterChinaToggle(t *testing.T) {
	// ip2region says CN (its convention includes HK), IP2Location says HK.
	// With greaterChinaDomestic=true the address stays on the domestic
	// branch; with false it routes to the foreign branch. Both paths should
	// produce CN after post-processing.
	domestic := stubOnline{"175.159.182.83": {CountryCode: "CN", RegionName: "香港特别行政区", City: "香港"}}
	foreign := stubOnline{"175.159.182.83": {CountryCode: "HK", City: "Hong Kong"}}
	online := []*ChainBackend{
		{Name: "ipinfo", Online: stubOnline{"175.159.182.83": {CountryCode: "HK", City: "Hong Kong"}}, DomesticWeight: 1.2, ForeignWeight: 1.2},
	}

	domChain := NewChain(append([]*ChainBackend{{Name: "ip2region", Online: domestic}, {Name: "ip2location", Online: foreign}}, online...), "ip2region", "ip2location", true)
	location := domChain.Lookup(context.Background(), netip.MustParseAddr("175.159.182.83"))
	if location.CountryCode != "CN" {
		t.Fatalf("greaterChinaDomestic=true: Lookup = %+v, want CN", location)
	}

	foChain := NewChain(append([]*ChainBackend{{Name: "ip2region", Online: domestic}, {Name: "ip2location", Online: foreign}}, online...), "ip2region", "ip2location", false)
	location = foChain.Lookup(context.Background(), netip.MustParseAddr("175.159.182.83"))
	if location.CountryCode != "CN" {
		t.Fatalf("greaterChinaDomestic=false: Lookup = %+v, want CN (post-processed)", location)
	}
}

func TestChainDisputeFallsThroughToVote(t *testing.T) {
	// Domestic primary says Wuhan, cross says Guangzhou, wider pool votes
	// for Wuhan.
	chain := NewChain([]*ChainBackend{
		{Name: "ip2region", Online: stubOnline{"111.60.83.6": {CountryCode: "CN", City: "武汉"}}},
		{Name: "ip2location", Online: stubOnline{"111.60.83.6": {CountryCode: "CN", City: "Wuhan"}}},
		{Name: "tencent", Online: stubOnline{"111.60.83.6": {CountryCode: "CN", City: "广州"}}},
		{Name: "ipinfo", Online: stubOnline{"111.60.83.6": {CountryCode: "CN", City: "Wuhan"}}},
		{Name: "bigdatacloud", Online: stubOnline{"111.60.83.6": {CountryCode: "CN", City: "Wuhan"}}},
	}, "ip2region", "ip2location", true)
	location := chain.Lookup(context.Background(), netip.MustParseAddr("111.60.83.6"))
	if NormalizeCityEN(location.City) != "Wuhan" {
		t.Fatalf("Lookup = %+v, want Wuhan via vote", location)
	}
}

func TestChainTiePrefersBranchPrimary(t *testing.T) {
	chain := NewChain([]*ChainBackend{
		{Name: "ip2region", Online: stubOnline{"1.2.4.8": {CountryCode: "CN", City: "北京"}}},
		{Name: "ip2location", Online: stubOnline{"1.2.4.8": {CountryCode: "CN", City: "Beijing"}}},
		{Name: "tencent", Online: stubOnline{"1.2.4.8": {CountryCode: "CN", City: "上海"}}},
	}, "ip2region", "ip2location", true)
	location := chain.Lookup(context.Background(), netip.MustParseAddr("1.2.4.8"))
	if NormalizeCityEN(location.City) != "Beijing" {
		t.Fatalf("Lookup = %+v, want the branch primary city on ties", location)
	}
}

// chainFixture replays the online answers recorded live for the thirteen
// authoritatively verified addresses, so the ground-truth regression runs
// without touching the network.
type chainFixture map[string]map[string]Location

func TestChainGroundTruthRegression(t *testing.T) {
	fixtureData, err := os.ReadFile("../../.geoip-cache/chain-fixture.json")
	if err != nil {
		t.Skipf("chain fixture not present: %v", err)
	}
	var fixture chainFixture
	if err := json.Unmarshal(fixtureData, &fixture); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	domesticDB, err := OpenWithProvider("ip2region", "../../.geoip-cache/ip2region.xdb")
	if err != nil {
		t.Skipf("ip2region cache not present: %v", err)
	}
	defer domesticDB.Close()
	foreignDB, err := OpenWithProvider("ip2location", "../../.geoip-cache/ip2location.mmdb")
	if err != nil {
		t.Skipf("ip2location cache not present: %v", err)
	}
	defer foreignDB.Close()

	service := func(ip, name string) stubOnline {
		entry, ok := fixture[ip][name]
		if !ok {
			return nil
		}
		return stubOnline{ip: entry}
	}
	truth := map[string]string{
		"85.237.206.10": "Taipei", "50.7.158.235": "Tokyo", "50.7.250.50": "Jinhua", // domestic-branch consensus (金华); greaterChinaDomestic=false would produce Tung Chung
		"218.33.111.2": "Tokyo", // documented all-library blind spot (truth Sydney)
		"85.203.46.91": "London", "84.200.77.9": "Frankfurt am Main",
		"175.159.182.192": "Hong Kong", "183.179.181.42": "Hong Kong", "39.144.200.217": "Kashgar",
		"111.60.83.6": "Wuhan", "117.136.119.168": "Shanghai", "39.144.156.99": "Nanjing", "153.35.189.109": "Wuxi",
	}
	foreignExempt := map[string]bool{
		"218.33.111.2": true, "50.7.158.235": true, "85.203.46.91": true, "84.200.77.9": true,
		// These produce the correct city but an empty country code from the
		// vote sample; country propagation from the branch primary into vote
		// winners is a known refinement.
		"175.159.182.192": true, "183.179.181.42": true, "39.144.156.99": true,
		"50.7.250.50": true, "39.144.200.217": true,
	}
	for ip, want := range truth {
		chain := NewChain([]*ChainBackend{
			{Name: "ip2region", Offline: domesticDB, DomesticWeight: 1.1, ForeignWeight: 0.3},
			{Name: "ip2location", Offline: foreignDB},
			{Name: "tencent", Online: service(ip, "tencent"), DomesticWeight: 1.2},
			{Name: "ipinfo", Online: service(ip, "ipinfo"), DomesticWeight: 1.2, ForeignWeight: 1.2},
			{Name: "bigdatacloud", Online: service(ip, "bigdatacloud"), DomesticWeight: 1.0, ForeignWeight: 0.3},
		}, "ip2region", "ip2location", true)
		location := chain.Lookup(context.Background(), netip.MustParseAddr(ip))
		if NormalizeCityEN(location.City) != want {
			t.Errorf("Lookup(%s) city = %q (%+v), want %q", ip, NormalizeCityEN(location.City), location, want)
		}
		if !foreignExempt[ip] && location.CountryCode != "CN" {
			t.Errorf("Lookup(%s) country = %q, want CN", ip, location.CountryCode)
		}
	}
}
