package geoip

import (
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

type Resolver struct {
	reader *maxminddb.Reader
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

func Open(path string) (*Resolver, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("GeoIP path is empty")
	}
	reader, err := maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open GeoIP database: %w", err)
	}
	return &Resolver{reader: reader}, nil
}

func Validate(path string) error {
	reader, err := maxminddb.Open(path)
	if err != nil {
		return fmt.Errorf("open GeoIP database: %w", err)
	}
	defer reader.Close()
	databaseType := strings.ToLower(reader.Metadata.DatabaseType)
	if !strings.Contains(databaseType, "city") && !strings.Contains(databaseType, "location") {
		return fmt.Errorf("unsupported GeoIP database type %q", reader.Metadata.DatabaseType)
	}
	if err := reader.Verify(); err != nil {
		return fmt.Errorf("verify GeoIP database: %w", err)
	}
	return nil
}

func (r *Resolver) Lookup(address netip.Addr) Location {
	if r == nil || r.reader == nil || !address.IsValid() {
		return Location{}
	}
	var record mmdbRecord
	if err := r.reader.Lookup(net.IP(address.AsSlice()), &record); err != nil {
		return Location{}
	}
	result := Location{
		CountryCode: record.Country.ISOCode,
		CountryName: localizedName(record.Country.Names),
	}
	if len(record.Subdivisions) > 0 {
		result.RegionCode = record.Subdivisions[0].ISOCode
		result.RegionName = localizedName(record.Subdivisions[0].Names)
	}
	result.City = CityLevelName(result.CountryCode, localizedName(record.City.Names), result.RegionName)
	if record.Location.Latitude != 0 || record.Location.Longitude != 0 {
		latitude := record.Location.Latitude
		longitude := record.Location.Longitude
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
