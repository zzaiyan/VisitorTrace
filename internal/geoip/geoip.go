package geoip

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/oschwald/maxminddb-golang"
)

type Location struct {
	CountryCode string
	CountryName string
	RegionCode  string
	RegionName  string
	City        string
	Latitude    *float64
	Longitude   *float64
}

type Provider string

const (
	ProviderDBIP        Provider = "dbip"
	ProviderMaxMind     Provider = "maxmind"
	ProviderIP2Location Provider = "ip2location"
)

type Attribution struct {
	URL   string
	Label string
}

func NormalizeProvider(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		value = string(ProviderDBIP)
	}
	switch Provider(value) {
	case ProviderDBIP, ProviderMaxMind, ProviderIP2Location:
		return value, nil
	default:
		return "", fmt.Errorf("unsupported GeoIP provider %q (want dbip, maxmind, or ip2location)", value)
	}
}

func AttributionForProvider(value string) Attribution {
	provider, err := NormalizeProvider(value)
	if err != nil {
		provider = string(ProviderDBIP)
	}
	switch Provider(provider) {
	case ProviderMaxMind:
		return Attribution{URL: "https://www.maxmind.com", Label: "IP geolocation by MaxMind"}
	case ProviderIP2Location:
		return Attribution{URL: "https://www.ip2location.com", Label: "IP geolocation by IP2Location"}
	default:
		return Attribution{URL: "https://db-ip.com", Label: "IP geolocation by DB-IP"}
	}
}

type Resolver struct {
	reader   *maxminddb.Reader
	provider Provider
}

type mmdbRecord struct {
	Country struct {
		ISOCode string            `maxminddb:"iso_code"`
		Names   map[string]string `maxminddb:"names"`
	} `maxminddb:"country"`
	Subdivisions []struct {
		ISOCode string            `maxminddb:"iso_code"`
		Names   map[string]string `maxminddb:"names"`
	} `maxminddb:"subdivisions"`
	City struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
	Location struct {
		Latitude  float64 `maxminddb:"latitude"`
		Longitude float64 `maxminddb:"longitude"`
	} `maxminddb:"location"`
}

type ip2LocationRecord struct {
	CountryCode string  `maxminddb:"country_code"`
	CountryName string  `maxminddb:"country_name"`
	RegionName  string  `maxminddb:"region_name"`
	CityName    string  `maxminddb:"city_name"`
	Latitude    float64 `maxminddb:"latitude"`
	Longitude   float64 `maxminddb:"longitude"`
}

func Open(path string) (*Resolver, error) {
	return OpenWithProvider(string(ProviderDBIP), path)
}

func OpenWithProvider(provider, path string) (*Resolver, error) {
	normalized, err := NormalizeProvider(provider)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("GeoIP path is empty")
	}
	reader, err := maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open GeoIP database: %w", err)
	}
	if err := validateProvider(reader, Provider(normalized), false); err != nil {
		_ = reader.Close()
		return nil, err
	}
	return &Resolver{reader: reader, provider: Provider(normalized)}, nil
}

func Validate(path string) error {
	return ValidateWithProvider(string(ProviderDBIP), path)
}

func ValidateWithProvider(provider, path string) error {
	normalized, err := NormalizeProvider(provider)
	if err != nil {
		return err
	}
	reader, err := maxminddb.Open(path)
	if err != nil {
		return fmt.Errorf("open GeoIP database: %w", err)
	}
	defer reader.Close()
	return validateProvider(reader, Provider(normalized), true)
}

func validateProvider(reader *maxminddb.Reader, provider Provider, verify bool) error {
	databaseType := strings.ToLower(reader.Metadata.DatabaseType)
	switch provider {
	case ProviderDBIP:
		if !strings.Contains(databaseType, "dbip") || !strings.Contains(databaseType, "city") {
			return fmt.Errorf("unsupported GeoIP database type %q for provider %s", reader.Metadata.DatabaseType, provider)
		}
	case ProviderMaxMind:
		if (!strings.Contains(databaseType, "geoip2") && !strings.Contains(databaseType, "geolite2")) || !strings.Contains(databaseType, "city") {
			return fmt.Errorf("unsupported GeoIP database type %q for provider %s", reader.Metadata.DatabaseType, provider)
		}
	case ProviderIP2Location:
		if err := validateIP2LocationSchema(reader); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported GeoIP provider %q", provider)
	}
	if verify {
		if err := reader.Verify(); err != nil {
			return fmt.Errorf("verify GeoIP database: %w", err)
		}
	}
	return nil
}

func validateIP2LocationSchema(reader *maxminddb.Reader) error {
	networks := reader.Networks(maxminddb.SkipAliasedNetworks)
	if !networks.Next() {
		if err := networks.Err(); err != nil {
			return fmt.Errorf("inspect IP2Location database: %w", err)
		}
		return errors.New("IP2Location database contains no networks")
	}
	var raw map[string]any
	network, err := networks.Network(&raw)
	if err != nil {
		return fmt.Errorf("inspect IP2Location database: %w", err)
	}
	var flat ip2LocationRecord
	if err := reader.Lookup(network.IP, &flat); err != nil {
		return fmt.Errorf("read IP2Location database: %w", err)
	}
	if locationFromIP2LocationRecord(flat) != (Location{}) {
		return nil
	}
	var nested mmdbRecord
	if err := reader.Lookup(network.IP, &nested); err != nil {
		return fmt.Errorf("read IP2Location database: %w", err)
	}
	if locationFromMMDBRecord(nested, false) == (Location{}) {
		return errors.New("IP2Location database does not expose supported city and coordinate fields")
	}
	return nil
}

func (r *Resolver) Lookup(address netip.Addr) Location {
	if r == nil || r.reader == nil || !address.IsValid() {
		return Location{}
	}
	if r.provider == ProviderIP2Location {
		return r.lookupIP2Location(address)
	}
	var record mmdbRecord
	if err := r.reader.Lookup(net.IP(address.AsSlice()), &record); err != nil {
		return Location{}
	}
	return locationFromMMDBRecord(record, r.provider == ProviderDBIP)
}

func locationFromMMDBRecord(record mmdbRecord, normalizeDBIP bool) Location {
	result := Location{
		CountryCode: record.Country.ISOCode,
		CountryName: localizedName(record.Country.Names),
	}
	if len(record.Subdivisions) > 0 {
		result.RegionCode = record.Subdivisions[0].ISOCode
		result.RegionName = localizedName(record.Subdivisions[0].Names)
	}
	result.City = localizedName(record.City.Names)
	if normalizeDBIP {
		result.City = CityLevelName(result.CountryCode, result.City, result.RegionName)
	}
	if record.Location.Latitude != 0 || record.Location.Longitude != 0 {
		latitude := record.Location.Latitude
		longitude := record.Location.Longitude
		result.Latitude = &latitude
		result.Longitude = &longitude
	}
	return result
}

func (r *Resolver) lookupIP2Location(address netip.Addr) Location {
	var record ip2LocationRecord
	if err := r.reader.Lookup(net.IP(address.AsSlice()), &record); err != nil {
		return Location{}
	}
	if result := locationFromIP2LocationRecord(record); result != (Location{}) {
		return result
	}

	// Some IP2Location MMDB releases use the MaxMind-compatible nested shape
	// so they can be consumed by existing MMDB integrations.
	var nested mmdbRecord
	if err := r.reader.Lookup(net.IP(address.AsSlice()), &nested); err != nil {
		return Location{}
	}
	return locationFromMMDBRecord(nested, false)
}

func locationFromIP2LocationRecord(record ip2LocationRecord) Location {
	if strings.TrimSpace(record.CountryCode) == "" && strings.TrimSpace(record.CountryName) == "" && strings.TrimSpace(record.RegionName) == "" && strings.TrimSpace(record.CityName) == "" && record.Latitude == 0 && record.Longitude == 0 {
		return Location{}
	}
	result := Location{
		CountryCode: strings.TrimSpace(record.CountryCode),
		CountryName: strings.TrimSpace(record.CountryName),
		RegionName:  strings.TrimSpace(record.RegionName),
		City:        strings.TrimSpace(record.CityName),
	}
	if record.Latitude != 0 || record.Longitude != 0 {
		latitude := record.Latitude
		longitude := record.Longitude
		result.Latitude = &latitude
		result.Longitude = &longitude
	}
	return result
}

// CityLevelName removes DB-IP's lower-level qualifier from Chinese locality
// labels. DB-IP City Lite exposes a city name and broad subdivisions, but no
// feature code that can distinguish a city from a district or subdistrict.
func CityLevelName(countryCode, city, region string) string {
	city = strings.TrimSpace(city)
	if city == "" || strings.ToUpper(strings.TrimSpace(countryCode)) != "CN" {
		return city
	}
	region = strings.TrimSpace(region)
	if isChinaMunicipality(region) {
		return region
	}
	if open := strings.Index(city, " ("); open > 0 && strings.HasSuffix(city, ")") {
		base := strings.TrimSpace(city[:open])
		detail := strings.TrimSpace(city[open+2 : len(city)-1])
		parts := strings.Split(detail, ",")
		if len(parts) > 1 {
			candidate := strings.TrimSpace(parts[len(parts)-1])
			if candidate != "" && !looksLikeFinePlace(candidate) {
				return candidate
			}
		}
		if strings.HasSuffix(strings.ToLower(base), " old town") {
			return strings.TrimSpace(base[:len(base)-len(" old town")])
		}
		if looksLikeFinePlace(base) {
			return ""
		}
		return base
	}
	if strings.HasSuffix(strings.ToLower(city), " old town") {
		return strings.TrimSpace(city[:len(city)-len(" old town")])
	}
	if looksLikeFinePlace(city) {
		return ""
	}
	return city
}

func isChinaMunicipality(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "beijing", "shanghai", "tianjin", "chongqing":
		return true
	default:
		return false
	}
}

func looksLikeFinePlace(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	for _, suffix := range []string{" district", " subdistrict", " residential district", " county", " town", " village", " street", "区", "街道", "镇", "乡", "村", "县"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

func (r *Resolver) Close() error {
	if r == nil || r.reader == nil {
		return nil
	}
	return r.reader.Close()
}

func localizedName(values map[string]string) string {
	for _, key := range []string{"en", "zh-CN", "zh"} {
		if value := values[key]; value != "" {
			return value
		}
	}
	for _, value := range values {
		return value
	}
	return ""
}
