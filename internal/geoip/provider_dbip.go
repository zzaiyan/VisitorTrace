package geoip

import (
	"fmt"
	"net"
	"strings"

	"github.com/oschwald/maxminddb-golang"
)

type dbipProvider struct{}

func (dbipProvider) attribution() Attribution {
	return Attribution{URL: "https://db-ip.com", Label: "IP geolocation by DB-IP"}
}

func (dbipProvider) updateProfile() UpdateProfile {
	return UpdateProfile{
		URL:             "https://download.db-ip.com/free/dbip-city-lite-{YYYY-MM}.mmdb.gz",
		OfficialHost:    "download.db-ip.com",
		CalendarMonthly: true,
	}
}

func (dbipProvider) validate(reader *maxminddb.Reader) error {
	databaseType := strings.ToLower(reader.Metadata.DatabaseType)
	if !strings.Contains(databaseType, "dbip") || !strings.Contains(databaseType, "city") {
		return fmt.Errorf("unsupported GeoIP database type %q for provider %s", reader.Metadata.DatabaseType, ProviderDBIP)
	}
	return nil
}

func (dbipProvider) lookup(reader *maxminddb.Reader, address net.IP) Location {
	result := lookupNested(reader, address)
	result.City = normalizeDBIPCity(result.CountryCode, result.City, result.RegionName)
	return result
}

// normalizeDBIPCity removes lower-level qualifiers from Chinese locality
// labels returned by DB-IP City Lite. Its schema does not expose a feature
// code that can reliably distinguish a city from a district or subdistrict.
func normalizeDBIPCity(countryCode, city, region string) string {
	city = strings.TrimSpace(city)
	if city == "" || strings.ToUpper(strings.TrimSpace(countryCode)) != "CN" {
		return city
	}
	region = strings.TrimSpace(region)
	if isDBIPChinaMunicipality(region) {
		return region
	}
	if open := strings.Index(city, " ("); open > 0 && strings.HasSuffix(city, ")") {
		base := strings.TrimSpace(city[:open])
		detail := strings.TrimSpace(city[open+2 : len(city)-1])
		parts := strings.Split(detail, ",")
		if len(parts) > 1 {
			candidate := strings.TrimSpace(parts[len(parts)-1])
			if candidate != "" && !looksLikeDBIPFinePlace(candidate) {
				return candidate
			}
		}
		if strings.HasSuffix(strings.ToLower(base), " old town") {
			return strings.TrimSpace(base[:len(base)-len(" old town")])
		}
		if looksLikeDBIPFinePlace(base) {
			return ""
		}
		return base
	}
	if strings.HasSuffix(strings.ToLower(city), " old town") {
		return strings.TrimSpace(city[:len(city)-len(" old town")])
	}
	if looksLikeDBIPFinePlace(city) {
		return ""
	}
	return city
}

func isDBIPChinaMunicipality(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "beijing", "shanghai", "tianjin", "chongqing":
		return true
	default:
		return false
	}
}

func looksLikeDBIPFinePlace(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	for _, suffix := range []string{" district", " subdistrict", " residential district", " county", " town", " village", " street", "区", "街道", "镇", "乡", "村", "县"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}
