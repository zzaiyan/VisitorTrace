package geoip

import (
	"context"
	"net/netip"
	"strings"
	"time"
)

// OnlineLookup is the minimal query interface online GeoIP clients satisfy.
type OnlineLookup interface {
	Lookup(context.Context, netip.Addr) (Location, error)
}

// ChainBackend is one queryable backend with its calibrated voting weights.
// A backend carries either an offline Resolver or an OnlineLookup client.
type ChainBackend struct {
	Name           string
	Offline        *Resolver
	Online         OnlineLookup
	DomesticWeight float64
	ForeignWeight  float64
}

func (b *ChainBackend) lookup(ctx context.Context, address netip.Addr) Location {
	if b.Online != nil {
		lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		location, err := b.Online.Lookup(lookupCtx, address)
		if err == nil {
			return location
		}
		return Location{}
	}
	if b.Offline != nil {
		return b.Offline.Lookup(address)
	}
	return Location{}
}

// DefaultChainWeights returns the per-branch voting weights calibrated on the
// 285-address cross-validation and the 13 authoritatively verified samples.
// Domestic weights apply to the CN/HK/TW/MO branch; zero excludes a backend
// from that branch entirely (for example Amap abroad).
func DefaultChainWeights(name string) (domestic, foreign float64) {
	switch name {
	case "ipinfo":
		return 1.2, 1.2
	case "tencent":
		return 1.2, 0
	case "ip2region":
		return 1.1, 0.3
	case "ip2location":
		return 1.0, 1.0
	case "bigdatacloud":
		return 1.0, 0.3
	case "amap":
		return 1.0, 0
	case "dbip":
		return 0.5, 0.5
	default:
		return 1.0, 1.0
	}
}

// Chain resolves an address through a dual-library consensus route: the
// domestic and foreign offline primaries are queried together, their
// country codes decide the branch, and the branch's online counterpart
// cross-validates the city. Disagreements fall through to a weighted vote
// across every configured backend. Numeric ISO codes and the HK/TW/MO codes
// are post-processed at the chain output.
type Chain struct {
	backends             []*ChainBackend
	domesticID           string
	foreignID            string
	greaterChinaDomestic bool
}

// NewChain builds the resolution chain. domestic and foreign name the
// primary backends for each branch. When greaterChinaDomestic is true,
// addresses that the domestic library labels CN and the foreign library
// labels HK/TW/MO stay on the domestic branch; otherwise they route to
// the foreign branch for ISO-accurate handling.
func NewChain(backends []*ChainBackend, domestic, foreign string, greaterChinaDomestic bool) *Chain {
	return &Chain{backends: backends, domesticID: domestic, foreignID: foreign, greaterChinaDomestic: greaterChinaDomestic}
}

// SetDomestic attaches or replaces the domestic-branch offline resolver.
func (c *Chain) SetDomestic(resolver *Resolver) {
	if backend := c.backend(c.domesticID); backend != nil {
		backend.Offline = resolver
		return
	}
	c.backends = append(c.backends, &ChainBackend{Name: c.domesticID, Offline: resolver, DomesticWeight: 1.1, ForeignWeight: 0.3})
}

// SetForeign attaches or replaces the foreign-branch offline resolver.
func (c *Chain) SetForeign(resolver *Resolver) {
	if backend := c.backend(c.foreignID); backend != nil {
		backend.Offline = resolver
		return
	}
	c.backends = append(c.backends, &ChainBackend{Name: c.foreignID, Offline: resolver})
}

func (c *Chain) backend(name string) *ChainBackend {
	for _, backend := range c.backends {
		if backend.Name == name {
			return backend
		}
	}
	return nil
}

// routeDomestic decides the branch from the two offline primaries: when
// both indicate China (or the foreign library has no data), the address
// goes to the domestic branch. When the domestic library labels an address
// CN but the foreign library labels it HK/TW/MO, the greaterChinaDomestic
// flag decides. Any other disagreement trusts the foreign library's ISO
// code, which our cross-validation showed is more reliable for non-CN
// routing than the domestic library's country field.
func (c *Chain) routeDomestic(domestic, foreign Location) bool {
	domesticCN := NormalizeCountryCode(domestic.CountryCode) == "CN"
	foreignCC := NormalizeCountryCode(foreign.CountryCode)
	switch {
	case domesticCN && foreignCC == "":
		return true
	case domesticCN && foreignCC == "CN":
		return true
	case domesticCN && (foreignCC == "HK" || foreignCC == "TW" || foreignCC == "MO"):
		return c.greaterChinaDomestic
	case domesticCN:
		return false
	case foreignCC == "CN":
		return true
	default:
		return false
	}
}

// Lookup resolves one address through the full chain: query both offline
// primaries, decide the branch from their consensus, cross-validate with
// the branch's online counterpart, and vote on disagreements.
func (c *Chain) Lookup(ctx context.Context, address netip.Addr) Location {
	// Always query both offline primaries; their results drive routing and
	// are reused as the branch primary answer.
	domesticResult := Location{}
	if backend := c.backend(c.domesticID); backend != nil {
		domesticResult = backend.lookup(ctx, address)
	}
	foreignResult := Location{}
	if backend := c.backend(c.foreignID); backend != nil {
		foreignResult = backend.lookup(ctx, address)
	}
	domestic := c.routeDomestic(domesticResult, foreignResult)

	primary := foreignResult
	if domestic {
		primary = domesticResult
	}

	// Cross-validate against the branch's online counterpart.
	cross := Location{}
	for _, backend := range c.backends {
		if backend.Online == nil || backend.Name == c.branchName(domestic) {
			continue
		}
		if domestic && backend.DomesticWeight <= 0 {
			continue
		}
		if !domestic && backend.ForeignWeight <= 0 {
			continue
		}
		candidate := backend.lookup(ctx, address)
		if candidate.City != "" || candidate.CountryCode != "" {
			if cross.City == "" && cross.CountryCode == "" {
				cross = candidate
			}
		}
	}
	city := NormalizeCityEN(primary.City)
	crossCity := NormalizeCityEN(cross.City)
	if city != "" && city == crossCity {
		return PostprocessLocation(primary)
	}
	if city == "" && crossCity == "" {
		if primary.CountryCode == "" && cross.CountryCode == "" {
			return Location{}
		}
		if primary.CountryCode == "" {
			return PostprocessLocation(cross)
		}
		return PostprocessLocation(primary)
	}
	return c.vote(ctx, address, domestic, primary)
}

// vote queries every backend eligible for the branch and returns the
// weighted-majority city, preferring the branch primary on ties or misses.
func (c *Chain) vote(ctx context.Context, address netip.Addr, domestic bool, fallback Location) Location {
	type vote struct {
		weight  float64
		sample  Location
		primary bool
	}
	tally := map[string]*vote{}
	for _, backend := range c.backends {
		weight := backend.DomesticWeight
		if !domestic {
			weight = backend.ForeignWeight
		}
		if weight <= 0 {
			continue
		}
		location := backend.lookup(ctx, address)
		key := NormalizeCityEN(location.City)
		if key == "" {
			key = "country:" + strings.ToUpper(location.CountryCode)
			if location.CountryCode == "" {
				continue
			}
		}
		existing, ok := tally[key]
		if !ok {
			existing = &vote{sample: location}
			tally[key] = existing
		} else if NormalizeCountryCode(existing.sample.CountryCode) == "" && NormalizeCountryCode(location.CountryCode) != "" {
			existing.sample = location
		}
		existing.weight += weight
		if backend.Name == c.branchName(domestic) {
			existing.primary = true
		}
	}
	bestWeight := 0.0
	var best *vote
	primaryWeight := 0.0
	for _, entry := range tally {
		if entry.primary {
			primaryWeight = entry.weight
		}
		if entry.weight > bestWeight {
			bestWeight, best = entry.weight, entry
		}
	}
	if best == nil || bestWeight <= 0 {
		return PostprocessLocation(fallback)
	}
	// A strict majority wins outright; otherwise prefer the branch primary
	// when it is within one backend weight of the leader.
	if best.primary || bestWeight > primaryWeight+0.01 {
		return PostprocessLocation(best.sample)
	}
	return PostprocessLocation(fallback)
}

func (c *Chain) branchName(domestic bool) string {
	if domestic {
		return c.domesticID
	}
	return c.foreignID
}

// isoNumericCountries maps the numeric ISO 3166-1 codes some services
// return onto the alpha-2 codes VisitorTrace stores.
var isoNumericCountries = map[string]string{
	"156": "CN", "840": "US", "392": "JP", "410": "KR", "826": "GB", "276": "DE",
	"250": "FR", "036": "AU", "36": "AU", "124": "CA", "643": "RU", "356": "IN",
	"158": "TW", "344": "HK", "446": "MO", "702": "SG", "458": "MY", "360": "ID",
	"608": "PH", "764": "TH", "398": "KZ", "104": "MM", "524": "NP", "144": "LK",
	"116": "KH", "418": "LA", "496": "MN", "364": "IR", "400": "JO", "634": "QA",
	"414": "KW", "484": "MX", "032": "AR", "076": "BR", "710": "ZA", "818": "EG",
}

// NormalizeCountryCode maps numeric ISO codes to alpha-2 and upper-cases
// the result; anything unrecognized is returned unchanged.
func NormalizeCountryCode(value string) string {
	value = strings.TrimSpace(value)
	if code, ok := isoNumericCountries[value]; ok {
		return code
	}
	return strings.ToUpper(value)
}

// PostprocessLocation unifies numeric codes and Hong Kong, Macau, and
// Taiwan onto CN. The original region and city labels are preserved for
// finer-grained views.
func PostprocessLocation(location Location) Location {
	location.CountryCode = NormalizeCountryCode(location.CountryCode)
	switch location.CountryCode {
	case "HK", "TW", "MO":
		location.CountryCode = "CN"
	}
	return location
}
