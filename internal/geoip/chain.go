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

// Chain routes addresses through domestic and foreign primaries. With a
// second primary in each branch it cross-checks cities and votes on conflicts.
// Numeric ISO codes and HK/TW/MO are normalized at the output.
type Chain struct {
	backends             []*ChainBackend
	domesticID           string
	foreignID            string
	domesticCrossID      string
	foreignCrossID       string
	crossConfigured      bool
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

// SetCross selects the second backend used to check each branch's primary.
// An empty name means that branch has no cross-checking backend.
func (c *Chain) SetCross(domestic, foreign string) {
	c.domesticCrossID, c.foreignCrossID = domestic, foreign
	c.crossConfigured = true
}

// SetDomestic attaches or replaces the domestic-branch offline resolver.
func (c *Chain) SetDomestic(resolver *Resolver) {
	if backend := c.backend(c.domesticID); backend != nil {
		backend.Offline = resolver
		return
	}
	domestic, foreign := DefaultChainWeights(c.domesticID)
	c.backends = append(c.backends, &ChainBackend{Name: c.domesticID, Offline: resolver, DomesticWeight: domestic, ForeignWeight: foreign})
}

// SetForeign attaches or replaces the foreign-branch offline resolver.
func (c *Chain) SetForeign(resolver *Resolver) {
	if backend := c.backend(c.foreignID); backend != nil {
		backend.Offline = resolver
		return
	}
	domestic, foreign := DefaultChainWeights(c.foreignID)
	c.backends = append(c.backends, &ChainBackend{Name: c.foreignID, Offline: resolver, DomesticWeight: domestic, ForeignWeight: foreign})
}

// SetBackup attaches an offline resolver that participates in conflict votes.
func (c *Chain) SetBackup(name string, resolver *Resolver) {
	if backend := c.backend(name); backend != nil {
		backend.Offline = resolver
		return
	}
	domestic, foreign := DefaultChainWeights(name)
	c.backends = append(c.backends, &ChainBackend{Name: name, Offline: resolver, DomesticWeight: domestic, ForeignWeight: foreign})
}

func (c *Chain) backend(name string) *ChainBackend {
	for _, backend := range c.backends {
		if backend.Name == name {
			return backend
		}
	}
	return nil
}

// Available reports whether any selected backend can currently answer a lookup.
func (c *Chain) Available() bool {
	if c == nil {
		return false
	}
	for _, backend := range c.backends {
		if backend.Offline != nil || backend.Online != nil {
			return true
		}
	}
	return false
}

// routeDomestic decides the branch from the two primaries: when
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

// Lookup resolves one address through the selected branch and, where
// configured, cross-checks it against a second primary.
func (c *Chain) Lookup(ctx context.Context, address netip.Addr) Location {
	// With one primary per branch, the foreign offline source first determines
	// whether an online domestic lookup is needed. This saves online quota for
	// addresses already identified as foreign.
	if c.crossConfigured && c.domesticCrossID == "" && c.foreignCrossID == "" {
		foreignResult := Location{}
		if backend := c.backend(c.foreignID); backend != nil {
			foreignResult = backend.lookup(ctx, address)
		}
		foreignCC := NormalizeCountryCode(foreignResult.CountryCode)
		if foreignCC != "" && foreignCC != "CN" && foreignCC != "HK" && foreignCC != "TW" && foreignCC != "MO" {
			return PostprocessLocation(foreignResult)
		}
		domesticResult := Location{}
		if backend := c.backend(c.domesticID); backend != nil {
			domesticResult = backend.lookup(ctx, address)
		}
		if c.routeDomestic(domesticResult, foreignResult) {
			if domesticResult.CountryCode != "" || domesticResult.City != "" {
				return PostprocessLocation(domesticResult)
			}
			return PostprocessLocation(foreignResult)
		}
		if foreignResult.CountryCode != "" || foreignResult.City != "" {
			return PostprocessLocation(foreignResult)
		}
		return PostprocessLocation(domesticResult)
	}
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
	if c.crossConfigured {
		name := c.foreignCrossID
		if domestic {
			name = c.domesticCrossID
		}
		if name != "" && name != c.branchName(domestic) {
			if backend := c.backend(name); backend != nil {
				cross = backend.lookup(ctx, address)
			}
		}
	} else {
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
	}
	city := NormalizeCityEN(primary.City)
	crossCity := NormalizeCityEN(cross.City)
	if city != "" && city == crossCity {
		if primary.Latitude == nil && cross.Latitude != nil && (primary.CountryCode == "" || cross.CountryCode == "" || NormalizeCountryCode(primary.CountryCode) == NormalizeCountryCode(cross.CountryCode)) {
			primary.Latitude = cross.Latitude
			primary.Longitude = cross.Longitude
		}
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
		order   int
	}
	tally := map[string]*vote{}
	for order, backend := range c.backends {
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
			existing = &vote{sample: location, order: order}
			tally[key] = existing
		} else {
			// Prefer a sample that carries both a valid country code and
			// coordinates, so the map can render the winning city.
			if existing.sample.Latitude == nil && location.Latitude != nil {
				if existing.sample.CountryCode == "" && location.CountryCode != "" {
					existing.sample = location
				} else if existing.sample.CountryCode != "" || location.CountryCode != "" {
					// Keep the city but upgrade coordinates from the new sample.
					upgraded := existing.sample
					upgraded.Latitude = location.Latitude
					upgraded.Longitude = location.Longitude
					existing.sample = upgraded
				}
			} else if NormalizeCountryCode(existing.sample.CountryCode) == "" && NormalizeCountryCode(location.CountryCode) != "" {
				existing.sample = location
			}
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
		if entry.weight > bestWeight+0.01 || (best != nil && entry.weight >= bestWeight-0.01 && ((entry.primary && !best.primary) || (entry.primary == best.primary && entry.order < best.order))) {
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

// PostprocessLocation unifies country and city labels before they are stored.
func PostprocessLocation(location Location) Location {
	location.CountryCode = NormalizeCountryCode(location.CountryCode)
	switch location.CountryCode {
	case "HK", "TW", "MO":
		location.CountryCode = "CN"
	}
	location.City = NormalizeCityEN(location.City)
	return location
}
