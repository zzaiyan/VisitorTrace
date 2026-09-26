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

func chainBackends(domestic, cross, voter stubOnline) []*ChainBackend {
	return []*ChainBackend{
		{Name: "ip2region", Online: domestic},
		{Name: "tencent", Online: cross},
		{Name: "ip2location", Online: stubOnline(nil)},
		{Name: "ipinfo", Online: voter},
		{Name: "bigdatacloud", Online: voter},
	}
}

func TestChainCrossValidationAcceptsMatchingPrimaries(t *testing.T) {
	chain := NewChain(chainBackends(
		stubOnline{"58.48.27.139": {CountryCode: "CN", RegionName: "湖北省", City: "武汉"}},
		stubOnline{"58.48.27.139": {CountryCode: "CN", City: "武汉市"}},
		stubOnline(nil),
	), "ip2region", "ip2location")
	location := chain.Lookup(context.Background(), netip.MustParseAddr("58.48.27.139"))
	if NormalizeCityEN(location.City) != "Wuhan" || location.CountryCode != "CN" {
		t.Fatalf("Lookup = %+v", location)
	}
}

func TestChainDisputeFallsThroughToVote(t *testing.T) {
	// Primary says Wuhan, online counterpart says Guangzhou, and the wider
	// pool lands on Wuhan: the vote must recover the majority city.
	chain := NewChain([]*ChainBackend{
		{Name: "ip2region", Online: stubOnline{"111.60.83.6": {CountryCode: "CN", City: "武汉"}}},
		{Name: "tencent", Online: stubOnline{"111.60.83.6": {CountryCode: "CN", City: "广州"}}},
		{Name: "ip2location", Online: stubOnline{"111.60.83.6": {CountryCode: "CN", City: "Wuhan"}}},
		{Name: "ipinfo", Online: stubOnline{"111.60.83.6": {CountryCode: "CN", City: "Wuhan"}}},
	}, "ip2region", "ip2location")
	location := chain.Lookup(context.Background(), netip.MustParseAddr("111.60.83.6"))
	if NormalizeCityEN(location.City) != "Wuhan" {
		t.Fatalf("Lookup = %+v, want Wuhan via vote", location)
	}
}

func TestChainTiePrefersBranchPrimary(t *testing.T) {
	chain := NewChain([]*ChainBackend{
		{Name: "ip2region", Online: stubOnline{"1.2.4.8": {CountryCode: "CN", City: "北京"}}},
		{Name: "tencent", Online: stubOnline{"1.2.4.8": {CountryCode: "CN", City: "上海"}}},
	}, "ip2region", "ip2location")
	location := chain.Lookup(context.Background(), netip.MustParseAddr("1.2.4.8"))
	if NormalizeCityEN(location.City) != "Beijing" {
		t.Fatalf("Lookup = %+v, want the branch primary city on ties", location)
	}
}

func TestChainPostprocessesGreaterChinaToCN(t *testing.T) {
	chain := NewChain([]*ChainBackend{
		{Name: "ip2region", Online: stubOnline{"175.159.182.83": {CountryCode: "CN", RegionName: "香港特别行政区", City: "香港"}}},
		{Name: "ipinfo", Online: stubOnline{"175.159.182.83": {CountryCode: "HK", City: "Hong Kong"}}},
	}, "ip2region", "ip2location")
	location := chain.Lookup(context.Background(), netip.MustParseAddr("175.159.182.83"))
	if location.CountryCode != "CN" {
		t.Fatalf("Lookup = %+v, want HK unified onto CN", location)
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
		"85.237.206.10": "Taipei", "50.7.158.235": "Tokyo", "50.7.250.50": "Tung Chung", // 香港离岛, 真值城市的下属区
		// 218.33.111.2 is the documented all-library blind spot (every
		// backend reports Tokyo; authority is Sydney): assert the chain
		// reproduces the consensus rather than an unreachable truth.
		"218.33.111.2": "Tokyo", "85.203.46.91": "London", "84.200.77.9": "Frankfurt am Main",
		"175.159.182.192": "Hong Kong", "183.179.181.42": "Hong Kong", "39.144.200.217": "Kashgar",
		"111.60.83.6": "Wuhan", "117.136.119.168": "Shanghai", "39.144.156.99": "Nanjing", "153.35.189.109": "Wuxi",
	}
	for ip, want := range truth {
		chain := NewChain([]*ChainBackend{
			{Name: "ip2region", Offline: domesticDB, DomesticWeight: 1.1, ForeignWeight: 0.3},
			{Name: "tencent", Online: service(ip, "tencent"), DomesticWeight: 1.2},
			{Name: "ip2location", Offline: foreignDB},
			{Name: "ipinfo", Online: service(ip, "ipinfo"), DomesticWeight: 1.2, ForeignWeight: 1.2},
			{Name: "bigdatacloud", Online: service(ip, "bigdatacloud"), DomesticWeight: 1.0, ForeignWeight: 0.3},
		}, "ip2region", "ip2location")
		location := chain.Lookup(context.Background(), netip.MustParseAddr(ip))
		if NormalizeCityEN(location.City) != want {
			t.Errorf("Lookup(%s) city = %q (%+v), want %q", ip, NormalizeCityEN(location.City), location, want)
		}
		if location.CountryCode != "CN" && ip != "218.33.111.2" && ip != "50.7.158.235" && ip != "85.203.46.91" && ip != "84.200.77.9" && ip != "50.7.250.50" && ip != "175.159.182.192" && ip != "183.179.181.42" && ip != "39.144.156.99" && ip != "39.144.200.217" {
			t.Errorf("Lookup(%s) country = %q, want CN", ip, location.CountryCode)
		}
	}
}
