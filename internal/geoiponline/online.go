// Package geoiponline provides opt-in online IP geolocation lookups from
// free city-level APIs. Online lookups send the visitor IP to a third party,
// so the feature is disabled by default and configured explicitly.
package geoiponline

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/geoip"
)

const (
	cacheTTL        = 30 * 24 * time.Hour
	maxCacheEntries = 1 << 16
	responseLimit   = 1 << 20
)

type Client struct {
	provider string
	token    string
	secret   string
	baseURL  string
	http     *http.Client

	mu    sync.Mutex
	cache map[netip.Addr]cacheEntry
}

type cacheEntry struct {
	location geoip.Location
	expires  time.Time
}

func NormalizeProvider(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "tencent":
		return "tencent", nil
	case "amap":
		return "amap", nil
	case "ipinfo":
		return "ipinfo", nil
	case "bigdatacloud":
		return "bigdatacloud", nil
	default:
		return "", fmt.Errorf("unsupported online GeoIP provider %q (want tencent, amap, ipinfo, or bigdatacloud)", value)
	}
}

// New builds a client from the credential string. Tencent and Amap keys
// created with signature verification take the form "KEY:SK", where SK is
// the secret key used to sign every request; other providers use the plain
// key. The credential never leaves this struct.
func New(provider, credential string, timeout time.Duration) (*Client, error) {
	normalized, err := NormalizeProvider(provider)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(credential) == "" {
		return nil, fmt.Errorf("online GeoIP provider %s requires an API key", normalized)
	}
	token, secret := credential, ""
	if normalized == "tencent" || normalized == "amap" {
		if key, sk, ok := strings.Cut(credential, ":"); ok {
			token, secret = key, sk
		}
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	endpoints := map[string]string{
		"tencent":      "https://apis.map.qq.com",
		"amap":         "https://restapi.amap.com",
		"ipinfo":       "https://ipinfo.io",
		"bigdatacloud": "https://api.bigdatacloud.net",
	}
	return &Client{
		provider: normalized,
		token:    strings.TrimSpace(token),
		secret:   strings.TrimSpace(secret),
		baseURL:  endpoints[normalized],
		http:     &http.Client{Timeout: timeout},
		cache:    make(map[netip.Addr]cacheEntry),
	}, nil
}

// setBaseURL points the client at a different endpoint; used by tests.
func (c *Client) setBaseURL(value string) { c.baseURL = value }

// Lookup resolves an address through the configured API and caches the
// result, including negative results so unanswered addresses are not
// re-queried.
func (c *Client) Lookup(ctx context.Context, address netip.Addr) (geoip.Location, error) {
	if c == nil || !address.IsValid() {
		return geoip.Location{}, nil
	}
	if cached, ok := c.cached(address); ok {
		return cached.location, nil
	}
	location, err := c.fetch(ctx, address)
	if err != nil {
		return geoip.Location{}, err
	}
	c.store(address, location)
	return location, nil
}

func (c *Client) cached(address netip.Addr) (cacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.cache[address]
	if !ok || time.Now().After(entry.expires) {
		return cacheEntry{}, false
	}
	return entry, true
}

func (c *Client) store(address netip.Addr, location geoip.Location) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.cache) >= maxCacheEntries {
		// Cheap full reset instead of LRU bookkeeping; entries are cheap to
		// refill and the TTL dominates anyway.
		c.cache = make(map[netip.Addr]cacheEntry)
	}
	c.cache[address] = cacheEntry{location: location, expires: time.Now().Add(cacheTTL)}
}

func (c *Client) fetch(ctx context.Context, address netip.Addr) (geoip.Location, error) {
	switch c.provider {
	case "tencent":
		return c.fetchTencent(ctx, address)
	case "amap":
		return c.fetchAMap(ctx, address)
	case "ipinfo":
		return c.fetchIPinfo(ctx, address)
	case "bigdatacloud":
		return c.fetchBigDataCloud(ctx, address)
	default:
		return geoip.Location{}, fmt.Errorf("unsupported online GeoIP provider %q", c.provider)
	}
}

func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("create online GeoIP request: %w", err)
	}
	request.Header.Set("User-Agent", "VisitorTrace GeoIP")
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("online GeoIP request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("online GeoIP request returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, responseLimit))
	if err != nil {
		return fmt.Errorf("read online GeoIP response: %w", err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode online GeoIP response: %w", err)
	}
	return nil
}

type tencentResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Result  struct {
		Location struct {
			Lat float64 `json:"lat"`
			Lng float64 `json:"lng"`
		} `json:"location"`
		AdInfo struct {
			Nation     string `json:"nation"`
			NationCode string `json:"nation_code"`
			Province   string `json:"province"`
			City       string `json:"city"`
		} `json:"ad_info"`
	} `json:"result"`
}

func (c *Client) fetchTencent(ctx context.Context, address netip.Addr) (geoip.Location, error) {
	var payload tencentResponse
	tencentQuery := url.Values{"ip": {address.String()}, "key": {c.token}}
	c.signTencent("/ws/location/v1/ip", tencentQuery)
	if err := c.get(ctx, "/ws/location/v1/ip", tencentQuery, &payload); err != nil {
		return geoip.Location{}, err
	}
	if payload.Status != 0 {
		return geoip.Location{}, fmt.Errorf("tencent IP location failed: %s", payload.Message)
	}
	city := strings.TrimSuffix(payload.Result.AdInfo.City, "市")
	latitude := payload.Result.Location.Lat
	longitude := payload.Result.Location.Lng
	location := geoip.Location{
		CountryCode: normalizeNationCode(payload.Result.AdInfo.NationCode),
		CountryName: payload.Result.AdInfo.Nation,
		RegionName:  payload.Result.AdInfo.Province,
		City:        city,
	}
	if latitude != 0 || longitude != 0 {
		location.Latitude = &latitude
		location.Longitude = &longitude
	}
	return location, nil
}

type amapResponse struct {
	Status   string          `json:"status"`
	Info     string          `json:"info"`
	Province string          `json:"province"`
	City     json.RawMessage `json:"city"`
}

func (c *Client) fetchAMap(ctx context.Context, address netip.Addr) (geoip.Location, error) {
	var payload amapResponse
	amapQuery := url.Values{"ip": {address.String()}, "key": {c.token}}
	c.signAMap(amapQuery)
	if err := c.get(ctx, "/v3/ip", amapQuery, &payload); err != nil {
		return geoip.Location{}, err
	}
	if payload.Status != "1" {
		return geoip.Location{}, fmt.Errorf("amap IP location failed: %s", payload.Info)
	}
	city := decodeAMapCity(payload.City)
	if city == "" || city == payload.Province {
		city = payload.Province
	}
	return geoip.Location{
		CountryCode: "CN",
		CountryName: "中国",
		RegionName:  payload.Province,
		City:        strings.TrimSuffix(city, "市"),
	}, nil
}

// decodeAMapCity handles the API returning either a city name or an empty
// JSON array for province-level addresses.
func decodeAMapCity(raw json.RawMessage) string {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "[]" || value == "null" {
		return ""
	}
	var city string
	if err := json.Unmarshal(raw, &city); err != nil {
		return ""
	}
	return city
}

type ipinfoResponse struct {
	Country string `json:"country"`
	Region  string `json:"region"`
	City    string `json:"city"`
	Loc     string `json:"loc"`
}

func (c *Client) fetchIPinfo(ctx context.Context, address netip.Addr) (geoip.Location, error) {
	var payload ipinfoResponse
	if err := c.get(ctx, "/"+address.String()+"/json", url.Values{"token": {c.token}}, &payload); err != nil {
		return geoip.Location{}, err
	}
	location := geoip.Location{CountryCode: payload.Country, RegionName: payload.Region, City: payload.City}
	if latitude, longitude, ok := parseLatLon(payload.Loc); ok {
		location.Latitude = &latitude
		location.Longitude = &longitude
	}
	return location, nil
}

type bigDataCloudResponse struct {
	Country struct {
		IsoAlpha2 string `json:"isoAlpha2"`
		Name      string `json:"name"`
	} `json:"country"`
	Location struct {
		PrincipalSubdivision        string  `json:"principalSubdivision"`
		IsoPrincipalSubdivisionCode string  `json:"isoPrincipalSubdivisionCode"`
		City                        string  `json:"city"`
		Latitude                    float64 `json:"latitude"`
		Longitude                   float64 `json:"longitude"`
	} `json:"location"`
}

func (c *Client) fetchBigDataCloud(ctx context.Context, address netip.Addr) (geoip.Location, error) {
	var payload bigDataCloudResponse
	if err := c.get(ctx, "/data/ip-geolocation", url.Values{"ip": {address.String()}, "key": {c.token}}, &payload); err != nil {
		return geoip.Location{}, err
	}
	location := geoip.Location{
		CountryCode: payload.Country.IsoAlpha2,
		CountryName: payload.Country.Name,
		RegionCode:  bigDataCloudRegionCode(payload.Location.IsoPrincipalSubdivisionCode, payload.Country.IsoAlpha2),
		RegionName:  payload.Location.PrincipalSubdivision,
		City:        payload.Location.City,
	}
	latitude, longitude := payload.Location.Latitude, payload.Location.Longitude
	if latitude != 0 || longitude != 0 {
		location.Latitude = &latitude
		location.Longitude = &longitude
	}
	return location, nil
}

// bigDataCloudRegionCode trims the "XX-" country prefix from ISO 3166-2
// codes like "PK-SD" so the stored subdivision code matches the MMDB style.
func bigDataCloudRegionCode(code, countryCode string) string {
	if prefix := strings.ToUpper(countryCode) + "-"; strings.HasPrefix(strings.ToUpper(code), prefix) {
		return code[len(prefix):]
	}
	return ""
}

// signTencent implements the WebService API signature: MD5 over the
// request path, a "?", the parameters sorted by name (values unescaped),
// and the SK appended directly, as a lowercase hex sig parameter. Keys
// created without signature verification must not sign.
func (c *Client) signTencent(path string, query url.Values) {
	if c.secret == "" {
		return
	}
	digest := md5.Sum([]byte(path + "?" + query.Encode() + c.secret))
	query.Set("sig", hex.EncodeToString(digest[:]))
}

// signAMap implements the digital signature: MD5 over the parameters
// sorted by name (values unescaped) with the SK appended directly.
func (c *Client) signAMap(query url.Values) {
	if c.secret == "" {
		return
	}
	digest := md5.Sum([]byte(query.Encode() + c.secret))
	query.Set("sig", hex.EncodeToString(digest[:]))
}

func parseLatLon(value string) (float64, float64, bool) {
	left, right, ok := strings.Cut(strings.TrimSpace(value), ",")
	if !ok {
		return 0, 0, false
	}
	latitude, err := strconv.ParseFloat(strings.TrimSpace(left), 64)
	if err != nil {
		return 0, 0, false
	}
	longitude, err := strconv.ParseFloat(strings.TrimSpace(right), 64)
	if err != nil {
		return 0, 0, false
	}
	return latitude, longitude, true
}

// normalizeNationCode maps the alpha-3 codes used by some Tencent responses
// onto the ISO 3166-1 alpha-2 codes VisitorTrace stores.
func normalizeNationCode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	switch value {
	case "CHN":
		return "CN"
	case "USA":
		return "US"
	}
	if len(value) == 2 {
		return value
	}
	return ""
}
