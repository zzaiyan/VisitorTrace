package geoip

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/oschwald/maxminddb-golang"
)

type ip2LocationProvider struct{}

type ip2LocationRecord struct {
	CountryCode string  `maxminddb:"country_code"`
	CountryName string  `maxminddb:"country_name"`
	RegionName  string  `maxminddb:"region_name"`
	CityName    string  `maxminddb:"city_name"`
	Latitude    float64 `maxminddb:"latitude"`
	Longitude   float64 `maxminddb:"longitude"`
}

func (ip2LocationProvider) attribution() Attribution {
	return Attribution{URL: "https://www.ip2location.com", Label: "IP geolocation by IP2Location"}
}

func (ip2LocationProvider) updateProfile() UpdateProfile {
	return UpdateProfile{
		URL:             "https://www.ip2location.com/download?file=DB11LITEMMDB",
		OfficialHost:    "www.ip2location.com",
		CalendarMonthly: true,
	}
}

func (ip2LocationProvider) validate(reader *maxminddb.Reader) error {
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
	if result := lookupIP2LocationFlat(reader, network.IP); result != (Location{}) {
		return nil
	}
	if result := lookupNested(reader, network.IP); result == (Location{}) {
		return errors.New("IP2Location database does not expose supported city and coordinate fields")
	}
	return nil
}

func (ip2LocationProvider) lookup(reader *maxminddb.Reader, address net.IP) Location {
	if result := lookupIP2LocationFlat(reader, address); result != (Location{}) {
		return result
	}
	// Some IP2Location MMDB releases use the MaxMind-compatible nested shape.
	return lookupNested(reader, address)
}

func lookupIP2LocationFlat(reader *maxminddb.Reader, address net.IP) Location {
	var record ip2LocationRecord
	if err := reader.Lookup(address, &record); err != nil {
		return Location{}
	}
	return locationFromIP2LocationRecord(record)
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
